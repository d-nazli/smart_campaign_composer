package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	briefs "smartcampaign/internal/brief"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/locale"
	"smartcampaign/internal/offer"
)

// A campaign's text exists once per locale. The source locale (tr-TR) is
// written first; every other locale is a translation of it. The source row
// counts its versions and each translation remembers the source version it was
// made from: a translation made from an older version is not up to date
// ("stale") and the campaign cannot be approved until it is translated again
// A campaign is written in the source locale and in every locale that has at
// least one reachable person in its audience. Only the source is edited by a
// person: a translation is always the translator's output for the current
// source, so every locale says what the source says (D97).

// maxParallelTranslations bounds the model calls running at once.
const maxParallelTranslations = 4

// storedContent is one locale's row of campaign_contents.
type storedContent struct {
	content.Content
	Source string
	// Version counts the changes of the source row. SourceVersion is the source
	// version a translation was made from (0 on the source row) and SourceText
	// that source text (nil on the source row).
	Version       int
	SourceVersion int
	SourceText    *content.Content
	UpdatedAt     time.Time
}

// queryer is satisfied by *pgxpool.Pool and pgx.Tx.
type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// loadContents returns the stored text of a campaign by locale code.
func loadContents(ctx context.Context, q queryer, campaignID int64) (map[string]storedContent, error) {
	rows, err := q.Query(ctx, `
		SELECT locale, sms_body, email_subject, email_body, source, version, coalesce(source_version, 0), source_text, updated_at
		FROM campaign_contents WHERE campaign_id=$1`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("load content: %w", err)
	}
	defer rows.Close()
	out := map[string]storedContent{}
	for rows.Next() {
		var code string
		var sc storedContent
		var srcText []byte
		if err := rows.Scan(&code, &sc.SMSBody, &sc.EmailSubject, &sc.EmailBody, &sc.Source, &sc.Version, &sc.SourceVersion, &srcText, &sc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("load content: %w", err)
		}
		if len(srcText) > 0 {
			var c content.Content
			if err := json.Unmarshal(srcText, &c); err != nil {
				return nil, fmt.Errorf("load content: source text: %w", err)
			}
			sc.SourceText = &c
		}
		out[code] = sc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load content: %w", err)
	}
	return out, nil
}

// campaignLocales is the source locale and every locale with at least one
// reachable person (counts), in the set's order.
func campaignLocales(ls content.Locales, counts map[string]int) []string {
	want := map[string]bool{ls.Source().Code: true}
	for code, n := range counts {
		if n > 0 {
			want[code] = true
		}
	}
	return ls.Ordered(want)
}

// isStale reports whether a translation was made from an older source version.
func isStale(src, tr storedContent) bool { return tr.SourceVersion < src.Version }

// pendingLocales lists the campaign's translations that are missing or stale.
func pendingLocales(ls content.Locales, required []string, stored map[string]storedContent) (missing, stale []string) {
	src, hasSource := stored[ls.Source().Code]
	for _, code := range required {
		if code == ls.Source().Code {
			continue
		}
		tr, ok := stored[code]
		switch {
		case !ok:
			missing = append(missing, code)
		case hasSource && isStale(src, tr):
			stale = append(stale, code)
		}
	}
	return missing, stale
}

func (s *Service) loadLocales(ctx context.Context) (content.Locales, error) {
	return locale.Load(ctx, s.pool, s.policy)
}

// ---- message generation (streaming) ---------------------------------------

// Events receives progress while messages are written. All fields optional.
// Every callback names its locale; they run one at a time (never
// concurrently), even though translations run in parallel.
type Events struct {
	Attempt   func(locale string, n, max int)
	Delta     func(locale, field, text string)
	Retry     func(locale string, next int, problems []string)
	Validated func(locale string, c content.Content, r content.Report)
	Fallback  func(locale, reason string, c content.Content, r content.Report)
	// Failed: a translation could not be made; the locale keeps what it had.
	Failed func(locale, reason string)
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
	if e.Failed != nil {
		out.Failed = func(l, reason string) { mu.Lock(); defer mu.Unlock(); e.Failed(l, reason) }
	}
	return out
}

// GenerateMessages writes the source text with the LLM, streaming it while it
// is produced, and then translates it into the campaign's other locales in
// parallel. The source text is parsed and validated; if it breaks a limit the
// model is asked to fix it, up to 3 attempts. If it still fails, or the model is
// unavailable or refuses, a built-in source template that respects every limit
// is stored and reported through Events.Fallback. A translation that fails is
// reported through Events.Failed and keeps what it had (stale or missing).
func (s *Service) GenerateMessages(ctx context.Context, id int64, ev Events) error {
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return err
	}
	if row.status != StatusSegmentReady && row.status != StatusContentReady {
		return &StateError{fmt.Sprintf("kampanya %q durumunda; önce geçerli bir kitle kuralı gerekli", row.status)}
	}
	ls, err := s.loadLocales(ctx)
	if err != nil {
		return err
	}
	src := ls.Source()
	ev = ev.serialized()

	genCtx, cancel := context.WithTimeout(ctx, messagesTimeout)
	defer cancel()
	promised := rowOffer(row)

	c, err := s.generateSource(genCtx, id, row, promised, src, ev)
	source := SourceLLM
	reason := ""
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err() // the client went away: store nothing
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
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
		s.log.Warn("message generation failed; using the fallback template", "campaign_id", id, "reason", reason, "error", err)
		c, source = s.policy.Fallback(promised), SourceFallback
	}
	norm, rep := s.policy.Validate(c)
	if !rep.OK {
		return fmt.Errorf("source text violates the limits after validation (misconfigured budgets?): %v", rep.Messages())
	}
	version, err := s.writeSource(ctx, id, norm, source)
	if err != nil {
		return err
	}
	if source == SourceFallback {
		s.markFallback(id, src.Code)
		if ev.Fallback != nil {
			ev.Fallback(src.Code, reason, norm, rep)
		}
	} else if ev.Validated != nil {
		ev.Validated(src.Code, norm, rep)
	}

	// Translate every other locale of the campaign.
	var targets []string
	for _, code := range campaignLocales(ls, rowCounts(row)) {
		if code != src.Code {
			targets = append(targets, code)
		}
	}
	s.translateAll(genCtx, id, row, promised, ls, norm, version, targets, ev)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// generateSource runs the ask-validate-repair loop for the source text.
func (s *Service) generateSource(ctx context.Context, id int64, row *campaignRow, promised offer.Offer, src content.Locale, ev Events) (content.Content, error) {
	var scanner *llm.FieldStream
	return llm.Repair(ctx, maxAttempts,
		func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
			scanner = llm.NewFieldStream(content.FieldSMSBody, content.FieldEmailSubject, content.FieldEmailBody)
			return s.llm.Messages(ctx, llm.MessageRequest{
				Brief: row.brief, Explanation: row.explanation, Offer: promised, Policy: s.policy, Previous: prev,
				Locale: src, Theme: briefs.ThemeOf(row.brief, src.Lang()),
			}, deltas(scanner, src.Code, ev))
		},
		func(raw string) (content.Content, []string) { return s.validateMessages(raw, promised) },
		hooks(s, id, "messages", src.Code, ev),
	)
}

// translateAll translates the source text into the targets, a few at a time.
func (s *Service) translateAll(ctx context.Context, id int64, row *campaignRow, promised offer.Offer, ls content.Locales,
	src content.Content, version int, targets []string, ev Events) {
	sem := make(chan struct{}, maxParallelTranslations)
	var wg sync.WaitGroup
	for _, code := range targets {
		target, ok := ls.Get(code)
		if !ok {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := s.translateOne(ctx, id, row, promised, ls.Source(), src, version, target, ev); err != nil {
				s.log.Warn("translation failed", "campaign_id", id, "locale", code, "error", err)
				if ev.Failed != nil {
					ev.Failed(code, translationFailure(err))
				}
			}
		}()
	}
	wg.Wait()
}

// translateOne runs the ask-validate-repair loop for one locale and stores the
// result, unless the source changed meanwhile.
func (s *Service) translateOne(ctx context.Context, id int64, row *campaignRow, promised offer.Offer,
	srcLocale content.Locale, src content.Content, version int, target content.Locale, ev Events) error {
	var scanner *llm.FieldStream
	c, err := llm.Repair(ctx, maxAttempts,
		func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
			scanner = llm.NewFieldStream(content.FieldSMSBody, content.FieldEmailSubject, content.FieldEmailBody)
			return s.llm.Translate(ctx, llm.TranslateRequest{
				Source: src, SourceLocale: srcLocale, Target: target, Policy: s.policy, Previous: prev,
				Brief: row.brief, Offer: promised, Theme: briefs.ThemeOf(row.brief, srcLocale.Lang()),
			}, deltas(scanner, target.Code, ev))
		},
		func(raw string) (content.Content, []string) {
			return s.validateTranslation(raw, srcLocale, src, target)
		},
		hooks(s, id, "translate", target.Code, ev),
	)
	if err != nil {
		return err
	}
	norm, rep := s.policy.Validate(c)
	if err := s.writeTranslation(ctx, id, srcLocale.Code, target.Code, norm, version); err != nil {
		return err
	}
	if ev.Validated != nil {
		ev.Validated(target.Code, norm, rep)
	}
	return nil
}

func deltas(scanner *llm.FieldStream, code string, ev Events) func(string) {
	return func(text string) {
		if ev.Delta == nil {
			return
		}
		for _, d := range scanner.Write(text) {
			ev.Delta(code, d.Field, d.Text)
		}
	}
}

func hooks(s *Service, id int64, kind, code string, ev Events) llm.Hooks {
	return llm.Hooks{
		OnAttempt: func(n, max int) {
			if ev.Attempt != nil {
				ev.Attempt(code, n, max)
			}
		},
		OnRetry: func(next int, problems []string) {
			if ev.Retry != nil {
				ev.Retry(code, next, problems)
			}
		},
		OnLog: s.recorder(id, kind, code),
	}
}

// validateTranslation parses a translation strictly and checks it like any
// text (limits, variables) plus that it says what the source says. The offer
// wording checks of the source text (Quality) are Turkish and are not applied:
// the comparison with the source covers the offer.
func (s *Service) validateTranslation(raw string, srcLocale content.Locale, src content.Content, target content.Locale) (content.Content, []string) {
	c, err := decodeStrict(raw)
	if err != nil {
		return c, []string{err.Error()}
	}
	norm, rep := s.policy.Validate(c)
	problems := rep.Messages()
	if rep.OK {
		for _, i := range content.ConsistentWith(srcLocale, src, target, norm) {
			problems = append(problems, i.Message)
		}
	}
	if len(problems) > 0 {
		return c, problems
	}
	return norm, nil
}

// decodeStrict reads exactly one JSON object with the three text fields.
func decodeStrict(raw string) (content.Content, error) {
	var c content.Content
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("geçerli bir JSON nesnesi değil veya şemaya uymuyor (sms_body, email_subject, email_body): %s", err.Error())
	}
	if dec.More() {
		return c, errors.New("JSON nesnesinden sonra fazladan içerik var")
	}
	return c, nil
}

// Translation failure reasons reported to the UI.
const (
	TranslationInvalid     = "invalid_output"
	TranslationUnavailable = "llm_unavailable"
	TranslationRefused     = "llm_refused"
	TranslationOutdated    = "source_changed"
)

var errSourceChanged = errors.New("the source text changed while translating")

func translationFailure(err error) string {
	var refusal *llm.RefusalError
	switch {
	case errors.Is(err, errSourceChanged):
		return TranslationOutdated
	case errors.As(err, &refusal):
		return TranslationRefused
	case errors.Is(err, context.DeadlineExceeded):
		return TranslationUnavailable
	}
	if ex, ok := llm.IsExhausted(err); ok && ex.AllFailedWithErrors() {
		return TranslationUnavailable
	}
	var auth *llm.AuthError
	if errors.As(err, &auth) {
		return TranslationUnavailable
	}
	return TranslationInvalid
}

// ---- translate on request -------------------------------------------------

// Translate translates the source text again into the given locales of the
// campaign (the "Yeniden çevir" and "Çevir ve göster" buttons). The returned
// view lists the locales whose translation failed (TranslationFailures).
func (s *Service) Translate(ctx context.Context, id int64, locales []string) (*View, error) {
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	if row.status != StatusSegmentReady && row.status != StatusContentReady {
		return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; çeviri yapılamaz", row.status)}
	}
	ls, err := s.loadLocales(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := loadContents(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	src, ok := stored[ls.Source().Code]
	if !ok {
		return nil, &StateError{"önce kaynak metin (" + ls.Source().Code + ") üretilmeli"}
	}
	if len(locales) == 0 {
		return nil, &InputError{"çevrilecek locale yok"}
	}
	inCampaign := map[string]bool{}
	for _, code := range campaignLocales(ls, rowCounts(row)) {
		inCampaign[code] = true
	}
	seen := map[string]bool{}
	var targets []string
	for _, code := range locales {
		if seen[code] {
			continue
		}
		seen[code] = true
		switch {
		case code == ls.Source().Code:
			return nil, &InputError{code + " kaynak locale; çevrilmez"}
		case !inCampaign[code]:
			return nil, &InputError{fmt.Sprintf("%q bu kampanyanın locale'leri arasında değil: %v", code, campaignLocales(ls, rowCounts(row)))}
		}
		targets = append(targets, code)
	}

	genCtx, cancel := context.WithTimeout(ctx, messagesTimeout)
	defer cancel()
	failures := map[string]string{}
	var mu sync.Mutex
	s.translateAll(genCtx, id, row, rowOffer(row), ls, src.Content, src.Version, targets, Events{
		Failed: func(code, reason string) { mu.Lock(); defer mu.Unlock(); failures[code] = reason },
	})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	v, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(failures) > 0 {
		v.TranslationFailures = failures
	}
	return v, nil
}

// ---- manual edit ----------------------------------------------------------

// SaveContent edits the source locale's text.
func (s *Service) SaveContent(ctx context.Context, id int64, c content.Content) (*View, error) {
	ls, err := s.loadLocales(ctx)
	if err != nil {
		return nil, err
	}
	return s.SaveContents(ctx, id, map[string]content.Content{ls.Source().Code: c})
}

// SaveContents stores a text edited by a person. Only the source locale can be
// edited: a translation is written by the translator alone (D97), so any other
// locale is refused. The text must satisfy every limit; invalid content is
// rejected, never truncated. A changed source gets a new version, which makes
// every translation stale until it is translated again.
func (s *Service) SaveContents(ctx context.Context, id int64, edits map[string]content.Content) (*View, error) {
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	if row.status != StatusSegmentReady && row.status != StatusContentReady {
		return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; içerik düzenlenemez", row.status)}
	}
	ls, err := s.loadLocales(ctx)
	if err != nil {
		return nil, err
	}
	srcLocale := ls.Source()
	for code := range edits {
		if _, ok := ls.Get(code); !ok {
			return nil, &InputError{fmt.Sprintf("bilinmeyen locale %q; tanımlı olanlar: %v", code, ls.Codes())}
		}
		if code != srcLocale.Code {
			return nil, &ReadOnlyTranslationError{Locale: code, Source: srcLocale.Code}
		}
	}
	c, ok := edits[srcLocale.Code]
	if !ok {
		return nil, &InputError{"kaydedilecek metin yok"}
	}
	norm, rep := s.policy.Validate(c)
	if !rep.OK {
		issues := []content.Issue{}
		for _, i := range rep.Issues {
			i.Locale = srcLocale.Code
			issues = append(issues, i)
		}
		return nil, &InvalidContentError{Report: content.Report{OK: false, Issues: issues, Warnings: []content.Issue{}}}
	}
	stored, err := loadContents(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	if cur, has := stored[srcLocale.Code]; has && cur.Content == norm {
		return s.Get(ctx, id) // saving text unchanged does not turn model output into "edited"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO campaign_contents (campaign_id, locale, sms_body, email_subject, email_body, source, version)
		VALUES ($1,$2,$3,$4,$5,'edited',1)
		ON CONFLICT (campaign_id, locale) DO UPDATE
		   SET sms_body=$3, email_subject=$4, email_body=$5, source='edited', version=campaign_contents.version+1, updated_at=now()`,
		id, srcLocale.Code, norm.SMSBody, norm.EmailSubject, norm.EmailBody); err != nil {
		return nil, fmt.Errorf("store content: %w", err)
	}
	if err := advance(ctx, tx, id, srcLocale.Code); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// writeSource stores the source text and returns its version: a new version
// when the text changed (every translation becomes stale), the same otherwise.
func (s *Service) writeSource(ctx context.Context, id int64, c content.Content, source string) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	ls, err := locale.Load(ctx, tx, s.policy)
	if err != nil {
		return 0, err
	}
	var version int
	err = tx.QueryRow(ctx, `
		INSERT INTO campaign_contents (campaign_id, locale, sms_body, email_subject, email_body, source, version)
		VALUES ($1,$2,$3,$4,$5,$6,1)
		ON CONFLICT (campaign_id, locale) DO UPDATE
		   SET sms_body=$3, email_subject=$4, email_body=$5, source=$6, updated_at=now(),
		       version = campaign_contents.version + CASE
		           WHEN (campaign_contents.sms_body, campaign_contents.email_subject, campaign_contents.email_body) = ($3, $4, $5) THEN 0 ELSE 1 END
		RETURNING version`,
		id, ls.Source().Code, c.SMSBody, c.EmailSubject, c.EmailBody, source).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("store content: %w", err)
	}
	if err := advance(ctx, tx, id, ls.Source().Code); err != nil {
		return 0, err
	}
	return version, tx.Commit(ctx)
}

// writeTranslation stores one translation made from source version `version`.
// Nothing is stored when the source has moved on since (errSourceChanged).
func (s *Service) writeTranslation(ctx context.Context, id int64, srcCode, code string, c content.Content, version int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var current int
	var status string
	err = tx.QueryRow(ctx, `
		SELECT cc.version, c.status FROM campaign_contents cc JOIN campaigns c ON c.id = cc.campaign_id
		WHERE cc.campaign_id=$1 AND cc.locale=$2 FOR SHARE OF cc`, id, srcCode).Scan(&current, &status)
	if err != nil {
		return fmt.Errorf("read source version: %w", err)
	}
	if status != StatusSegmentReady && status != StatusContentReady {
		return &StateError{fmt.Sprintf("kampanya %q durumunda; çeviri saklanmadı", status)}
	}
	if current != version {
		return errSourceChanged
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO campaign_contents (campaign_id, locale, sms_body, email_subject, email_body, source, version, source_version, source_text)
		VALUES ($1,$2,$3,$4,$5,'translated',1,$6,(SELECT jsonb_build_object('sms_body', s.sms_body, 'email_subject', s.email_subject, 'email_body', s.email_body) FROM campaign_contents s WHERE s.campaign_id=$1 AND s.locale=$7))
		ON CONFLICT (campaign_id, locale) DO UPDATE
		   SET sms_body=$3, email_subject=$4, email_body=$5, source='translated', source_version=$6,
		       source_text=EXCLUDED.source_text, version=campaign_contents.version+1, updated_at=now()`,
		id, code, c.SMSBody, c.EmailSubject, c.EmailBody, version, srcCode)
	if err != nil {
		return fmt.Errorf("store translation: %w", err)
	}
	return tx.Commit(ctx)
}

// advance moves the campaign from segment_ready to content_ready once it has a
// source text (whether every translation is ready is checked at approval).
func advance(ctx context.Context, tx pgx.Tx, id int64, srcCode string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE campaigns SET status='content_ready', updated_at=now()
		WHERE id=$1 AND status='segment_ready'
		  AND EXISTS (SELECT 1 FROM campaign_contents WHERE campaign_id=$1 AND locale=$2)`, id, srcCode); err != nil {
		return fmt.Errorf("advance campaign: %w", err)
	}
	return nil
}
