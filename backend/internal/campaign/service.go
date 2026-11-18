package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	briefs "smartcampaign/internal/brief"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

const (
	maxAttempts      = 3
	minBriefRunes    = 5
	maxBriefRunes    = 2000
	maxRawLogBytes   = 8 << 10
	segmentTimeout   = 60 * time.Second
	messagesTimeout  = 180 * time.Second // the source text first, then the translations
	auditWriteBudget = 5 * time.Second
)

// Campaign statuses used by this flow.
const (
	StatusDraft        = "draft"
	StatusSegmentReady = "segment_ready"
	StatusContentReady = "content_ready"
)

// Content sources.
const (
	SourceLLM        = "llm"
	SourceFallback   = "fallback"
	SourceEdited     = "edited" // the source text, edited by a person (translations never are)
	SourceTranslated = "translated"
)

// Fallback reasons reported to the UI.
const (
	FallbackInvalidOutput = "invalid_output"
	FallbackUnavailable   = "llm_unavailable"
	FallbackRefused       = "llm_refused"
)

// Service runs the campaign flow.
type Service struct {
	pool   *pgxpool.Pool
	llm    llm.Client
	policy content.Policy
	log    *slog.Logger
	// segmentMode says who understands the brief: the rule reader (Strict, the
	// default and the mock's mode) or the real model (Grounded, D87).
	segmentMode briefs.Mode
}

// UseSegmentMode sets who understands the brief (see brief.Mode).
func (s *Service) UseSegmentMode(m briefs.Mode) *Service { s.segmentMode = m; return s }

func NewService(pool *pgxpool.Pool, client llm.Client, policy content.Policy, log *slog.Logger) *Service {
	return &Service{pool: pool, llm: client, policy: policy, log: log}
}

// Policy returns the content policy (limits/budgets) in force.
func (s *Service) Policy() content.Policy { return s.policy }

// Locales returns the locales table, checked against the policy.
func (s *Service) Locales(ctx context.Context) (content.Locales, error) { return s.loadLocales(ctx) }

// ---- views ----------------------------------------------------------------

type SegmentView struct {
	Status      string              `json:"status"`
	Match       string              `json:"match,omitempty"`
	Conditions  []segment.Condition `json:"conditions"`
	Explanation string              `json:"explanation"`
	Question    string              `json:"question,omitempty"`
	Audience    *segment.Audience   `json:"audience,omitempty"`
	Offer       offer.Offer         `json:"offer"`
	// Suggestion is a one-click answer to Question (segment.SuggestAllCustomers).
	Suggestion string `json:"suggestion,omitempty"`
	// Note is a sentence shown with an ok audience, for example why everyone was
	// chosen ("Kitle belirtilmediği için tüm müşteriler seçildi.").
	Note string `json:"note,omitempty"`
	// Locales is how many reachable people each locale has (for example
	// {"tr-TR": 120, "en-GB": 45}); it is recounted when the audience is built.
	Locales map[string]int `json:"locales,omitempty"`
}

type ContentView struct {
	content.Content
	Locale string `json:"locale"`
	Source string `json:"source"`
	// Version counts the source text's changes; SourceVersion is the source
	// version a translation was made from. Stale: made from an older one.
	Version       int  `json:"version"`
	SourceVersion int  `json:"source_version,omitempty"`
	Stale         bool `json:"stale"`
	// SourceText is the source text a stale translation was made from (so the
	// screen can show what changed); absent when the translation is up to date.
	SourceText *content.Content `json:"source_text,omitempty"`
	// Approach is the source text's approach (D105): one of content.Approaches,
	// absent on translations, on the built-in template and on older texts.
	Approach  string         `json:"approach,omitempty"`
	Report    content.Report `json:"report"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type View struct {
	ID    int64  `json:"id"`
	Brief string `json:"brief"`
	// Name is the campaign's short name written by the model ("" when it has none).
	Name    string       `json:"name,omitempty"`
	Status  string       `json:"status"`
	Segment *SegmentView `json:"segment,omitempty"`
	// SourceLocale is the locale the text is written in first; Content is that
	// text. Locales are the campaign's locales: the source and every locale with
	// a reachable person. Translations holds their texts (and texts of locales
	// that left the audience). Pending lists the campaign's translations that
	// are missing or stale: while it is not empty the campaign cannot be
	// approved. Mismatches lists up-to-date translations that do not say what
	// the source says.
	SourceLocale string                  `json:"source_locale"`
	Locales      []string                `json:"locales"`
	Content      *ContentView            `json:"content,omitempty"`
	Translations map[string]*ContentView `json:"translations,omitempty"`
	Pending      []string                `json:"pending_locales"`
	Mismatches   []content.Issue         `json:"mismatches"`
	// TranslationFailures names the locales a translate request could not
	// translate, with the reason (only in the answer to that request).
	TranslationFailures map[string]string `json:"translation_failures,omitempty"`
	// Variants are the campaign's variants: ["A"], or ["A", "B"] once Variant B
	// was written. Content, Translations, Pending and Mismatches above are
	// Variant A's; VariantB holds the same for B (absent without B).
	// VariantCounts is how many people each variant reaches (with B only).
	Variants      []string                `json:"variants"`
	VariantB      *VariantView            `json:"variant_b,omitempty"`
	VariantCounts map[string]VariantCount `json:"variant_counts,omitempty"`
	// Delivery is present once the campaign has been approved.
	Delivery *DeliveryView `json:"delivery,omitempty"`
	// ScheduledAt is when a scheduled campaign starts (set from approval on).
	// ScheduleError says why a scheduled campaign could not start.
	ScheduledAt   *time.Time `json:"scheduled_at,omitempty"`
	ScheduleError string     `json:"schedule_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// VariantView is one variant's text, like the top-level fields of View for
// Variant A. Duplicate is true when it is the same text as Variant A (the
// campaign cannot be approved then).
type VariantView struct {
	Content      *ContentView            `json:"content,omitempty"`
	Translations map[string]*ContentView `json:"translations,omitempty"`
	Pending      []string                `json:"pending_locales"`
	Mismatches   []content.Issue         `json:"mismatches"`
	Duplicate    bool                    `json:"duplicate"`
}

// ---- create (brief -> segment -> audience) --------------------------------

// Create stores the brief, asks the LLM for a segment rule, validates it,
// compiles it to parameterized SQL and counts the audience.
func (s *Service) Create(ctx context.Context, brief string) (*View, error) {
	brief = strings.TrimSpace(brief)
	if n := utf8.RuneCountInString(brief); n < minBriefRunes || n > maxBriefRunes {
		return nil, &InputError{fmt.Sprintf("brief %d ile %d karakter arasında olmalı", minBriefRunes, maxBriefRunes)}
	}

	var id int64
	if err := s.pool.QueryRow(ctx, `INSERT INTO campaigns (brief) VALUES ($1) RETURNING id`, brief).Scan(&id); err != nil {
		return nil, fmt.Errorf("insert campaign: %w", err)
	}

	genCtx, cancel := context.WithTimeout(ctx, segmentTimeout)
	defer cancel()

	rule, err := briefs.ResolveSegmentWith(genCtx, s.llm, brief, llm.Hooks{OnLog: s.recorder(id, "segment", "", VariantA)}, s.segmentMode)
	if err != nil {
		return nil, s.generationError(err, "Kitle kuralı üretilemedi")
	}

	if rule.Status != segment.StatusOK {
		// The rule is stored too: a clarification can carry a one-click suggestion and
		// the offer that WAS understood, which the screen shows next to the question.
		ruleJSON, err := json.Marshal(rule)
		if err != nil {
			return nil, err
		}
		_, err = s.pool.Exec(ctx, `UPDATE campaigns SET segment_status=$2, explanation=$3, question=$4, segment_rule=$5, name=$6, updated_at=now() WHERE id=$1`,
			id, rule.Status, rule.Explanation, rule.Question, ruleJSON, rule.Name)
		if err != nil {
			return nil, fmt.Errorf("store segment status: %w", err)
		}
		return s.Get(ctx, id)
	}

	compiled, err := segment.Compile(rule, time.Now())
	if err != nil {
		return nil, fmt.Errorf("compile validated rule: %w", err) // a bug, not a user error
	}
	aud, err := segment.CountAudience(ctx, s.pool, compiled)
	if err != nil {
		return nil, err
	}
	counts, err := segment.CountByLocale(ctx, s.pool, compiled)
	if err != nil {
		return nil, err
	}
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return nil, err
	}
	ruleJSON, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE campaigns SET status='segment_ready', segment_status='ok', segment_rule=$2, explanation=$3, question='',
		       matched=$4, sms_reachable=$5, email_reachable=$6, both_reachable=$7, locale_counts=$8, name=$9, updated_at=now()
		WHERE id=$1`,
		id, ruleJSON, rule.Explanation, aud.Matched, aud.SMSReachable, aud.EmailReachable, aud.BothReachable, countsJSON, rule.Name)
	if err != nil {
		return nil, fmt.Errorf("store segment: %w", err)
	}
	return s.Get(ctx, id)
}

// ---- read -----------------------------------------------------------------

type campaignRow struct {
	id             int64
	brief          string
	name           string
	status         string
	segmentStatus  *string
	segmentRule    []byte
	explanation    string
	question       string
	matched        *int
	smsReachable   *int
	emailReachable *int
	bothReachable  *int
	localeCounts   []byte
	variantCounts  []byte
	scheduledAt    *time.Time
	scheduleError  string
	createdAt      time.Time
}

func (s *Service) loadRow(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id int64) (*campaignRow, error) {
	var r campaignRow
	err := q.QueryRow(ctx, `
		SELECT id, brief, name, status, segment_status, segment_rule, explanation, question,
		       matched, sms_reachable, email_reachable, both_reachable, locale_counts, variant_counts, scheduled_at, schedule_error, created_at
		FROM campaigns WHERE id=$1`, id).
		Scan(&r.id, &r.brief, &r.name, &r.status, &r.segmentStatus, &r.segmentRule, &r.explanation, &r.question,
			&r.matched, &r.smsReachable, &r.emailReachable, &r.bothReachable, &r.localeCounts, &r.variantCounts, &r.scheduledAt, &r.scheduleError, &r.createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load campaign: %w", err)
	}
	return &r, nil
}

// Get returns the campaign with its segment and content.
func (s *Service) Get(ctx context.Context, id int64) (*View, error) {
	r, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	v := &View{ID: r.id, Brief: r.brief, Name: r.name, Status: r.status, ScheduledAt: r.scheduledAt, ScheduleError: r.scheduleError, CreatedAt: r.createdAt}

	if r.segmentStatus != nil {
		sv := &SegmentView{Status: *r.segmentStatus, Conditions: []segment.Condition{}, Explanation: r.explanation, Question: r.question,
			Offer: offer.Offer{Kind: offer.KindNone}}
		if len(r.segmentRule) > 0 {
			var stored segment.Rule
			if err := json.Unmarshal(r.segmentRule, &stored); err != nil {
				return nil, fmt.Errorf("stored rule is corrupt: %w", err)
			}
			sv.Match, sv.Conditions, sv.Offer, sv.Suggestion, sv.Note = stored.Match, stored.Conditions, stored.Offer.Normalize(), stored.Suggestion, stored.Note
		}
		if r.matched != nil {
			sv.Audience = &segment.Audience{Matched: *r.matched, SMSReachable: *r.smsReachable,
				EmailReachable: *r.emailReachable, BothReachable: *r.bothReachable}
		}
		sv.Locales = rowCounts(r)
		v.Segment = sv
	}

	ls, err := s.loadLocales(ctx)
	if err != nil {
		return nil, err
	}
	srcLocale := ls.Source()
	v.SourceLocale = srcLocale.Code
	v.Locales = campaignLocales(ls, rowCounts(r))
	stored, err := loadContents(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	a := s.variantView(ls, r, v.Locales, stored[VariantA])
	v.Content, v.Translations, v.Pending, v.Mismatches = a.Content, a.Translations, a.Pending, a.Mismatches
	v.Variants = campaignVariants(stored, srcLocale.Code)
	if len(v.Variants) > 1 {
		b := s.variantView(ls, r, v.Locales, stored[VariantB])
		if a.Content != nil && b.Content != nil {
			b.Duplicate = sameText(a.Content.Content, b.Content.Content)
		}
		v.VariantB = &b
		if len(r.variantCounts) > 0 {
			_ = json.Unmarshal(r.variantCounts, &v.VariantCounts)
		}
	}
	switch r.status {
	case StatusApproved, StatusSending, StatusCompleted:
		d, err := s.deliveryProgress(ctx, id)
		if err != nil {
			return nil, err
		}
		v.Delivery = d
	}
	return v, nil
}

// variantView builds one variant's texts: the source, the translations (with
// staleness and what changed), the locales still waiting and the up-to-date
// translations that do not say what the source says.
func (s *Service) variantView(ls content.Locales, r *campaignRow, required []string, stored map[string]storedContent) VariantView {
	srcLocale := ls.Source()
	vv := VariantView{Mismatches: []content.Issue{}}
	src, hasSource := stored[srcLocale.Code]
	for _, l := range ls.All() {
		sc, ok := stored[l.Code]
		if !ok {
			continue
		}
		cv := &ContentView{Content: sc.Content, Locale: l.Code, Source: sc.Source, Version: sc.Version, Approach: sc.Approach,
			SourceVersion: sc.SourceVersion, UpdatedAt: sc.UpdatedAt}
		_, cv.Report = s.policy.Validate(cv.Content)
		if l.IsSource {
			cv.Report.Warnings = s.policy.Quality(cv.Content, rowOffer(r))
			vv.Content = cv
			continue
		}
		cv.Report.Warnings = []content.Issue{}
		if hasSource {
			cv.Stale = isStale(src, sc)
			if cv.Stale {
				cv.SourceText = sc.SourceText
			} else {
				vv.Mismatches = append(vv.Mismatches, content.ConsistentWith(srcLocale, src.Content, l, sc.Content)...)
			}
		}
		if vv.Translations == nil {
			vv.Translations = map[string]*ContentView{}
		}
		vv.Translations[l.Code] = cv
	}
	missing, stale := pendingLocales(ls, required, stored)
	vv.Pending = ls.Ordered(toSet(append(missing, stale...)))
	if vv.Pending == nil {
		vv.Pending = []string{}
	}
	return vv
}

// checkMessages applies the content rules to a source text: the limits and
// variables (Validate) and the offer (Quality). The returned problems are fed
// back to the model on a retry; the text comes back normalized.
func (s *Service) checkMessages(c content.Content, promised offer.Offer) (content.Content, []string) {
	norm, rep := s.policy.Validate(c)
	problems := rep.Messages()
	for _, q := range s.policy.Quality(norm, promised) {
		problems = append(problems, q.Message)
	}
	return norm, problems
}

// rowCounts is the stored number of reachable people per locale.
func rowCounts(r *campaignRow) map[string]int {
	counts := map[string]int{}
	if len(r.localeCounts) > 0 {
		_ = json.Unmarshal(r.localeCounts, &counts)
	}
	return counts
}

func toSet(codes []string) map[string]bool {
	out := map[string]bool{}
	for _, c := range codes {
		out[c] = true
	}
	return out
}

// rowOffer is the offer stored with the campaign's segment rule (none for
// campaigns created before offers existed).
func rowOffer(r *campaignRow) offer.Offer {
	if len(r.segmentRule) == 0 {
		return offer.Offer{Kind: offer.KindNone}
	}
	var stored segment.Rule
	if err := json.Unmarshal(r.segmentRule, &stored); err != nil {
		return offer.Offer{Kind: offer.KindNone}
	}
	return stored.Offer.Normalize()
}

// ---- audit & errors -------------------------------------------------------

// recorder writes every attempt (valid or not) to llm_generations. It must work
// even when the request context was cancelled, hence WithoutCancel.
// code is the locale written; "" for the segment step (stored as the source).
func (s *Service) recorder(campaignID int64, kind, code, variant string) func(llm.AttemptLog) {
	return func(a llm.AttemptLog) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), auditWriteBudget)
		defer cancel()

		raw := a.Raw
		if len(raw) > maxRawLogBytes {
			raw = raw[:maxRawLogBytes]
		}
		problems, _ := json.Marshal(append([]string{}, a.Problems...))
		errText := ""
		if a.Err != nil {
			errText = a.Err.Error()
		}
		_, err := s.pool.Exec(ctx, `
			INSERT INTO llm_generations (campaign_id, kind, attempt, mode, model, valid, problems, raw_output,
			                             input_tokens, output_tokens, latency_ms, error, refused, locale, variant)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
			        coalesce(nullif($14, ''), (SELECT code FROM locales WHERE is_source)), $15)`,
			campaignID, kind, a.N, s.llm.Mode(), s.llm.Model(), a.Valid, problems, strings.ToValidUTF8(raw, ""),
			a.Usage.InputTokens, a.Usage.OutputTokens, a.Latency.Milliseconds(), errText, isRefusal(a.Err), code, variant)
		if err != nil {
			s.log.Error("could not record LLM attempt", "campaign_id", campaignID, "error", err)
		}
	}
}

func isRefusal(err error) bool {
	var refusal *llm.RefusalError
	return errors.As(err, &refusal)
}

// markFallback flags the last recorded attempt of the message run that just
// ended in the built-in template, so the metrics count it exactly.
func (s *Service) markFallback(campaignID int64, code string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), auditWriteBudget)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE llm_generations SET fell_back = true
		WHERE id = (SELECT max(id) FROM llm_generations WHERE campaign_id = $1 AND kind = 'messages' AND locale = $2 AND variant = 'A')`, campaignID, code)
	if err != nil {
		s.log.Error("could not mark LLM fallback", "campaign_id", campaignID, "error", err)
	}
}

// generationError turns a failure of llm.Repair into an API-level error.
func (s *Service) generationError(err error, what string) error {
	var refusal *llm.RefusalError
	var auth *llm.AuthError
	switch {
	case errors.As(err, &auth):
		s.log.Error("LLM authentication failed", "error", err)
		return &GenerationError{Code: CodeLLMMisconfigured, Cause: err,
			Message: what + ": Claude API anahtarı geçersiz veya yetkisiz. ANTHROPIC_API_KEY değerini kontrol edin."}
	case errors.As(err, &refusal):
		return &GenerationError{Code: CodeLLMRefused, Cause: err,
			Message: what + ": model bu isteği yanıtlamayı reddetti. Brief'i değiştirip tekrar deneyin."}
	case errors.Is(err, context.DeadlineExceeded):
		return &GenerationError{Code: CodeLLMUnavailable, Cause: err, Message: what + ": model zamanında yanıt vermedi."}
	}
	if ex, ok := llm.IsExhausted(err); ok {
		ge := &GenerationError{Cause: err}
		for _, a := range ex.Attempts {
			ge.Attempts = append(ge.Attempts, a.Problems)
		}
		if ex.AllFailedWithErrors() {
			ge.Code, ge.Message = CodeLLMUnavailable, what+": model servisine ulaşılamadı. Biraz sonra tekrar deneyin."
		} else {
			ge.Code, ge.Message = CodeSegmentFailed,
				fmt.Sprintf("%s: model %d denemede geçerli bir çıktı veremedi. Brief'i daha net yazıp tekrar deneyin.", what, len(ex.Attempts))
		}
		return ge
	}
	return err
}
