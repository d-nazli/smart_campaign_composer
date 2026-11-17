package campaign

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	briefs "smartcampaign/internal/brief"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/offer"
)

// A campaign's text exists once per language (content.Languages). The default
// language (Turkish) is written first and is the reference: the others are
// written next, in parallel, and must say the same thing (same discount,
// amounts, dates and placeholders; content.Consistency). Nothing is stored
// until every language is ready, so a campaign never has half its languages.

// storedContent is one language's row of campaign_contents.
type storedContent struct {
	content.Content
	Source    string
	UpdatedAt time.Time
}

// queryer is satisfied by *pgxpool.Pool and pgx.Tx.
type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// loadContents returns the stored text of a campaign by language code.
func loadContents(ctx context.Context, q queryer, campaignID int64) (map[string]storedContent, error) {
	rows, err := q.Query(ctx, `
		SELECT language, sms_body, email_subject, email_body, source, updated_at
		FROM campaign_contents WHERE campaign_id=$1`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("load content: %w", err)
	}
	defer rows.Close()
	out := map[string]storedContent{}
	for rows.Next() {
		var lang string
		var sc storedContent
		if err := rows.Scan(&lang, &sc.SMSBody, &sc.EmailSubject, &sc.EmailBody, &sc.Source, &sc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("load content: %w", err)
		}
		out[lang] = sc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load content: %w", err)
	}
	return out, nil
}

// requireAllLanguages fails unless every supported language has a text.
func requireAllLanguages(have map[string]content.Content) error {
	for _, l := range content.Languages {
		if _, ok := have[l.Code]; !ok {
			return &StateError{fmt.Sprintf("%s (%s) metni yok; önce mesajlar tüm diller için üretilmeli", l.Display, l.Code)}
		}
	}
	return nil
}

// ---- message generation (streaming) ---------------------------------------

// Events receives progress while messages are generated. All fields optional.
// Every callback says which language it is about; they run one at a time (never
// concurrently), even though the languages are written in parallel.
type Events struct {
	Attempt   func(lang string, n, max int)
	Delta     func(lang, field, text string)
	Retry     func(lang string, next int, problems []string)
	Validated func(lang string, c content.Content, r content.Report)
	Fallback  func(lang, reason string, c content.Content, r content.Report)
}

// serialized wraps every callback in one mutex.
func (e Events) serialized() Events {
	var mu sync.Mutex
	out := Events{}
	if e.Attempt != nil {
		out.Attempt = func(l string, n, m int) { mu.Lock(); defer mu.Unlock(); e.Attempt(l, n, m) }
	}
	if e.Delta != nil {
		out.Delta = func(l, f, t string) { mu.Lock(); defer mu.Unlock(); e.Delta(l, f, t) }
	}
	if e.Retry != nil {
		out.Retry = func(l string, n int, p []string) { mu.Lock(); defer mu.Unlock(); e.Retry(l, n, p) }
	}
	if e.Validated != nil {
		out.Validated = func(l string, c content.Content, r content.Report) {
			mu.Lock()
			defer mu.Unlock()
			e.Validated(l, c, r)
		}
	}
	if e.Fallback != nil {
		out.Fallback = func(l, reason string, c content.Content, r content.Report) {
			mu.Lock()
			defer mu.Unlock()
			e.Fallback(l, reason, c, r)
		}
	}
	return out
}

// GenerateMessages asks the LLM for the SMS/email in every language, streaming
// the text while it is produced. Each text is parsed and validated; if it breaks
// a limit (or, for a translation, does not match the reference language) the
// model is asked to fix it, up to 3 attempts per language. If any language still
// fails, or the model is unavailable or refuses, a built-in template that
// respects every limit is stored for ALL languages (so they stay consistent) and
// reported through Events.Fallback.
func (s *Service) GenerateMessages(ctx context.Context, id int64, ev Events) error {
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return err
	}
	if row.status != StatusSegmentReady && row.status != StatusContentReady {
		return &StateError{fmt.Sprintf("kampanya %q durumunda; önce geçerli bir kitle kuralı gerekli", row.status)}
	}
	ev = ev.serialized()

	genCtx, cancel := context.WithTimeout(ctx, messagesTimeout)
	defer cancel()
	promised := rowOffer(row)

	results := map[string]content.Content{}
	errs := map[string]error{}

	ref, err := s.generateLanguage(genCtx, id, row, promised, content.DefaultLanguage, nil, ev)
	if err != nil {
		errs[content.DefaultLanguage] = err
	} else {
		results[content.DefaultLanguage] = ref
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, l := range content.Languages {
			if l.Code == content.DefaultLanguage {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				c, err := s.generateLanguage(genCtx, id, row, promised, l.Code, &ref, ev)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs[l.Code] = err
				} else {
					results[l.Code] = c
				}
			}()
		}
		wg.Wait()
	}

	if len(errs) == 0 {
		set := map[string]content.Content{}
		reports := map[string]content.Report{}
		for code, c := range results {
			norm, rep := s.policy.Validate(c)
			set[code], reports[code] = norm, rep
		}
		if issues := content.Consistency(set); len(issues) > 0 {
			return fmt.Errorf("generated languages disagree after validation (a bug): %s", issues[0].Message)
		}
		if err := s.writeContents(ctx, id, set, func(string) string { return SourceLLM }); err != nil {
			return err
		}
		for _, l := range content.Languages {
			if ev.Validated != nil {
				ev.Validated(l.Code, set[l.Code], reports[l.Code])
			}
		}
		return nil
	}

	// The client went away: store nothing.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// The first failing language (in display order) decides why we fall back.
	var failed string
	var failure error
	for _, l := range content.Languages {
		if e, ok := errs[l.Code]; ok {
			failed, failure = l.Code, e
			break
		}
	}
	if errors.Is(failure, context.Canceled) {
		return failure
	}

	reason := ""
	var refusal *llm.RefusalError
	var auth *llm.AuthError
	switch {
	case errors.As(failure, &auth):
		return s.generationError(failure, "Mesajlar üretilemedi") // misconfiguration: do not hide it behind a template
	case errors.As(failure, &refusal):
		reason = FallbackRefused
	case errors.Is(failure, context.DeadlineExceeded):
		reason = FallbackUnavailable
	default:
		reason = FallbackInvalidOutput
		if ex, ok := llm.IsExhausted(failure); ok && ex.AllFailedWithErrors() {
			reason = FallbackUnavailable
		}
	}

	s.log.Warn("message generation failed; using fallback templates", "campaign_id", id, "language", failed, "reason", reason, "error", failure)
	fbSet := s.policy.FallbackSet(promised)
	reports := map[string]content.Report{}
	for code, c := range fbSet {
		norm, rep := s.policy.Validate(c)
		if !rep.OK {
			return fmt.Errorf("built-in fallback (%s) violates the limits (misconfigured budgets?): %v", code, rep.Messages())
		}
		fbSet[code], reports[code] = norm, rep
	}
	if issues := content.Consistency(fbSet); len(issues) > 0 {
		return fmt.Errorf("built-in fallbacks disagree (a bug): %s", issues[0].Message)
	}
	if err := s.writeContents(ctx, id, fbSet, func(string) string { return SourceFallback }); err != nil {
		return err
	}
	s.markFallback(id, failed)
	if ev.Fallback != nil {
		for _, l := range content.Languages {
			ev.Fallback(l.Code, reason, fbSet[l.Code], reports[l.Code])
		}
	}
	return nil
}

// generateLanguage runs the ask-validate-repair loop for one language. ref is
// the reference language's text a translation has to match (nil for the
// reference itself).
func (s *Service) generateLanguage(ctx context.Context, id int64, row *campaignRow, promised offer.Offer, lang string, ref *content.Content, ev Events) (content.Content, error) {
	var scanner *llm.FieldStream
	return llm.Repair(ctx, maxAttempts,
		func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
			scanner = llm.NewFieldStream(content.FieldSMSBody, content.FieldEmailSubject, content.FieldEmailBody)
			return s.llm.Messages(ctx, llm.MessageRequest{
				Brief: row.brief, Explanation: row.explanation, Offer: promised, Policy: s.policy, Previous: prev,
				Language: lang, Reference: ref, ReferenceLanguage: content.DefaultLanguage,
				Theme: briefs.ThemeOf(row.brief, lang),
			}, func(text string) {
				if ev.Delta == nil {
					return
				}
				for _, d := range scanner.Write(text) {
					ev.Delta(lang, d.Field, d.Text)
				}
			})
		},
		func(raw string) (content.Content, []string) {
			c, problems := s.validateMessages(raw, promised)
			if len(problems) == 0 && ref != nil {
				for _, i := range content.ConsistentWith(content.DefaultLanguage, *ref, lang, c) {
					problems = append(problems, i.Message)
				}
			}
			return c, problems
		},
		llm.Hooks{
			OnAttempt: func(n, max int) {
				if ev.Attempt != nil {
					ev.Attempt(lang, n, max)
				}
			},
			OnRetry: func(next int, problems []string) {
				if ev.Retry != nil {
					ev.Retry(lang, next, problems)
				}
			},
			OnLog: s.recorder(id, "messages", lang),
		},
	)
}

// ---- manual edit ----------------------------------------------------------

// SaveContent edits the reference (default) language's text. The other
// languages keep theirs, so an edit that changes what is promised is rejected
// until they say the same (use SaveContents to change several at once).
func (s *Service) SaveContent(ctx context.Context, id int64, c content.Content) (*View, error) {
	return s.SaveContents(ctx, id, map[string]content.Content{content.DefaultLanguage: c})
}

// SaveContents validates and stores texts edited by a person, for any subset of
// the languages. The same rules as for model output apply, and the whole set
// (edited and untouched languages together) must say the same thing; invalid
// content is rejected, never truncated.
func (s *Service) SaveContents(ctx context.Context, id int64, edits map[string]content.Content) (*View, error) {
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	if row.status != StatusSegmentReady && row.status != StatusContentReady {
		return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; içerik düzenlenemez", row.status)}
	}
	for code := range edits {
		if !content.IsSupported(code) {
			return nil, &InputError{fmt.Sprintf("desteklenmeyen dil %q; desteklenenler: %v", code, content.LanguageCodes())}
		}
	}

	stored, err := loadContents(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	merged := map[string]content.Content{}
	for code, sc := range stored {
		merged[code] = sc.Content
	}
	normalized := map[string]content.Content{}
	var issues []content.Issue
	for _, l := range content.Languages {
		c, ok := edits[l.Code]
		if !ok {
			continue
		}
		norm, rep := s.policy.Validate(c)
		if !rep.OK {
			for _, i := range rep.Issues {
				i.Language = l.Code
				issues = append(issues, i)
			}
			continue
		}
		normalized[l.Code], merged[l.Code] = norm, norm
	}
	if len(issues) > 0 {
		return nil, &InvalidContentError{Report: content.Report{OK: false, Issues: issues, Warnings: []content.Issue{}}}
	}
	if err := requireAllLanguages(merged); err != nil {
		return nil, err
	}
	if mismatches := content.Consistency(merged); len(mismatches) > 0 {
		return nil, &InvalidContentError{Report: content.Report{OK: false, Issues: mismatches, Warnings: []content.Issue{}}}
	}

	// Saving text unchanged does not turn model output into "edited".
	err = s.writeContents(ctx, id, normalized, func(code string) string {
		if cur, ok := stored[code]; ok && cur.Content == normalized[code] {
			return cur.Source
		}
		return SourceEdited
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// writeContents stores the given languages and, once the campaign has all of
// them, moves it forward from segment_ready. It is one transaction.
func (s *Service) writeContents(ctx context.Context, id int64, set map[string]content.Content, source func(lang string) string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	for _, l := range content.Languages {
		c, ok := set[l.Code]
		if !ok {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO campaign_contents (campaign_id, language, sms_body, email_subject, email_body, source)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (campaign_id, language) DO UPDATE
			   SET sms_body=$3, email_subject=$4, email_body=$5, source=$6, updated_at=now()`,
			id, l.Code, c.SMSBody, c.EmailSubject, c.EmailBody, source(l.Code)); err != nil {
			return fmt.Errorf("store content: %w", err)
		}
	}
	// Only move forward from segment_ready, and only when every language has a text.
	if _, err := tx.Exec(ctx, `
		UPDATE campaigns SET status='content_ready', updated_at=now()
		WHERE id=$1 AND status='segment_ready'
		  AND (SELECT count(*) FROM campaign_contents WHERE campaign_id=$1) >= $2`, id, len(content.Languages)); err != nil {
		return fmt.Errorf("advance campaign: %w", err)
	}
	return tx.Commit(ctx)
}
