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
	messagesTimeout  = 120 * time.Second
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
}

type ContentView struct {
	content.Content
	Source    string         `json:"source"`
	Report    content.Report `json:"report"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type View struct {
	ID        int64        `json:"id"`
	Brief     string       `json:"brief"`
	Status    string       `json:"status"`
	Segment   *SegmentView `json:"segment,omitempty"`
	Content   *ContentView `json:"content,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
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

	rule, err := briefs.ResolveSegment(genCtx, s.llm, brief, llm.Hooks{OnLog: s.recorder(id, "segment")})
	if err != nil {
		return nil, s.generationError(err, "Kitle kuralı üretilemedi")
	}

	if rule.Status != segment.StatusOK {
		_, err := s.pool.Exec(ctx, `UPDATE campaigns SET segment_status=$2, explanation=$3, question=$4, updated_at=now() WHERE id=$1`,
			id, rule.Status, rule.Explanation, rule.Question)
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
	ruleJSON, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE campaigns SET status='segment_ready', segment_status='ok', segment_rule=$2, explanation=$3, question='',
		       matched=$4, sms_reachable=$5, email_reachable=$6, both_reachable=$7, updated_at=now()
		WHERE id=$1`,
		id, ruleJSON, rule.Explanation, aud.Matched, aud.SMSReachable, aud.EmailReachable, aud.BothReachable)
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
	createdAt      time.Time
}

func (s *Service) loadRow(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id int64) (*campaignRow, error) {
	var r campaignRow
	err := q.QueryRow(ctx, `
		SELECT id, brief, status, segment_status, segment_rule, explanation, question,
		       matched, sms_reachable, email_reachable, both_reachable, created_at
		FROM campaigns WHERE id=$1`, id).
		Scan(&r.id, &r.brief, &r.status, &r.segmentStatus, &r.segmentRule, &r.explanation, &r.question,
			&r.matched, &r.smsReachable, &r.emailReachable, &r.bothReachable, &r.createdAt)
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
	v := &View{ID: r.id, Brief: r.brief, Status: r.status, CreatedAt: r.createdAt}

	if r.segmentStatus != nil {
		sv := &SegmentView{Status: *r.segmentStatus, Conditions: []segment.Condition{}, Explanation: r.explanation, Question: r.question,
			Offer: offer.Offer{Kind: offer.KindNone}}
		if len(r.segmentRule) > 0 {
			var stored segment.Rule
			if err := json.Unmarshal(r.segmentRule, &stored); err != nil {
				return nil, fmt.Errorf("stored rule is corrupt: %w", err)
			}
			sv.Match, sv.Conditions, sv.Offer = stored.Match, stored.Conditions, stored.Offer.Normalize()
		}
		if r.matched != nil {
			sv.Audience = &segment.Audience{Matched: *r.matched, SMSReachable: *r.smsReachable,
				EmailReachable: *r.emailReachable, BothReachable: *r.bothReachable}
		}
		v.Segment = sv
	}

	var cv ContentView
	err = s.pool.QueryRow(ctx, `SELECT sms_body, email_subject, email_body, source, updated_at FROM campaign_contents WHERE campaign_id=$1`, id).
		Scan(&cv.SMSBody, &cv.EmailSubject, &cv.EmailBody, &cv.Source, &cv.UpdatedAt)
	switch {
	case err == nil:
		_, cv.Report = s.policy.Validate(cv.Content)
		cv.Report.Warnings = s.policy.Quality(cv.Content, rowOffer(r))
		v.Content = &cv
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return nil, fmt.Errorf("load content: %w", err)
	}
	return v, nil
}

// ---- message generation (streaming) ---------------------------------------

// Events receives progress while messages are generated. All fields optional.
type Events struct {
	Attempt   func(n, max int)
	Delta     func(field, text string)
	Retry     func(next int, problems []string)
	Validated func(c content.Content, r content.Report)
	Fallback  func(reason string, c content.Content, r content.Report)
}

// GenerateMessages asks the LLM for the SMS/email, streaming the text while it
// is produced. The final text is parsed and validated; if it breaks a limit the
// model is asked to fix it (up to 3 attempts). If it still fails, or the model
// is unavailable or refuses, a built-in template that also respects every limit
// is stored instead and reported through Events.Fallback.
func (s *Service) GenerateMessages(ctx context.Context, id int64, ev Events) error {
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return err
	}
	if row.status != StatusSegmentReady && row.status != StatusContentReady {
		return &StateError{fmt.Sprintf("kampanya %q durumunda; önce geçerli bir kitle kuralı gerekli", row.status)}
	}

	genCtx, cancel := context.WithTimeout(ctx, messagesTimeout)
	defer cancel()

	promised := rowOffer(row)
	var scanner *llm.FieldStream
	got, err := llm.Repair(genCtx, maxAttempts,
		func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
			scanner = llm.NewFieldStream(content.FieldSMSBody, content.FieldEmailSubject, content.FieldEmailBody)
			return s.llm.Messages(ctx, llm.MessageRequest{
				Brief: row.brief, Explanation: row.explanation, Offer: promised, Policy: s.policy, Previous: prev,
			}, func(text string) {
				if ev.Delta == nil {
					return
				}
				for _, d := range scanner.Write(text) {
					ev.Delta(d.Field, d.Text)
				}
			})
		},
		func(raw string) (content.Content, []string) { return s.validateMessages(raw, promised) },
		llm.Hooks{
			OnAttempt: ev.Attempt,
			OnRetry:   ev.Retry,
			OnLog:     s.recorder(id, "messages"),
		},
	)

	if err == nil {
		norm, rep := s.policy.Validate(got)
		if err := s.saveContent(ctx, id, norm, SourceLLM); err != nil {
			return err
		}
		if ev.Validated != nil {
			ev.Validated(norm, rep)
		}
		return nil
	}

	// The client went away: store nothing.
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return ctx.Err()
	}

	reason := ""
	var refusal *llm.RefusalError
	var auth *llm.AuthError
	switch {
	case errors.As(err, &auth):
		return s.generationError(err, "Mesajlar üretilemedi") // misconfiguration: do not hide it behind a template
	case errors.As(err, &refusal):
		reason = FallbackRefused
	case errors.Is(err, context.DeadlineExceeded):
		reason = FallbackUnavailable
	default:
		reason = FallbackInvalidOutput
		if ex, ok := llm.IsExhausted(err); ok && ex.AllFailedWithErrors() {
			reason = FallbackUnavailable
		}
	}

	s.log.Warn("message generation failed; using fallback template", "campaign_id", id, "reason", reason, "error", err)
	fb, rep := s.policy.Validate(s.policy.Fallback(promised))
	if !rep.OK {
		return fmt.Errorf("built-in fallback violates the limits (misconfigured budgets?): %v", rep.Messages())
	}
	if err := s.saveContent(ctx, id, fb, SourceFallback); err != nil {
		return err
	}
	if ev.Fallback != nil {
		ev.Fallback(reason, fb, rep)
	}
	return nil
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

// ---- manual edit ----------------------------------------------------------

// SaveContent validates and stores content edited by a person. The same rules
// as for model output apply; invalid content is rejected, never truncated.
func (s *Service) SaveContent(ctx context.Context, id int64, c content.Content) (*View, error) {
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	if row.status != StatusSegmentReady && row.status != StatusContentReady {
		return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; içerik düzenlenemez", row.status)}
	}
	norm, rep := s.policy.Validate(c)
	if !rep.OK {
		return nil, &InvalidContentError{Report: rep}
	}

	source := SourceEdited
	var cur content.Content
	var curSource string
	err = s.pool.QueryRow(ctx, `SELECT sms_body, email_subject, email_body, source FROM campaign_contents WHERE campaign_id=$1`, id).
		Scan(&cur.SMSBody, &cur.EmailSubject, &cur.EmailBody, &curSource)
	if err == nil && cur == norm {
		source = curSource // saving without changes does not turn model output into "edited"
	}
	if err := s.saveContent(ctx, id, norm, source); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Service) saveContent(ctx context.Context, id int64, c content.Content, source string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO campaign_contents (campaign_id, sms_body, email_subject, email_body, source)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (campaign_id) DO UPDATE
		   SET sms_body=$2, email_subject=$3, email_body=$4, source=$5, updated_at=now()`,
		id, c.SMSBody, c.EmailSubject, c.EmailBody, source); err != nil {
		return fmt.Errorf("store content: %w", err)
	}
	// Only move forward from segment_ready; never touch later states.
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET status='content_ready', updated_at=now() WHERE id=$1 AND status='segment_ready'`, id); err != nil {
		return fmt.Errorf("advance campaign: %w", err)
	}
	return tx.Commit(ctx)
}

// ---- audit & errors -------------------------------------------------------

// recorder writes every attempt (valid or not) to llm_generations. It must work
// even when the request context was cancelled, hence WithoutCancel.
func (s *Service) recorder(campaignID int64, kind string) func(llm.AttemptLog) {
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
			                             input_tokens, output_tokens, latency_ms, error)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			campaignID, kind, a.N, s.llm.Mode(), s.llm.Model(), a.Valid, problems, strings.ToValidUTF8(raw, ""),
			a.Usage.InputTokens, a.Usage.OutputTokens, a.Latency.Milliseconds(), errText)
		if err != nil {
			s.log.Error("could not record LLM attempt", "campaign_id", campaignID, "error", err)
		}
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
