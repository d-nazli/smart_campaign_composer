package campaign_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"

	"github.com/jackc/pgx/v5/pgxpool"
)

// attemptsOf counts the recorded attempts of one locale's run.
func attemptsOf(t *testing.T, id int64, kind, code string, pool *pgxpool.Pool) (total, valid int) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT count(*), count(*) FILTER (WHERE valid) FROM llm_generations WHERE campaign_id=$1 AND kind=$2 AND locale=$3`,
		id, kind, code).Scan(&total, &valid)
	if err != nil {
		t.Fatal(err)
	}
	return
}

// saveSource stores an edited source text and translates it again, as a
// person does before approving (otherwise every translation is stale).
func saveSource(ctx context.Context, svc *campaign.Service, id int64, c content.Content) (*campaign.View, error) {
	v, err := svc.SaveContent(ctx, id, c)
	if err != nil || len(v.Pending) == 0 {
		return v, err
	}
	return svc.Translate(ctx, id, v.Pending)
}

func editAll(c content.Content, r *strings.Replacer) content.Content {
	return content.Content{SMSBody: r.Replace(c.SMSBody), EmailSubject: r.Replace(c.EmailSubject), EmailBody: r.Replace(c.EmailBody)}
}

// ---- writing -------------------------------------------------------------------

// The source is written first, then translated into every locale of the
// audience: the locales with at least one reachable person, and only those.
func TestTheSourceIsTranslatedIntoEveryLocaleOfTheAudience(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := offerCampaign(t, svc)
	ctx := context.Background()

	seen := map[string]int{}
	ev := campaign.Events{Validated: func(code string, _ content.Content, rep content.Report) {
		if !rep.OK {
			t.Errorf("%s: %+v", code, rep.Issues)
		}
		seen[code]++
	}}
	if err := svc.GenerateMessages(ctx, id, ev); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.SourceLocale != "tr-TR" || v.Locales[0] != "tr-TR" || v.Content.Locale != "tr-TR" || v.Content.Version != 1 {
		t.Fatalf("source: %+v %v", v.Content, v.Locales)
	}
	want := []string{"tr-TR"}
	for code, n := range v.Segment.Locales {
		if n > 0 && code != "tr-TR" {
			want = append(want, code)
		}
	}
	if len(v.Locales) != len(want) || len(v.Locales) < 6 {
		t.Fatalf("campaign locales %v, audience %v", v.Locales, v.Segment.Locales)
	}
	for _, code := range v.Locales {
		if seen[code] != 1 {
			t.Errorf("validated events for %s: %d", code, seen[code])
		}
		if code == "tr-TR" {
			continue
		}
		tr := v.Translations[code]
		if tr == nil || tr.Source != campaign.SourceTranslated || tr.Stale || tr.SourceVersion != 1 || !tr.Report.OK {
			t.Errorf("%s: %+v", code, tr)
		}
		if total, valid := attemptsOf(t, id, "translate", code, pool); total != 1 || valid != 1 {
			t.Errorf("%s: audit rows %d/%d", code, total, valid)
		}
	}
	if len(v.Translations) != len(v.Locales)-1 || len(v.Pending) != 0 || len(v.Mismatches) != 0 {
		t.Fatalf("translations %d, pending %v, mismatches %+v", len(v.Translations), v.Pending, v.Mismatches)
	}
	if !strings.Contains(v.Translations["en-GB"].SMSBody, "15% off") || !strings.Contains(v.Translations["de-DE"].SMSBody, "15 % Rabatt") ||
		!strings.Contains(v.Translations["fr-FR"].SMSBody, "15 % de réduction") || !strings.Contains(v.Translations["ar-SA"].SMSBody, "خصم 15%") {
		t.Fatalf("en=%q de=%q fr=%q ar=%q", v.Translations["en-GB"].SMSBody, v.Translations["de-DE"].SMSBody, v.Translations["fr-FR"].SMSBody, v.Translations["ar-SA"].SMSBody)
	}
	for code, tr := range v.Translations {
		if all := tr.SMSBody + tr.EmailSubject + tr.EmailBody; strings.Contains(all, "indirim") || strings.Contains(all, "Merhaba") {
			t.Errorf("%s: Turkish in the translation: %q", code, all)
		}
	}
}

// Writing the same source text again is not a new version: nothing goes stale.
func TestRegeneratingTheSameSourceKeepsTheVersion(t *testing.T) {
	svc, id := generated(t)
	ctx := context.Background()
	if err := svc.GenerateMessages(ctx, id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(ctx, id)
	if v.Content.Version != 1 || len(v.Pending) != 0 {
		t.Fatalf("version %d, pending %v", v.Content.Version, v.Pending)
	}
}

// A translation is asked again, with the reason, when it drops {{first_name}}.
func TestATranslationThatDropsAPlaceholderIsRepaired(t *testing.T) {
	noName := translation("15% off is waiting for you!", "15% off", "Hello,\\n\\nEnjoy 15% off.")
	m := mock.NewScripted(nil, nil).WithTranslation("en-GB", mock.Step{Kind: mock.Raw, Raw: noName}, mock.Step{Kind: mock.Smart})
	svc, pool := newService(t, m)
	id := offerCampaign(t, svc)
	if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(context.Background(), id)
	if v.Translations["en-GB"].Source != campaign.SourceTranslated || !strings.Contains(v.Translations["en-GB"].SMSBody, "{{first_name}}") || len(v.Mismatches) != 0 {
		t.Fatalf("expected a repaired English text, got %+v / %+v", v.Translations["en-GB"], v.Mismatches)
	}
	if total, valid := attemptsOf(t, id, "translate", "en-GB", pool); total != 2 || valid != 1 {
		t.Fatalf("attempts: %d/%d, want 2/1", total, valid)
	}
	var calls []mock.Call
	for _, c := range m.Calls() {
		if c.Kind == "translate" && c.Locale == "en-GB" {
			calls = append(calls, c)
		}
	}
	feedback := strings.Join(calls[len(calls)-1].Previous[0].Problems, " ")
	if !strings.Contains(feedback, "{{first_name}}") || !strings.Contains(feedback, "değişkenler") {
		t.Fatalf("the feedback must name the placeholder: %s", feedback)
	}
}

func TestATranslationWithADifferentDiscountIsRepaired(t *testing.T) {
	wrong := translation("{{first_name}}, 20 % Rabatt wartet!", "20 % Rabatt", "Hallo {{first_name}},\\n\\n20 % Rabatt.")
	m := mock.NewScripted(nil, nil).WithTranslation("de-DE", mock.Step{Kind: mock.Raw, Raw: wrong}, mock.Step{Kind: mock.Smart})
	svc, pool := newService(t, m)
	id := offerCampaign(t, svc)
	if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(context.Background(), id)
	if strings.Contains(v.Translations["de-DE"].SMSBody, "20") || len(v.Mismatches) != 0 {
		t.Fatalf("a discount the source does not promise must not be stored: %+v", v.Translations["de-DE"])
	}
	if total, valid := attemptsOf(t, id, "translate", "de-DE", pool); total != 2 || valid != 1 {
		t.Fatalf("attempts: %d/%d", total, valid)
	}
}

// A translation that cannot be made is reported and left missing: it never
// gets a template or another locale's text, and the campaign waits for it.
func TestAFailingTranslationIsReportedAndBlocksApproval(t *testing.T) {
	over := `{"sms_body":"` + strings.Repeat("a", 170) + `","email_subject":"x","email_body":"y"}`
	m := mock.NewScripted(nil, nil).WithTranslation("it-IT", mock.Step{Kind: mock.Raw, Raw: over})
	svc, pool := newService(t, m)
	id := offerCampaign(t, svc)
	ctx := context.Background()

	failed := map[string]string{}
	if err := svc.GenerateMessages(ctx, id, campaign.Events{Failed: func(code, reason string) { failed[code] = reason }}); err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed["it-IT"] != campaign.TranslationInvalid {
		t.Fatalf("failed: %v", failed)
	}
	v, _ := svc.Get(ctx, id)
	if v.Content.Source != campaign.SourceLLM || v.Translations["it-IT"] != nil || !slices.Equal(v.Pending, []string{"it-IT"}) {
		t.Fatalf("source %s, it-IT %+v, pending %v", v.Content.Source, v.Translations["it-IT"], v.Pending)
	}
	_, err := svc.Approve(ctx, id)
	var st *campaign.StaleTranslationsError
	if !errors.As(err, &st) || !slices.Equal(st.Missing, []string{"it-IT"}) || len(st.Stale) != 0 {
		t.Fatalf("approve: %v", err)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 || campaignStatus(t, pool, id) != "content_ready" {
		t.Fatalf("a refused approval queued %d rows", total)
	}
}

// ---- the source changes ----------------------------------------------------------

// Editing the source makes every translation stale; the campaign cannot be
// approved until they are translated again.
func TestEditingTheSourceMakesTheTranslationsStale(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	ctx := context.Background()
	before, _ := svc.Get(ctx, id)

	edited := before.Content.Content
	edited.EmailBody += "\n\nGörüşmek üzere!"
	v, err := svc.SaveContent(ctx, id, edited)
	if err != nil {
		t.Fatal(err)
	}
	if v.Content.Version != 2 || v.Content.Source != campaign.SourceEdited || len(v.Pending) != len(v.Locales)-1 {
		t.Fatalf("version %d source %s pending %v", v.Content.Version, v.Content.Source, v.Pending)
	}
	for code, tr := range v.Translations {
		if !tr.Stale || tr.SourceVersion != 1 || tr.SourceText == nil || *tr.SourceText != before.Content.Content {
			t.Errorf("%s: the stale translation must show the source it was made from: %+v", code, tr)
		}
	}
	_, err = svc.Approve(ctx, id)
	var st *campaign.StaleTranslationsError
	if !errors.As(err, &st) || len(st.Stale) != len(v.Locales)-1 || len(st.Missing) != 0 {
		t.Fatalf("approve: %v", err)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 {
		t.Fatalf("queued %d", total)
	}

	again, err := svc.Translate(ctx, id, v.Pending)
	if err != nil || len(again.Pending) != 0 || len(again.TranslationFailures) != 0 {
		t.Fatalf("%v %+v", err, again)
	}
	// The added sentence is in every translation, in that language.
	if fr := again.Translations["fr-FR"]; fr.SourceVersion != 2 || fr.SourceText != nil || !strings.HasSuffix(fr.EmailBody, "À bientôt!") || strings.Contains(fr.EmailBody, "Görüşmek") ||
		!strings.HasSuffix(again.Translations["de-DE"].EmailBody, "Bis bald!") {
		t.Fatalf("fr-FR: %+v", again.Translations["fr-FR"])
	}
	if _, err := svc.Approve(ctx, id); err != nil {
		t.Fatalf("approve after translating: %v", err)
	}
}

// ---- only the source is edited (D97) -------------------------------------------------

// A translation is never written by hand: every other locale is refused and
// nothing is stored, even next to a valid source edit.
func TestOnlyTheSourceCanBeEdited(t *testing.T) {
	svc, id := generated(t)
	ctx := context.Background()
	before, _ := svc.Get(ctx, id)
	en := before.Translations["en-GB"].Content
	en.EmailBody += "\n\nSee you soon!"
	src := before.Content.Content
	src.EmailBody += "\n\nGörüşmek üzere!"
	for name, edits := range map[string]map[string]content.Content{
		"a translation":               {"en-GB": en},
		"an rtl translation":          {"ar-SA": before.Translations["ar-SA"].Content},
		"a translation next to tr-TR": {"tr-TR": src, "en-GB": en},
	} {
		_, err := svc.SaveContents(ctx, id, edits)
		var ro *campaign.ReadOnlyTranslationError
		if !errors.As(err, &ro) || ro.Source != "tr-TR" {
			t.Fatalf("%s: got %v", name, err)
		}
	}
	after, _ := svc.Get(ctx, id)
	if after.Content.Content != before.Content.Content || after.Translations["en-GB"].Content != before.Translations["en-GB"].Content {
		t.Fatal("a refused edit must leave the stored text untouched")
	}
}

// The database refuses it too: a translation row is only ever the translator's.
func TestTheDatabaseRefusesAHandWrittenTranslation(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	if _, err := pool.Exec(context.Background(), `UPDATE campaign_contents SET source='edited' WHERE campaign_id=$1 AND locale='en-GB'`, id); err == nil {
		t.Fatal("a translation marked as edited by hand must be refused")
	}
	if _, err := pool.Exec(context.Background(), `UPDATE campaign_contents SET source='translated' WHERE campaign_id=$1 AND locale='tr-TR'`, id); err == nil {
		t.Fatal("the source cannot be a translation")
	}
}

// "Yeniden çevir" works on an up-to-date translation too.
func TestAnUpToDateTranslationCanBeTranslatedAgain(t *testing.T) {
	svc, id := generated(t)
	ctx := context.Background()
	before, _ := svc.Get(ctx, id)
	v, err := svc.Translate(ctx, id, []string{"de-DE"})
	if err != nil || v.Translations["de-DE"].Source != campaign.SourceTranslated || v.Translations["de-DE"].Stale ||
		v.Translations["de-DE"].Version != before.Translations["de-DE"].Version+1 {
		t.Fatalf("%v %+v", err, v.Translations["de-DE"])
	}
}

func TestEditingTheSourceIsCheckedAgainstTheLimits(t *testing.T) {
	svc, id := generated(t)
	long := content.Content{SMSBody: strings.Repeat("a", 161), EmailSubject: "k", EmailBody: "b"}
	_, err := svc.SaveContents(context.Background(), id, map[string]content.Content{"tr-TR": long})
	var ic *campaign.InvalidContentError
	if !errors.As(err, &ic) || len(ic.Report.Issues) == 0 || ic.Report.Issues[0].Locale != "tr-TR" || ic.Report.Issues[0].Code != content.CodeTooLong {
		t.Fatalf("%v %+v", err, ic)
	}
}

func TestSavingAnUnknownLocaleIsRefused(t *testing.T) {
	svc, id := generated(t)
	for _, code := range []string{"fr", "fr-CA", "en_GB"} {
		_, err := svc.SaveContents(context.Background(), id, map[string]content.Content{code: {SMSBody: "a", EmailSubject: "b", EmailBody: "c"}})
		var in *campaign.InputError
		if !errors.As(err, &in) {
			t.Fatalf("%s: got %v", code, err)
		}
	}
}

func TestSavingTheUnchangedSourceKeepsItsSourceAndVersion(t *testing.T) {
	svc, id := generated(t)
	ctx := context.Background()
	v, _ := svc.Get(ctx, id)
	same, err := svc.SaveContent(ctx, id, v.Content.Content)
	if err != nil || same.Content.Source != campaign.SourceLLM || same.Content.Version != 1 || len(same.Pending) != 0 {
		t.Fatalf("%v %+v", err, same)
	}
}

func TestTranslatingALocaleOutsideTheCampaignIsRefused(t *testing.T) {
	svc, id := generated(t)
	var in *campaign.InputError
	for _, codes := range [][]string{{"tr-TR"}, {"xx-XX"}, {}} {
		if _, err := svc.Translate(context.Background(), id, codes); !errors.As(err, &in) {
			t.Fatalf("%v: %v", codes, err)
		}
	}
}

// ---- approval -----------------------------------------------------------------------

// A new locale is one row in the locales table. Once someone in the audience
// has it, the campaign needs its translation before it can be approved, and
// then the person gets their own text.
func TestANewLocaleInTheAudienceNeedsItsTranslationFirst(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO locales (code, name, native_name, fallbacks) VALUES
		('fr-CA', 'French (Canada)', 'Français (Canada)', '{"first_name": "cher client", "city": "votre région"}')`); err != nil {
		t.Fatal(err)
	}
	before, _ := svc.Get(ctx, id)
	// Everyone in the source locale moves to the new one.
	if _, err := pool.Exec(ctx, `UPDATE users SET locale='fr-CA' WHERE locale='tr-TR'`); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Approve(ctx, id)
	var st *campaign.StaleTranslationsError
	if !errors.As(err, &st) || !slices.Equal(st.Missing, []string{"fr-CA"}) {
		t.Fatalf("approve: %v (locales before %v)", err, before.Locales)
	}
	v, _ := svc.Get(ctx, id)
	if !slices.Contains(v.Locales, "fr-CA") || !slices.Equal(v.Pending, []string{"fr-CA"}) {
		t.Fatalf("the screen must show the new locale as missing: %v %v", v.Locales, v.Pending)
	}
	if _, err := svc.Translate(ctx, id, []string{"fr-CA"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Approve(ctx, id)
	if err != nil || res.Locales["fr-CA"] == 0 || res.Locales["tr-TR"] != 0 {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestApprovalRefusesATranslationThatDisagreesWithTheSource(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	id := approvableCampaign(t, svc)
	if _, err := pool.Exec(ctx, `UPDATE campaign_contents SET sms_body = replace(sms_body, '15%', '99%') WHERE campaign_id=$1 AND locale='en-GB'`, id); err != nil {
		t.Fatal(err)
	}
	var ic *campaign.InvalidContentError
	if _, err := svc.Approve(ctx, id); !errors.As(err, &ic) || ic.Report.Issues[0].Code != content.CodeLocaleMismatch {
		t.Fatalf("disagreeing locales: %v", err)
	}
	if _, _, total := deliveryCounts(t, pool, id); total != 0 {
		t.Fatalf("a refused approval queued %d rows", total)
	}
}

func TestApprovalReportsHowManyPeopleGetEachLocale(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	ctx := context.Background()

	res, err := svc.Approve(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	people := res.Audience.SMSReachable + res.Audience.EmailReachable - res.Audience.BothReachable
	sum := 0
	for _, n := range res.Locales {
		sum += n
	}
	if sum != people || len(res.Locales) < 6 {
		t.Fatalf("locales add up to %d, but %d people are reachable: %v", sum, people, res.Locales)
	}
	rows, err := pool.Query(ctx, `SELECT u.locale, count(DISTINCT d.user_id) FROM deliveries d JOIN users u ON u.id = d.user_id WHERE d.campaign_id=$1 GROUP BY 1`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var n int
		if err := rows.Scan(&code, &n); err != nil {
			t.Fatal(err)
		}
		if res.Locales[code] != n {
			t.Errorf("%s: approval says %d, the queue holds %d", code, res.Locales[code], n)
		}
	}
}

func generated(t *testing.T) (*campaign.Service, int64) {
	t.Helper()
	svc, _ := newService(t, mock.New(0))
	id := approvableCampaign(t, svc)
	return svc, id
}
