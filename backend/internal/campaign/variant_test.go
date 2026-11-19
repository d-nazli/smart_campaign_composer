package campaign_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/clock"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"
)

// abCampaign is a campaign with both variants written (and translated).
func abCampaign(t *testing.T, svc *campaign.Service) int64 {
	t.Helper()
	id := approvableCampaign(t, svc)
	if err := svc.GenerateVariant(context.Background(), id, campaign.VariantB, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	return id
}

// ---- writing Variant B ------------------------------------------------------------

// Variant B is written with Variant A in view: another text, the same offer,
// translated into the same locales.
func TestVariantBIsADifferentTextWithTheSameOffer(t *testing.T) {
	t.Parallel()
	m := mock.New(0)
	svc, pool := newService(t, m)
	id := abCampaign(t, svc)
	ctx := context.Background()

	v, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(v.Variants, []string{"A", "B"}) || v.VariantB == nil || v.VariantB.Content == nil {
		t.Fatalf("variants %v, B %+v", v.Variants, v.VariantB)
	}
	a, b := v.Content.Content, v.VariantB.Content.Content
	if a.SMSBody == b.SMSBody || a.EmailSubject == b.EmailSubject {
		t.Errorf("B repeats A:\nA %+v\nB %+v", a, b)
	}
	if v.VariantB.Duplicate {
		t.Error("B marked as a duplicate")
	}
	for _, c := range []content.Content{a, b} {
		if !strings.Contains(c.SMSBody, "%15") || !strings.Contains(c.SMSBody, "{{first_name}}") {
			t.Errorf("offer or name missing: %q", c.SMSBody)
		}
	}
	// Every locale of the campaign has B's translation, made from B's source.
	if len(v.VariantB.Pending) != 0 || len(v.Pending) != 0 {
		t.Fatalf("pending A %v B %v", v.Pending, v.VariantB.Pending)
	}
	for _, code := range v.Locales[1:] {
		bt, at := v.VariantB.Translations[code], v.Translations[code]
		if bt == nil || bt.Stale || bt.Source != campaign.SourceTranslated {
			t.Fatalf("%s: B translation %+v", code, bt)
		}
		if bt.SMSBody == at.SMSBody {
			t.Errorf("%s: B's translation is A's: %q", code, bt.SMSBody)
		}
	}
	// The model saw A while writing B; the attempts are audited as B.
	var sawA bool
	for _, c := range m.Calls() {
		if c.Kind == "messages" && c.Variant == "B" && c.Avoid != nil && *c.Avoid == a {
			sawA = true
		}
	}
	if !sawA {
		t.Error("Variant B was not written with Variant A in view")
	}
	var bRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM llm_generations WHERE campaign_id=$1 AND variant='B'`, id).Scan(&bRows); err != nil {
		t.Fatal(err)
	}
	if bRows < len(v.Locales) {
		t.Errorf("audited B attempts: %d, locales %d", bRows, len(v.Locales))
	}
	// The approval screen gets the people per variant.
	ca, cb := v.VariantCounts["A"], v.VariantCounts["B"]
	if ca.People == 0 || cb.People == 0 || ca.People+cb.People < v.Segment.Audience.SMSReachable {
		t.Errorf("variant counts %+v (audience %+v)", v.VariantCounts, v.Segment.Audience)
	}
}

func TestVariantBNeedsVariantAFirst(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	id := offerCampaign(t, svc)
	var se *campaign.StateError
	if err := svc.GenerateVariant(context.Background(), id, campaign.VariantB, campaign.Events{}); !errors.As(err, &se) {
		t.Fatalf("got %v", err)
	}
	if _, err := svc.SaveVariantContents(context.Background(), id, campaign.VariantB, map[string]content.Content{
		"tr-TR": {SMSBody: "{{first_name}}, %15 indirim", EmailSubject: "Konu", EmailBody: "Gövde"}}); !errors.As(err, &se) {
		t.Fatalf("save B without B: %v", err)
	}
}

// A model that writes Variant A again is told so and asked again.
func TestAModelThatRepeatsVariantAIsAskedAgain(t *testing.T) {
	t.Parallel()
	m := mock.NewScripted(nil, []mock.Step{raw(goodMessages), raw(goodMessages), {Kind: mock.Smart}})
	svc, _ := newService(t, m)
	id := readyCampaign(t, svc)
	ctx := context.Background()
	if err := svc.GenerateMessages(ctx, id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	var retries [][]string
	err := svc.GenerateVariant(ctx, id, campaign.VariantB, campaign.Events{
		Retry: func(code string, _ int, p []string) {
			if code == "tr-TR" {
				retries = append(retries, p)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(retries) != 1 || !slices.Contains(retries[0], campaign.DuplicateVariantProblem) {
		t.Fatalf("retries %v", retries)
	}
	v, _ := svc.Get(ctx, id)
	if v.VariantB.Duplicate || v.VariantB.Content.SMSBody == v.Content.SMSBody {
		t.Fatalf("B is A: %+v", v.VariantB.Content)
	}
}

// A Variant B the model cannot write is an error; nothing is stored and the
// built-in template is not used (it could be A's text again).
func TestAFailedVariantBStoresNothing(t *testing.T) {
	t.Parallel()
	m := mock.NewScripted(nil, []mock.Step{{Kind: mock.Smart}, raw(tooLongSMS())})
	svc, _ := newService(t, m)
	id := offerCampaign(t, svc)
	ctx := context.Background()
	if err := svc.GenerateMessages(ctx, id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	err := svc.GenerateVariant(ctx, id, campaign.VariantB, campaign.Events{})
	var ge *campaign.GenerationError
	if !errors.As(err, &ge) || !strings.HasPrefix(ge.Message, "Varyant B") {
		t.Fatalf("got %v", err)
	}
	v, _ := svc.Get(ctx, id)
	if len(v.Variants) != 1 || v.VariantB != nil {
		t.Fatalf("variants %v", v.Variants)
	}
}

// ---- editing ------------------------------------------------------------------------

// Each variant has its own source: changing A makes only A's translations
// stale, changing B only B's.
func TestEditingOneVariantMakesOnlyItsTranslationsStale(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	id := abCampaign(t, svc)
	ctx := context.Background()
	v, _ := svc.Get(ctx, id)
	translations := len(v.Locales) - 1

	editedA := editAll(v.Content.Content, strings.NewReplacer("indirim", "indirim fırsatı"))
	v, err := svc.SaveContent(ctx, id, editedA)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Pending) != translations || len(v.VariantB.Pending) != 0 {
		t.Fatalf("after editing A: A pending %v, B pending %v", v.Pending, v.VariantB.Pending)
	}
	if v.VariantB.Content.Version != 1 || v.VariantB.Content.Source != campaign.SourceLLM {
		t.Errorf("B changed: %+v", v.VariantB.Content)
	}

	editedB := editAll(v.VariantB.Content.Content, strings.NewReplacer("indirim", "indirim seni bekliyor"))
	v, err = svc.SaveVariantContents(ctx, id, campaign.VariantB, map[string]content.Content{"tr-TR": editedB})
	if err != nil {
		t.Fatal(err)
	}
	if v.VariantB.Content.Source != campaign.SourceEdited || v.VariantB.Content.Version != 2 || len(v.VariantB.Pending) != translations {
		t.Fatalf("after editing B: %+v pending %v", v.VariantB.Content, v.VariantB.Pending)
	}

	// Translating B again leaves A's (stale) translations alone.
	v, err = svc.TranslateVariant(ctx, id, campaign.VariantB, v.VariantB.Pending)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.VariantB.Pending) != 0 || len(v.Pending) != translations {
		t.Fatalf("after translating B: A %v B %v", v.Pending, v.VariantB.Pending)
	}
	// Only tr-TR is edited, in B as in A.
	var ro *campaign.ReadOnlyTranslationError
	if _, err := svc.SaveVariantContents(ctx, id, campaign.VariantB, map[string]content.Content{"en-GB": editedB}); !errors.As(err, &ro) {
		t.Fatalf("editing B's en-GB: %v", err)
	}
}

// Variant B may not become Variant A, nor A become B (case and spaces aside).
func TestTheTwoVariantsCannotBeTheSameText(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	id := abCampaign(t, svc)
	ctx := context.Background()
	v, _ := svc.Get(ctx, id)
	a, b := v.Content.Content, v.VariantB.Content.Content

	var dup *campaign.DuplicateVariantError
	copyOfA := content.Content{SMSBody: "  " + strings.ToUpper(a.SMSBody[:1]) + a.SMSBody[1:], EmailSubject: a.EmailSubject + " ", EmailBody: a.EmailBody}
	if _, err := svc.SaveVariantContents(ctx, id, campaign.VariantB, map[string]content.Content{"tr-TR": copyOfA}); !errors.As(err, &dup) {
		t.Fatalf("B = A: %v", err)
	}
	if _, err := svc.SaveContent(ctx, id, b); !errors.As(err, &dup) {
		t.Fatalf("A = B: %v", err)
	}
	// Only one field may be the same: still two texts.
	sameSMS := content.Content{SMSBody: a.SMSBody, EmailSubject: b.EmailSubject, EmailBody: b.EmailBody}
	if _, err := svc.SaveVariantContents(ctx, id, campaign.VariantB, map[string]content.Content{"tr-TR": sameSMS}); err != nil {
		t.Fatalf("B with A's SMS only: %v", err)
	}
}

// A copy that got into the database anyway (written before the rule) is shown
// as a duplicate and refused at approval.
func TestApprovalRefusesTwoIdenticalVariants(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	id := abCampaign(t, svc)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		UPDATE campaign_contents b SET sms_body=a.sms_body, email_subject=a.email_subject, email_body=a.email_body
		FROM campaign_contents a
		WHERE b.campaign_id=$1 AND b.variant='B' AND b.locale='tr-TR' AND a.campaign_id=$1 AND a.variant='A' AND a.locale='tr-TR'`, id); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(ctx, id)
	if !v.VariantB.Duplicate {
		t.Error("duplicate not shown")
	}
	var dup *campaign.DuplicateVariantError
	if _, err := svc.Approve(ctx, id); !errors.As(err, &dup) {
		t.Fatalf("approve: %v", err)
	}
	if s := campaignStatus(t, pool, id); s != campaign.StatusContentReady {
		t.Errorf("status %s", s)
	}
}

// ---- approval -----------------------------------------------------------------------

// A stale translation of either variant blocks the approval (409); the error
// says which variant waits for which locale.
func TestApprovalWaitsForTheTranslationsOfBothVariants(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	id := abCampaign(t, svc)
	ctx := context.Background()
	v, _ := svc.Get(ctx, id)
	editedB := editAll(v.VariantB.Content.Content, strings.NewReplacer("indirim", "indirim seni bekliyor"))
	if _, err := svc.SaveVariantContents(ctx, id, campaign.VariantB, map[string]content.Content{"tr-TR": editedB}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Approve(ctx, id)
	var st *campaign.StaleTranslationsError
	if !errors.As(err, &st) {
		t.Fatalf("approve: %v", err)
	}
	if len(st.Missing)+len(st.Stale) != 0 || len(st.Variants["A"].Stale) != 0 || len(st.Variants["B"].Stale) != len(v.Locales)-1 {
		t.Fatalf("A %v/%v, variants %+v", st.Missing, st.Stale, st.Variants)
	}
	if _, _, n := deliveryCounts(t, pool, id); n != 0 {
		t.Errorf("queued %d", n)
	}
}

// Everyone gets A or B, decided by a hash of (campaign, person) whatever their
// locale: both channels of a person carry the same variant, a person is queued
// once per channel, and the approval reports what was queued per variant.
func TestApprovalSplitsThePeopleBetweenTheVariants(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	id := abCampaign(t, svc)
	ctx := context.Background()
	res, err := svc.Approve(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := pool.Query(ctx, `SELECT user_id, array_agg(DISTINCT variant), count(*) FROM deliveries WHERE campaign_id=$1 GROUP BY 1`, id)
	if err != nil {
		t.Fatal(err)
	}
	people := map[string]int{}
	for rows.Next() {
		var user int64
		var variants []string
		var n int
		if err := rows.Scan(&user, &variants, &n); err != nil {
			t.Fatal(err)
		}
		if len(variants) != 1 || n > 2 {
			t.Fatalf("person %d: variants %v, %d deliveries", user, variants, n)
		}
		if want := campaign.AssignVariant(id, user); variants[0] != want {
			t.Fatalf("person %d got %s, the hash says %s", user, variants[0], want)
		}
		people[variants[0]]++
	}
	rows.Close()
	total := people["A"] + people["B"]
	if people["A"] < total/3 || people["B"] < total/3 {
		t.Errorf("split %v", people)
	}
	for _, variant := range campaign.Variants {
		var sms, email int
		if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE channel='sms'), count(*) FILTER (WHERE channel='email')
			FROM deliveries WHERE campaign_id=$1 AND variant=$2`, id, variant).Scan(&sms, &email); err != nil {
			t.Fatal(err)
		}
		got := res.Variants[variant]
		if got.People != people[variant] || got.SMS != sms || got.Email != email {
			t.Errorf("%s: approval says %+v, queued people %d sms %d email %d", variant, got, people[variant], sms, email)
		}
	}
	// The locale plays no part: both variants reach people of several locales.
	var localesB int
	if err := pool.QueryRow(ctx, `SELECT count(DISTINCT u.locale) FROM deliveries d JOIN users u ON u.id=d.user_id
		WHERE d.campaign_id=$1 AND d.variant='B'`, id).Scan(&localesB); err != nil {
		t.Fatal(err)
	}
	if localesB < 3 {
		t.Errorf("Variant B reaches %d locales", localesB)
	}
	// The sending screen compares the variants.
	v, _ := svc.Get(ctx, id)
	if len(v.Delivery.Variants) != 2 || v.Delivery.Variants[0].Variant != "A" || v.Delivery.Variants[1].People != people["B"] {
		t.Fatalf("delivery variants %+v", v.Delivery.Variants)
	}
}

func TestTheHashInGoMatchesTheDatabase(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	_ = svc
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT id, CASE WHEN get_byte(decode(md5((42::bigint)::text || ':' || id::text), 'hex'), 0) % 2 = 0 THEN 'A' ELSE 'B' END
		FROM users ORDER BY id LIMIT 200`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			t.Fatal(err)
		}
		if got := campaign.AssignVariant(42, id); got != v {
			t.Fatalf("user %d: Go %s, SQL %s", id, got, v)
		}
	}
}

// Removing Variant B turns the test off: everyone gets A.
func TestRemovingVariantBSendsEveryoneVariantA(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	id := abCampaign(t, svc)
	ctx := context.Background()
	v, err := svc.DeleteVariant(ctx, id, campaign.VariantB)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Variants) != 1 || v.VariantB != nil || v.VariantCounts != nil {
		t.Fatalf("after delete: %v %+v %v", v.Variants, v.VariantB, v.VariantCounts)
	}
	var ie *campaign.InputError
	if _, err := svc.DeleteVariant(ctx, id, campaign.VariantA); !errors.As(err, &ie) {
		t.Fatalf("delete A: %v", err)
	}
	res, err := svc.Approve(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if res.Variants != nil {
		t.Errorf("variants %v", res.Variants)
	}
	var b int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM deliveries WHERE campaign_id=$1 AND variant<>'A'`, id).Scan(&b); err != nil {
		t.Fatal(err)
	}
	if b != 0 {
		t.Errorf("%d deliveries not A", b)
	}
	if v, _ := svc.Get(ctx, id); v.Delivery.Variants != nil {
		t.Errorf("comparison without B: %+v", v.Delivery.Variants)
	}
	// After approval Variant B can be neither added nor removed.
	var se *campaign.StateError
	if err := svc.GenerateVariant(ctx, id, campaign.VariantB, campaign.Events{}); !errors.As(err, &se) {
		t.Errorf("generate B after approval: %v", err)
	}
	if _, err := svc.DeleteVariant(ctx, id, campaign.VariantB); !errors.As(err, &se) {
		t.Errorf("delete B after approval: %v", err)
	}
}

// A scheduled A/B campaign is split when it starts (the audience is decided then).
func TestAScheduledABCampaignIsSplitWhenItStarts(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	id := abCampaign(t, svc)
	ctx := context.Background()
	at := inTwoHours()
	res, err := svc.ApproveAt(ctx, id, &at)
	if err != nil {
		t.Fatal(err)
	}
	if res.Variants["A"].People == 0 || res.Variants["B"].People == 0 {
		t.Fatalf("estimate %+v", res.Variants)
	}
	if n, err := newScheduler(t, pool, clock.NewFake(at.Add(time.Minute))).RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("started %d, %v", n, err)
	}
	if got := variantsQueued(t, pool, id); got["A"] == 0 || got["B"] == 0 {
		t.Fatalf("queued %v", got)
	}
}

func variantsQueued(t *testing.T, pool *pgxpool.Pool, id int64) map[string]int {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT variant, count(*) FROM deliveries WHERE campaign_id=$1 GROUP BY 1`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var v string
		var n int
		if err := rows.Scan(&v, &n); err != nil {
			t.Fatal(err)
		}
		out[v] = n
	}
	return out
}

func TestUnknownVariantIsAnInputError(t *testing.T) {
	t.Parallel()
	var ie *campaign.InputError
	if _, err := campaign.ParseVariant("C"); !errors.As(err, &ie) {
		t.Fatalf("C: %v", err)
	}
	for in, want := range map[string]string{"": "A", "a": "A", "B": "B", " b ": "B"} {
		if got, err := campaign.ParseVariant(in); err != nil || got != want {
			t.Errorf("%q: %s %v", in, got, err)
		}
	}
}
