package campaign_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"

	"github.com/jackc/pgx/v5/pgxpool"
)

// attemptsOf counts the recorded attempts of one language's run.
func attemptsOf(t *testing.T, id int64, lang string, pool *pgxpool.Pool) (total, valid int) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT count(*), count(*) FILTER (WHERE valid) FROM llm_generations WHERE campaign_id=$1 AND kind='messages' AND language=$2`,
		id, lang).Scan(&total, &valid)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestMessagesAreWrittenInEveryLanguageAndStoredTogether(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := offerCampaign(t, svc)

	seen := map[string]int{}
	ev := campaign.Events{Validated: func(lang string, _ content.Content, rep content.Report) {
		if !rep.OK {
			t.Errorf("%s: %+v", lang, rep.Issues)
		}
		seen[lang]++
	}}
	if err := svc.GenerateMessages(context.Background(), id, ev); err != nil {
		t.Fatal(err)
	}
	for _, code := range content.LanguageCodes() {
		if seen[code] != 1 {
			t.Errorf("validated events for %s: %d", code, seen[code])
		}
	}

	v, err := svc.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != campaign.StatusContentReady || v.Content == nil || v.Content.Language != "tr" || len(v.Translations) != 2 {
		t.Fatalf("view: status=%s content=%+v translations=%d", v.Status, v.Content, len(v.Translations))
	}
	if len(v.Mismatches) != 0 {
		t.Fatalf("mismatches: %+v", v.Mismatches)
	}
	if !strings.Contains(v.Translations["en"].SMSBody, "15% off") || !strings.Contains(v.Translations["de"].SMSBody, "15 % Rabatt") || !strings.Contains(v.Content.SMSBody, "%15 indirim") {
		t.Fatalf("each language must be written in that language: tr=%q en=%q de=%q", v.Content.SMSBody, v.Translations["en"].SMSBody, v.Translations["de"].SMSBody)
	}
	for code, cv := range v.Translations {
		if cv.Source != campaign.SourceLLM || !cv.Report.OK || cv.Language != code {
			t.Errorf("%s: %+v", code, cv)
		}
	}
	for _, code := range content.LanguageCodes() {
		if total, valid := attemptsOf(t, id, code, pool); total != 1 || valid != 1 {
			t.Errorf("%s: audit rows %d/%d", code, total, valid)
		}
	}
}

// A translation is asked again, with the reason, when it does not mirror the
// reference: here the English SMS and email lack the {{first_name}} the Turkish
// text uses. Its numbers are right, so only the consistency check can catch it.
func TestATranslationThatDropsAPlaceholderIsRepaired(t *testing.T) {
	noName := msgs("15% off is waiting for you!", "15% off", "Hello,\\n\\nEnjoy 15% off.")
	m := mock.NewScripted(nil, nil).WithMessages("en", mock.Step{Kind: mock.Raw, Raw: noName}, mock.Step{Kind: mock.Smart})
	svc, pool := newService(t, m)
	id := offerCampaign(t, svc)

	if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(context.Background(), id)
	if v.Content.Source != campaign.SourceLLM || v.Translations["en"].Source != campaign.SourceLLM || len(v.Mismatches) != 0 {
		t.Fatalf("expected a repaired English text, got %+v / %+v", v.Translations["en"], v.Mismatches)
	}
	if total, valid := attemptsOf(t, id, "en", pool); total != 2 || valid != 1 {
		t.Fatalf("English attempts: %d/%d, want 2/1", total, valid)
	}
	if total, _ := attemptsOf(t, id, "tr", pool); total != 1 {
		t.Fatalf("the reference must not be asked again: %d", total)
	}
	// The model was told what differed and was shown the reference.
	var enCalls []mock.Call
	for _, c := range m.Calls() {
		if c.Kind == "messages" && c.Language == "en" {
			enCalls = append(enCalls, c)
		}
	}
	if len(enCalls) != 2 || len(enCalls[1].Previous) != 1 {
		t.Fatalf("English calls: %+v", enCalls)
	}
	feedback := strings.Join(enCalls[1].Previous[0].Problems, " ")
	if !strings.Contains(feedback, "{{first_name}}") || !strings.Contains(feedback, "değişkenler") {
		t.Fatalf("the feedback must name the placeholder: %s", feedback)
	}
}

func TestATranslationWithADifferentDiscountIsRepaired(t *testing.T) {
	wrong := msgs("{{first_name}}, 20% off is waiting for you!", "20% off", "Hi {{first_name}},\\n\\nEnjoy 20% off.")
	m := mock.NewScripted(nil, nil).WithMessages("de", mock.Step{Kind: mock.Raw, Raw: wrong}, mock.Step{Kind: mock.Smart})
	svc, pool := newService(t, m)
	id := offerCampaign(t, svc)
	if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(context.Background(), id)
	if strings.Contains(v.Translations["de"].SMSBody, "20") || len(v.Mismatches) != 0 {
		t.Fatalf("a discount the campaign does not promise must not be stored: %+v", v.Translations["de"])
	}
	if total, valid := attemptsOf(t, id, "de", pool); total != 2 || valid != 1 {
		t.Fatalf("German attempts: %d/%d", total, valid)
	}
}

// If any language cannot be produced, ALL of them use the built-in text: a
// campaign never mixes model text and template text that might disagree.
func TestOneFailingLanguageMakesEveryLanguageFallBackTogether(t *testing.T) {
	bad := msgs("kısa ama teklif yok", "Konu", "Gövde")
	m := mock.NewScripted(nil, nil).WithMessages("en", mock.Step{Kind: mock.Raw, Raw: bad})
	svc, pool := newService(t, m)
	id := offerCampaign(t, svc)

	fallbacks := map[string]string{}
	ev := campaign.Events{
		Fallback: func(lang, reason string, _ content.Content, _ content.Report) { fallbacks[lang] = reason },
		Validated: func(lang string, _ content.Content, _ content.Report) {
			t.Errorf("no language may be 'validated': %s", lang)
		},
	}
	if err := svc.GenerateMessages(context.Background(), id, ev); err != nil {
		t.Fatal(err)
	}
	if len(fallbacks) != 3 || fallbacks["tr"] != campaign.FallbackInvalidOutput {
		t.Fatalf("fallback events: %v", fallbacks)
	}
	v, _ := svc.Get(context.Background(), id)
	if v.Status != campaign.StatusContentReady || v.Content.Source != campaign.SourceFallback ||
		v.Translations["en"].Source != campaign.SourceFallback || v.Translations["de"].Source != campaign.SourceFallback {
		t.Fatalf("sources: %+v", v)
	}
	if len(v.Mismatches) != 0 || !v.Translations["en"].Report.OK || len(v.Translations["en"].Report.Warnings) != 0 {
		t.Fatalf("the built-in texts must agree and carry the offer: %+v %+v", v.Mismatches, v.Translations["en"])
	}
	if !strings.Contains(v.Translations["de"].SMSBody, "15 % Rabatt") {
		t.Fatalf("German fallback: %q", v.Translations["de"].SMSBody)
	}
	// Exactly one attempt row is flagged as the fallback, on the language that failed.
	var flagged int
	var lang string
	if err := pool.QueryRow(context.Background(), `SELECT count(*), coalesce(max(language), '') FROM llm_generations WHERE campaign_id=$1 AND fell_back`, id).Scan(&flagged, &lang); err != nil {
		t.Fatal(err)
	}
	if flagged != 1 || lang != "en" {
		t.Fatalf("flagged %d rows, language %q", flagged, lang)
	}
}

// ---- editing -----------------------------------------------------------------

func generated(t *testing.T) (*campaign.Service, int64) {
	t.Helper()
	svc, _ := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	return svc, id
}

func TestEditingOneLanguageCannotMakeTheLanguagesDisagree(t *testing.T) {
	svc, id := generated(t)
	ctx := context.Background()
	before, _ := svc.Get(ctx, id)

	// English promises 25% while Turkish and German still say 15%.
	en := before.Translations["en"].Content
	en.SMSBody = strings.Replace(en.SMSBody, "15%", "25%", 1)
	en.EmailBody = strings.Replace(en.EmailBody, "15%", "25%", 1)
	en.EmailSubject = strings.Replace(en.EmailSubject, "15%", "25%", 1)
	_, err := svc.SaveContents(ctx, id, map[string]content.Content{"en": en})
	var ic *campaign.InvalidContentError
	if !errors.As(err, &ic) {
		t.Fatalf("want InvalidContentError, got %v", err)
	}
	if len(ic.Report.Issues) == 0 || ic.Report.Issues[0].Code != content.CodeLanguageMismatch || ic.Report.Issues[0].Language != "en" {
		t.Fatalf("issues: %+v", ic.Report.Issues)
	}

	// Editing only the reference language is no way around it either.
	tr := before.Content.Content
	tr.SMSBody = strings.Replace(tr.SMSBody, "%15", "%25", 1)
	if _, err := svc.SaveContent(ctx, id, tr); !errors.As(err, &ic) {
		t.Fatalf("SaveContent: %v", err)
	}

	after, _ := svc.Get(ctx, id)
	if after.Translations["en"].Content != before.Translations["en"].Content || after.Content.Content != before.Content.Content {
		t.Fatal("a rejected edit must leave the stored text untouched")
	}

	// Changing the promise in every language at once is fine; the person is
	// warned that it is no longer the brief's offer (the soft check), not blocked.
	all := map[string]content.Content{}
	for code, cv := range map[string]*campaign.ContentView{"tr": before.Content, "en": before.Translations["en"], "de": before.Translations["de"]} {
		c := cv.Content
		c.SMSBody = strings.NewReplacer("%15", "%25", "15%", "25%", "15 %", "25 %").Replace(c.SMSBody)
		c.EmailSubject = strings.NewReplacer("%15", "%25", "15%", "25%", "15 %", "25 %").Replace(c.EmailSubject)
		c.EmailBody = strings.NewReplacer("%15", "%25", "15%", "25%", "15 %", "25 %").Replace(c.EmailBody)
		all[code] = c
	}
	v, err := svc.SaveContents(ctx, id, all)
	if err != nil {
		t.Fatalf("a consistent edit of all languages must be accepted: %v", err)
	}
	if v.Content.Source != campaign.SourceEdited || v.Translations["de"].Source != campaign.SourceEdited || len(v.Mismatches) != 0 {
		t.Fatalf("%+v", v)
	}
}

func TestEditingChecksEachLanguageAgainstTheLimitsAndNamesIt(t *testing.T) {
	svc, id := generated(t)
	long := content.Content{SMSBody: strings.Repeat("a", 161), EmailSubject: "k", EmailBody: "b"}
	_, err := svc.SaveContents(context.Background(), id, map[string]content.Content{"de": long})
	var ic *campaign.InvalidContentError
	if !errors.As(err, &ic) || len(ic.Report.Issues) == 0 || ic.Report.Issues[0].Language != "de" || ic.Report.Issues[0].Code != content.CodeTooLong {
		t.Fatalf("%v %+v", err, ic)
	}
}

func TestSavingAnUnsupportedLanguageIsRefused(t *testing.T) {
	svc, id := generated(t)
	_, err := svc.SaveContents(context.Background(), id, map[string]content.Content{"fr": {SMSBody: "a", EmailSubject: "b", EmailBody: "c"}})
	var in *campaign.InputError
	if !errors.As(err, &in) {
		t.Fatalf("got %v", err)
	}
}

func TestSavingUnchangedTextKeepsEachLanguagesSource(t *testing.T) {
	svc, id := generated(t)
	ctx := context.Background()
	v, _ := svc.Get(ctx, id)
	same, err := svc.SaveContents(ctx, id, map[string]content.Content{"en": v.Translations["en"].Content, "de": v.Translations["de"].Content})
	if err != nil || same.Translations["en"].Source != campaign.SourceLLM || same.Translations["de"].Source != campaign.SourceLLM {
		t.Fatalf("%v %+v", err, same)
	}
}

// ---- approval -----------------------------------------------------------------

func TestApprovalNeedsEveryLanguageAndTheirAgreement(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()

	// A language is missing (for example an old campaign made before languages existed).
	id := approvableCampaign(t, svc)
	if _, err := pool.Exec(ctx, `DELETE FROM campaign_contents WHERE campaign_id=$1 AND language='de'`, id); err != nil {
		t.Fatal(err)
	}
	var se *campaign.StateError
	if _, err := svc.Approve(ctx, id); !errors.As(err, &se) || !strings.Contains(err.Error(), "Deutsch") {
		t.Fatalf("missing language: %v", err)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 {
		t.Fatalf("a refused approval queued %d rows", total)
	}
	if got := campaignStatus(t, pool, id); got != "content_ready" {
		t.Fatalf("status %s", got)
	}

	// The stored texts disagree (tampered with, or a bug elsewhere): never sent.
	id2 := approvableCampaign(t, svc)
	if _, err := pool.Exec(ctx, `UPDATE campaign_contents SET sms_body = replace(sms_body, '15%', '99%') WHERE campaign_id=$1 AND language='en'`, id2); err != nil {
		t.Fatal(err)
	}
	var ic *campaign.InvalidContentError
	if _, err := svc.Approve(ctx, id2); !errors.As(err, &ic) || ic.Report.Issues[0].Code != content.CodeLanguageMismatch {
		t.Fatalf("disagreeing languages: %v", err)
	}
	if _, _, total := deliveryCounts(t, pool, id2); total != 0 {
		t.Fatalf("a refused approval queued %d rows", total)
	}
}

func TestApprovalReportsHowManyPeopleGetEachLanguage(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	ctx := context.Background()

	before, _ := svc.Get(ctx, id)
	if len(before.Segment.Languages) == 0 {
		t.Fatalf("the segment step must report the languages of the audience: %+v", before.Segment)
	}
	res, err := svc.Approve(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	people := res.Audience.SMSReachable + res.Audience.EmailReachable - res.Audience.BothReachable
	sum := 0
	for lang, n := range res.Languages {
		if !content.IsSupported(lang) || n <= 0 {
			t.Errorf("language %q: %d", lang, n)
		}
		sum += n
	}
	if sum != people {
		t.Fatalf("languages add up to %d, but %d people are reachable: %v", sum, people, res.Languages)
	}
	if len(res.Languages) != 3 {
		t.Fatalf("the seeded audience speaks all three languages: %v", res.Languages)
	}
	// The same numbers come from the queue itself.
	rows, err := pool.Query(ctx, `SELECT u.language, count(DISTINCT d.user_id) FROM deliveries d JOIN users u ON u.id = d.user_id WHERE d.campaign_id=$1 GROUP BY 1`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var lang string
		var n int
		if err := rows.Scan(&lang, &n); err != nil {
			t.Fatal(err)
		}
		if res.Languages[lang] != n {
			t.Errorf("%s: approval says %d, the queue holds %d", lang, res.Languages[lang], n)
		}
	}
}
