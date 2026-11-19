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
	"smartcampaign/internal/segment"
)

// A campaign's text exists once per variant and locale. Each variant's source
// locale (tr-TR) is written first; every other locale is a translation of that
// variant's source. The source row counts its versions and each translation
// remembers the source version it was made from: a translation made from an
// older version is not up to date ("stale") and the campaign cannot be approved
// until it is translated again. A campaign is written in the source locale and
// in every locale that has at least one reachable person in its audience. Only
// the source is edited by a person: a translation is always the translator's
// output for the current source, so every locale says what the source says
// (D97). Variant B, when there is one, is a second text with the same offer
// that must not be Variant A again (D102).

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
	// Approach is the source text's approach (D105); "" on translations, on
	// the built-in template and on texts written before approaches existed.
	Approach string
}

// queryer is satisfied by *pgxpool.Pool and pgx.Tx.
type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// loadContents returns the stored text of a campaign by variant and locale
// code (stored["A"]["tr-TR"]). Variant A is always in the map, even before it
// has a text; B only when it has one.
func loadContents(ctx context.Context, q queryer, campaignID int64) (map[string]map[string]storedContent, error) {
	rows, err := q.Query(ctx, `
		SELECT variant, locale, sms_body, email_subject, email_body, source, version, coalesce(source_version, 0), source_text, updated_at,
		       coalesce(approach, '')
		FROM campaign_contents WHERE campaign_id=$1`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("load content: %w", err)
	}
	defer rows.Close()
	out := map[string]map[string]storedContent{VariantA: {}}
	for rows.Next() {
		var variant, code string
		var sc storedContent
		var srcText []byte
		if err := rows.Scan(&variant, &code, &sc.SMSBody, &sc.EmailSubject, &sc.EmailBody, &sc.Source, &sc.Version, &sc.SourceVersion, &srcText, &sc.UpdatedAt, &sc.Approach); err != nil {
			return nil, fmt.Errorf("load content: %w", err)
		}
		if len(srcText) > 0 {
			var c content.Content
			if err := json.Unmarshal(srcText, &c); err != nil {
				return nil, fmt.Errorf("load content: source text: %w", err)
			}
			sc.SourceText = &c
		}
		if out[variant] == nil {
			out[variant] = map[string]storedContent{}
		}
		out[variant][code] = sc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load content: %w", err)
	}
	return out, nil
}

// campaignVariants lists the variants that have a source text: always A, plus
// B when it was written.
func campaignVariants(stored map[string]map[string]storedContent, srcCode string) []string {
	out := []string{VariantA}
	if _, ok := stored[VariantB][srcCode]; ok {
		out = append(out, VariantB)
	}
	return out
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

// pendingLocales lists one variant's translations that are missing or stale.
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

// GenerateMessages writes Variant A (see GenerateVariant).
func (s *Service) GenerateMessages(ctx context.Context, id int64, ev Events) error {
	return s.GenerateVariant(ctx, id, VariantA, ev)
}

// GenerateVariant writes one variant's source text with the LLM, streaming it
// while it is produced, and then translates it into the campaign's other
// locales in parallel. The source text is parsed and validated; if it breaks a
// limit the model is asked to fix it, up to 3 attempts.
//
// Variant A: if it still fails, or the model is unavailable or refuses, a
// built-in source template that respects every limit is stored and reported
// through Events.Fallback.
//
// Variant B needs Variant A first and is written to differ from it: the model
// sees A's text, and an answer that repeats A is rejected like any other broken
// answer. B never falls back to the template (the template could be A's text
// again): a B that cannot be written is an error and nothing is stored.
//
// A translation that fails is reported through Events.Failed and keeps what it
// had (stale or missing).
func (s *Service) GenerateVariant(ctx context.Context, id int64, variant string, ev Events) error {
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
	promised := rowOffer(row)
	stored, err := loadContents(ctx, s.pool, id)
	if err != nil {
		return err
	}
	// What the text is checked against: whether the brief allows time pressure
	// or a stock claim, and the other variant (B always has A; A has B when B
	// was written first and A is written again).
	chk := genCheck{locale: src, deadline: briefs.HasDeadline(row.messageBrief(), promised.ExpiresAt), stock: briefs.MentionsStock(row.messageBrief())}
	if other, ok := stored[otherVariant(variant)][src.Code]; ok {
		chk.other, chk.otherApproach = &other.Content, other.Approach
	} else if variant == VariantB {
		return &StateError{"Varyant B için önce Varyant A'nın mesajları üretilmeli"}
	}
	ev = ev.serialized()

	genCtx, cancel := context.WithTimeout(ctx, messagesTimeout)
	defer cancel()

	g, err := s.generateSource(genCtx, id, row, promised, src, variant, chk, ev)
	c, approach := g.Content, g.approach
	source := SourceLLM
	reason := ""
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err() // the client went away: store nothing
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
		if variant == VariantB {
			return s.generationError(err, "Varyant B üretilemedi")
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
		c, approach, source = s.policy.Fallback(promised), "", SourceFallback
	}
	norm, rep := s.policy.Validate(c)
	if !rep.OK {
		return fmt.Errorf("source text violates the limits after validation (misconfigured budgets?): %v", rep.Messages())
	}
	version, err := s.writeSource(ctx, id, variant, norm, source, approach)
	if err != nil {
		return err
	}
	if variant == VariantB {
		if err := s.storeVariantCounts(ctx, id, row); err != nil {
			return err
		}
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
	s.translateAll(genCtx, id, row, promised, ls, variant, norm, version, targets, ev)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// storeVariantCounts counts how many people each variant reaches with the
// campaign's rule now, for the approval screen (recounted at approval).
func (s *Service) storeVariantCounts(ctx context.Context, id int64, row *campaignRow) error {
	rule, err := segment.Parse(string(row.segmentRule))
	if err != nil {
		return fmt.Errorf("stored rule is corrupt: %w", err)
	}
	compiled, err := segment.Compile(rule, time.Now())
	if err != nil {
		return fmt.Errorf("compile stored rule: %w", err)
	}
	counts, err := countByVariant(ctx, s.pool, compiled, id)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := lockEditable(ctx, tx, id, "FOR UPDATE", "varyant sayıları saklanmadı"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET variant_counts=$2, updated_at=now() WHERE id=$1`, id, raw); err != nil {
		return fmt.Errorf("store variant counts: %w", err)
	}
	return tx.Commit(ctx)
}

// genCheck is what a variant's source text is held to besides the limits and
// the offer: the brief's deadline and stock (no invented claims), and the other
// variant (another text and another approach, the same promise).
type genCheck struct {
	locale          content.Locale
	deadline, stock bool
	other           *content.Content
	otherApproach   string
}

// generated is a validated source text with its approach.
type generated struct {
	content.Content
	approach string
}

// generateSource runs the ask-validate-repair loop for one variant's source text.
func (s *Service) generateSource(ctx context.Context, id int64, row *campaignRow, promised offer.Offer, src content.Locale,
	variant string, chk genCheck, ev Events) (generated, error) {
	var scanner *llm.FieldStream
	return llm.Repair(ctx, maxAttempts,
		func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
			scanner = llm.NewFieldStream(content.FieldSMSBody, content.FieldEmailSubject, content.FieldEmailBody)
			return s.llm.Messages(ctx, llm.MessageRequest{
				Brief: row.messageBrief(), Explanation: row.explanation, Offer: promised, Policy: s.policy, Previous: prev,
				Locale: src, Theme: briefs.ThemeOf(row.messageBrief(), src.Lang()), Variant: variant,
				Avoid: chk.other, AvoidApproach: chk.otherApproach, Deadline: chk.deadline,
			}, deltas(scanner, src.Code, ev))
		},
		func(raw string) (generated, []string) { return s.validateVariant(raw, promised, chk) },
		hooks(s, id, "messages", src.Code, variant, ev),
	)
}

// validateVariant parses the model's JSON strictly (the three texts and the
// approach) and checks it: the limits and the offer (validateMessages), an
// approach from the list that the brief allows and the other variant does not
// use, no time pressure or stock claim the brief does not back, and, next to
// the other variant, another text with the same promise.
func (s *Service) validateVariant(raw string, promised offer.Offer, chk genCheck) (generated, []string) {
	c, approach, err := decodeMessage(raw)
	if err != nil {
		return generated{Content: c}, []string{err.Error()}
	}
	norm, problems := s.checkMessages(c, promised)
	if approach == nil {
		problems = append(problems, fmt.Sprintf("approach alanı eksik: şunlardan biri olmalı: %s", strings.Join(content.Approaches, ", ")))
		approach = new(string)
	} else if p := content.ApproachProblem(*approach, chk.otherApproach, chk.deadline); p != "" {
		problems = append(problems, p)
	}
	for _, i := range content.UnsupportedClaims(norm, chk.deadline, chk.stock) {
		problems = append(problems, i.Message)
	}
	if chk.other != nil {
		if sameText(norm, *chk.other) {
			problems = append(problems, DuplicateVariantProblem)
		}
		for _, i := range content.VariantFacts(chk.locale, *chk.other, norm) {
			problems = append(problems, i.Message)
		}
	}
	if len(problems) > 0 {
		return generated{Content: c, approach: *approach}, problems
	}
	return generated{Content: norm, approach: *approach}, nil
}

// decodeMessage reads exactly one JSON object with the three texts and the
// approach (nil when the field is missing: reported with the other problems).
func decodeMessage(raw string) (content.Content, *string, error) {
	var m struct {
		content.Content
		Approach *string `json:"approach"`
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m.Content, nil, fmt.Errorf("geçerli bir JSON nesnesi değil veya şemaya uymuyor (sms_body, email_subject, email_body, approach): %s", err.Error())
	}
	if dec.More() {
		return m.Content, nil, errors.New("JSON nesnesinden sonra fazladan içerik var")
	}
	return m.Content, m.Approach, nil
}

// DuplicateVariantProblem is the feedback to a model that wrote the other variant again.
const DuplicateVariantProblem = "Varyant B, Varyant A ile aynı: farklı bir açılış, farklı bir e-posta konusu ve farklı bir SMS cümlesi yaz (teklif ve rakamlar aynı kalsın)"

// translateAll translates one variant's source text into the targets, a few at a time.
func (s *Service) translateAll(ctx context.Context, id int64, row *campaignRow, promised offer.Offer, ls content.Locales,
	variant string, src content.Content, version int, targets []string, ev Events) {
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
			if err := s.translateOne(ctx, id, row, promised, ls.Source(), variant, src, version, target, ev); err != nil {
				s.log.Warn("translation failed", "campaign_id", id, "variant", variant, "locale", code, "error", err)
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
	srcLocale content.Locale, variant string, src content.Content, version int, target content.Locale, ev Events) error {
	var scanner *llm.FieldStream
	c, err := llm.Repair(ctx, maxAttempts,
		func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
			scanner = llm.NewFieldStream(content.FieldSMSBody, content.FieldEmailSubject, content.FieldEmailBody)
			return s.llm.Translate(ctx, llm.TranslateRequest{
				Source: src, SourceLocale: srcLocale, Target: target, Policy: s.policy, Previous: prev,
				Brief: row.messageBrief(), Offer: promised, Theme: briefs.ThemeOf(row.messageBrief(), srcLocale.Lang()), Variant: variant,
			}, deltas(scanner, target.Code, ev))
		},
		func(raw string) (content.Content, []string) {
			return s.validateTranslation(raw, srcLocale, src, target, promised)
		},
		hooks(s, id, "translate", target.Code, variant, ev),
	)
	if err != nil {
		return err
	}
	norm, rep := s.policy.Validate(c)
	if err := s.writeTranslation(ctx, id, variant, srcLocale.Code, target.Code, norm, version); err != nil {
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

func hooks(s *Service, id int64, kind, code, variant string, ev Events) llm.Hooks {
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
		OnLog: s.recorder(id, kind, code, variant),
	}
}

// validateTranslation parses a translation strictly and checks it like any
// text (limits, variables) plus that it says what the source says. The offer
// wording checks of the source text (Quality) are Turkish and are not applied:
// the comparison with the source covers the offer. Two things it does not
// cover are checked here (D112): the offer's coupon code must be in the SMS
// and in the email exactly (a code is typed at checkout; "SUMMER30" for
// "YAZ30" does not work), and the text must be a translation, not the source
// text again.
func (s *Service) validateTranslation(raw string, srcLocale content.Locale, src content.Content, target content.Locale,
	promised offer.Offer) (content.Content, []string) {
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
		// The same matching as the source text's (offer.Mentions).
		if code := promised.Normalize().Code; code != "" {
			for _, ch := range []struct{ label, text string }{
				{"SMS", norm.SMSBody},
				{"E-posta (konu veya metin)", norm.EmailSubject + "\n" + norm.EmailBody},
			} {
				if !strings.Contains(strings.ToUpper(ch.text), code) {
					problems = append(problems, fmt.Sprintf("%s: kupon kodu %s çeviride aynen geçmeli. Kodu çevirme, değiştirme, çıkarma; kaynaktaki gibi %s yaz.", ch.label, code, code))
				}
			}
		}
		if target.Code != srcLocale.Code && sameText(norm, src) {
			problems = append(problems, fmt.Sprintf("Çeviri, %s kaynak metniyle birebir aynı. Metni %s (%s) diline çevir.", srcLocale.Code, target.Name, target.Code))
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

// Translate translates Variant A again (see TranslateVariant).
func (s *Service) Translate(ctx context.Context, id int64, locales []string) (*View, error) {
	return s.TranslateVariant(ctx, id, VariantA, locales)
}

// TranslateVariant translates one variant's source text again into the given
// locales of the campaign (the "Yeniden çevir" and "Çevir ve göster" buttons).
// Only that variant's translations change. The returned view lists the locales
// whose translation failed (TranslationFailures).
func (s *Service) TranslateVariant(ctx context.Context, id int64, variant string, locales []string) (*View, error) {
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
	src, ok := stored[variant][ls.Source().Code]
	if !ok {
		if variant == VariantB {
			return nil, &StateError{"Varyant B yok; önce üretilmeli"}
		}
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
	s.translateAll(genCtx, id, row, rowOffer(row), ls, variant, src.Content, src.Version, targets, Events{
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

// SaveContent edits Variant A's source text.
func (s *Service) SaveContent(ctx context.Context, id int64, c content.Content) (*View, error) {
	ls, err := s.loadLocales(ctx)
	if err != nil {
		return nil, err
	}
	return s.SaveVariantContents(ctx, id, VariantA, map[string]content.Content{ls.Source().Code: c})
}

// SaveContents edits Variant A (see SaveVariantContents).
func (s *Service) SaveContents(ctx context.Context, id int64, edits map[string]content.Content) (*View, error) {
	return s.SaveVariantContents(ctx, id, VariantA, edits)
}

// SaveVariantContents stores a text edited by a person. Only the source locale
// can be edited: a translation is written by the translator alone (D97), so any
// other locale is refused. The text must satisfy every limit; invalid content is
// rejected, never truncated. A changed source gets a new version, which makes
// that variant's translations stale until they are translated again (the other
// variant's stay as they are). Variant B must exist already (it is written by
// the model first) and the two variants may not become the same text.
func (s *Service) SaveVariantContents(ctx context.Context, id int64, variant string, edits map[string]content.Content) (*View, error) {
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
	cur, has := stored[variant][srcLocale.Code]
	if variant == VariantB && !has {
		return nil, &StateError{"Varyant B yok; önce üretilmeli"}
	}
	if has && cur.Content == norm {
		return s.Get(ctx, id) // saving text unchanged does not turn model output into "edited"
	}
	if other, ok := stored[otherVariant(variant)][srcLocale.Code]; ok && sameText(norm, other.Content) {
		return nil, &DuplicateVariantError{}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := lockEditable(ctx, tx, id, "FOR UPDATE", "içerik düzenlenemez"); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO campaign_contents (campaign_id, variant, locale, sms_body, email_subject, email_body, source, version)
		VALUES ($1,$2,$3,$4,$5,$6,'edited',1)
		ON CONFLICT (campaign_id, variant, locale) DO UPDATE
		   SET sms_body=$4, email_subject=$5, email_body=$6, source='edited', version=campaign_contents.version+1, updated_at=now()`,
		id, variant, srcLocale.Code, norm.SMSBody, norm.EmailSubject, norm.EmailBody); err != nil {
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

// DeleteVariant removes Variant B with its translations: the campaign goes
// back to one text and everyone gets Variant A. Variant A cannot be removed.
func (s *Service) DeleteVariant(ctx context.Context, id int64, variant string) (*View, error) {
	if variant != VariantB {
		return nil, &InputError{"Varyant A silinemez; her kampanyanın bir metni olmalı"}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM campaigns WHERE id=$1 FOR UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load campaign: %w", err)
	}
	if status != StatusSegmentReady && status != StatusContentReady {
		return nil, &StateError{fmt.Sprintf("kampanya %q durumunda; varyant silinemez", status)}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM campaign_contents WHERE campaign_id=$1 AND variant='B'`, id); err != nil {
		return nil, fmt.Errorf("delete variant: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET variant_counts=NULL, updated_at=now() WHERE id=$1`, id); err != nil {
		return nil, fmt.Errorf("delete variant: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// writeSource stores one variant's source text and returns its version: a new
// version when the text changed (that variant's translations become stale), the
// same otherwise.
func (s *Service) writeSource(ctx context.Context, id int64, variant string, c content.Content, source, approach string) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := lockEditable(ctx, tx, id, "FOR UPDATE", "içerik değiştirilemez"); err != nil {
		return 0, err
	}
	ls, err := locale.Load(ctx, tx, s.policy)
	if err != nil {
		return 0, err
	}
	var version int
	err = tx.QueryRow(ctx, `
		INSERT INTO campaign_contents (campaign_id, variant, locale, sms_body, email_subject, email_body, source, version, approach)
		VALUES ($1,$2,$3,$4,$5,$6,$7,1,nullif($8, ''))
		ON CONFLICT (campaign_id, variant, locale) DO UPDATE
		   SET sms_body=$4, email_subject=$5, email_body=$6, source=$7, approach=nullif($8, ''), updated_at=now(),
		       version = campaign_contents.version + CASE
		           WHEN (campaign_contents.sms_body, campaign_contents.email_subject, campaign_contents.email_body) = ($4, $5, $6) THEN 0 ELSE 1 END
		RETURNING version`,
		id, variant, ls.Source().Code, c.SMSBody, c.EmailSubject, c.EmailBody, source, approach).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("store content: %w", err)
	}
	if err := advance(ctx, tx, id, ls.Source().Code); err != nil {
		return 0, err
	}
	return version, tx.Commit(ctx)
}

// writeTranslation stores one translation of a variant made from that
// variant's source version `version`. Nothing is stored when the source has
// moved on since (errSourceChanged).
func (s *Service) writeTranslation(ctx context.Context, id int64, variant, srcCode, code string, c content.Content, version int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err := lockEditable(ctx, tx, id, "FOR SHARE", "çeviri saklanmadı"); err != nil {
		return err
	}
	var current int
	err = tx.QueryRow(ctx, `
		SELECT version FROM campaign_contents
		WHERE campaign_id=$1 AND variant=$2 AND locale=$3 FOR SHARE`, id, variant, srcCode).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return errSourceChanged // the variant was removed meanwhile
	}
	if err != nil {
		return fmt.Errorf("read source version: %w", err)
	}
	if current != version {
		return errSourceChanged
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO campaign_contents (campaign_id, variant, locale, sms_body, email_subject, email_body, source, version, source_version, source_text)
		VALUES ($1,$2,$3,$4,$5,$6,'translated',1,$7,(SELECT jsonb_build_object('sms_body', s.sms_body, 'email_subject', s.email_subject, 'email_body', s.email_body)
		                                            FROM campaign_contents s WHERE s.campaign_id=$1 AND s.variant=$2 AND s.locale=$8))
		ON CONFLICT (campaign_id, variant, locale) DO UPDATE
		   SET sms_body=$4, email_subject=$5, email_body=$6, source='translated', source_version=$7,
		       source_text=EXCLUDED.source_text, version=campaign_contents.version+1, updated_at=now()`,
		id, variant, code, c.SMSBody, c.EmailSubject, c.EmailBody, version, srcCode)
	if err != nil {
		return fmt.Errorf("store translation: %w", err)
	}
	return tx.Commit(ctx)
}

// lockEditable locks the campaign row (lock is "FOR UPDATE" or "FOR SHARE") and
// refuses unless its text may still change. Every write of a campaign's text
// takes it first, in the same transaction: the approval updates that row, so a
// write either commits before the approval (which then sees it) or waits for it
// and is refused; it never lands on an approved campaign. The campaign row is
// always locked before its campaign_contents rows. Translations take FOR SHARE,
// so parallel translations do not wait for each other.
func lockEditable(ctx context.Context, tx pgx.Tx, id int64, lock, what string) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM campaigns WHERE id=$1 `+lock, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock campaign: %w", err)
	}
	if status != StatusSegmentReady && status != StatusContentReady {
		return &StateError{fmt.Sprintf("kampanya %q durumunda; %s", status, what)}
	}
	return nil
}

// advance moves the campaign from segment_ready to content_ready once Variant A
// has a source text (whether every translation is ready is checked at approval).
func advance(ctx context.Context, tx pgx.Tx, id int64, srcCode string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE campaigns SET status='content_ready', updated_at=now()
		WHERE id=$1 AND status='segment_ready'
		  AND EXISTS (SELECT 1 FROM campaign_contents WHERE campaign_id=$1 AND variant='A' AND locale=$2)`, id, srcCode); err != nil {
		return fmt.Errorf("advance campaign: %w", err)
	}
	return nil
}
