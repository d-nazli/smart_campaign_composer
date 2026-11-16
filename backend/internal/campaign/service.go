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
	messagesTimeout  = 180 * time.Second // the reference language first, then the others
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
	SourceLLM      = "llm"
	SourceFallback = "fallback"
	SourceEdited   = "edited"
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
}

func NewService(pool *pgxpool.Pool, client llm.Client, policy content.Policy, log *slog.Logger) *Service {
	return &Service{pool: pool, llm: client, policy: policy, log: log}
}

// Policy returns the content policy (limits/budgets) in force.
func (s *Service) Policy() content.Policy { return s.policy }

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
	// Languages is how many reachable people speak each language (for example
	// {"tr": 120, "en": 45}); it is recounted when the audience is built.
	Languages map[string]int `json:"languages,omitempty"`
}

type ContentView struct {
	content.Content
	Language  string         `json:"language"`
	Source    string         `json:"source"`
	Report    content.Report `json:"report"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type View struct {
	ID      int64        `json:"id"`
	Brief   string       `json:"brief"`
	Status  string       `json:"status"`
	Segment *SegmentView `json:"segment,omitempty"`
	// Content is the reference (default) language's text; Translations holds
	// the other languages, which must say the same thing (Mismatches lists
	// where they do not).
	Content      *ContentView            `json:"content,omitempty"`
	Translations map[string]*ContentView `json:"translations,omitempty"`
	Mismatches   []content.Issue         `json:"mismatches"`
	// Delivery is present once the campaign has been approved.
	Delivery *DeliveryView `json:"delivery,omitempty"`
	// ScheduledAt is when a scheduled campaign starts (set from approval on).
	// ScheduleError says why a scheduled campaign could not start.
	ScheduledAt   *time.Time `json:"scheduled_at,omitempty"`
	ScheduleError string     `json:"schedule_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
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

	rule, err := briefs.ResolveSegment(genCtx, s.llm, brief, llm.Hooks{OnLog: s.recorder(id, "segment", content.DefaultLanguage)})
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
		_, err = s.pool.Exec(ctx, `UPDATE campaigns SET segment_status=$2, explanation=$3, question=$4, segment_rule=$5, updated_at=now() WHERE id=$1`,
			id, rule.Status, rule.Explanation, rule.Question, ruleJSON)
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
	langs, err := segment.CountByLanguage(ctx, s.pool, compiled)
	if err != nil {
		return nil, err
	}
	langJSON, err := json.Marshal(langs)
	if err != nil {
		return nil, err
	}
	ruleJSON, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE campaigns SET status='segment_ready', segment_status='ok', segment_rule=$2, explanation=$3, question='',
		       matched=$4, sms_reachable=$5, email_reachable=$6, both_reachable=$7, language_counts=$8, updated_at=now()
		WHERE id=$1`,
		id, ruleJSON, rule.Explanation, aud.Matched, aud.SMSReachable, aud.EmailReachable, aud.BothReachable, langJSON)
	if err != nil {
		return nil, fmt.Errorf("store segment: %w", err)
	}
	return s.Get(ctx, id)
}

// ---- read -----------------------------------------------------------------

type campaignRow struct {
	id             int64
	brief          string
	status         string
	segmentStatus  *string
	segmentRule    []byte
	explanation    string
	question       string
	matched        *int
	smsReachable   *int
	emailReachable *int
	bothReachable  *int
	languageCounts []byte
	scheduledAt    *time.Time
	scheduleError  string
	createdAt      time.Time
}

func (s *Service) loadRow(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id int64) (*campaignRow, error) {
	var r campaignRow
	err := q.QueryRow(ctx, `
		SELECT id, brief, status, segment_status, segment_rule, explanation, question,
		       matched, sms_reachable, email_reachable, both_reachable, language_counts, scheduled_at, schedule_error, created_at
		FROM campaigns WHERE id=$1`, id).
		Scan(&r.id, &r.brief, &r.status, &r.segmentStatus, &r.segmentRule, &r.explanation, &r.question,
			&r.matched, &r.smsReachable, &r.emailReachable, &r.bothReachable, &r.languageCounts, &r.scheduledAt, &r.scheduleError, &r.createdAt)
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
	v := &View{ID: r.id, Brief: r.brief, Status: r.status, ScheduledAt: r.scheduledAt, ScheduleError: r.scheduleError, CreatedAt: r.createdAt}

	if r.segmentStatus != nil {
		sv := &SegmentView{Status: *r.segmentStatus, Conditions: []segment.Condition{}, Explanation: r.explanation, Question: r.question,
			Offer: offer.Offer{Kind: offer.KindNone}}
		if len(r.segmentRule) > 0 {
			var stored segment.Rule
			if err := json.Unmarshal(r.segmentRule, &stored); err != nil {
				return nil, fmt.Errorf("stored rule is corrupt: %w", err)
			}
			sv.Match, sv.Conditions, sv.Offer, sv.Suggestion = stored.Match, stored.Conditions, stored.Offer.Normalize(), stored.Suggestion
		}
		if r.matched != nil {
			sv.Audience = &segment.Audience{Matched: *r.matched, SMSReachable: *r.smsReachable,
				EmailReachable: *r.emailReachable, BothReachable: *r.bothReachable}
		}
		if len(r.languageCounts) > 0 {
			_ = json.Unmarshal(r.languageCounts, &sv.Languages)
		}
		v.Segment = sv
	}

	stored, err := loadContents(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	v.Mismatches = []content.Issue{}
	set := map[string]content.Content{}
	for _, lang := range content.LanguageCodes() {
		sc, ok := stored[lang]
		if !ok {
			continue
		}
		cv := &ContentView{Content: sc.Content, Language: lang, Source: sc.Source, UpdatedAt: sc.UpdatedAt}
		_, cv.Report = s.policy.Validate(cv.Content)
		cv.Report.Warnings = s.policy.Quality(cv.Content, rowOffer(r))
		set[lang] = sc.Content
		if lang == content.DefaultLanguage {
			v.Content = cv
			continue
		}
		if v.Translations == nil {
			v.Translations = map[string]*ContentView{}
		}
		v.Translations[lang] = cv
	}
	v.Mismatches = append(v.Mismatches, content.Consistency(set)...)
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

// validateMessages parses the model's JSON strictly and applies the content
// rules. The returned problems are fed back to the model on a retry.
func (s *Service) validateMessages(raw string, promised offer.Offer) (content.Content, []string) {
	var c content.Content
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, []string{"geçerli bir JSON nesnesi değil veya şemaya uymuyor (sms_body, email_subject, email_body): " + err.Error()}
	}
	if dec.More() {
		return c, []string{"JSON nesnesinden sonra fazladan içerik var"}
	}
	norm, rep := s.policy.Validate(c)
	problems := rep.Messages()
	for _, q := range s.policy.Quality(norm, promised) {
		problems = append(problems, q.Message)
	}
	if len(problems) > 0 {
		return c, problems
	}
	return norm, nil
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
func (s *Service) recorder(campaignID int64, kind, lang string) func(llm.AttemptLog) {
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
			                             input_tokens, output_tokens, latency_ms, error, refused, language)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			campaignID, kind, a.N, s.llm.Mode(), s.llm.Model(), a.Valid, problems, strings.ToValidUTF8(raw, ""),
			a.Usage.InputTokens, a.Usage.OutputTokens, a.Latency.Milliseconds(), errText, isRefusal(a.Err), lang)
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
func (s *Service) markFallback(campaignID int64, lang string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), auditWriteBudget)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		UPDATE llm_generations SET fell_back = true
		WHERE id = (SELECT max(id) FROM llm_generations WHERE campaign_id = $1 AND kind = 'messages' AND language = $2)`, campaignID, lang)
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
