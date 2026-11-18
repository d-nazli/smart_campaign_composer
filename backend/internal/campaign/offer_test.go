package campaign_test

import (
	"context"
	"strings"
	"testing"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// The brief that was reported: the audience included people who had bought.
const reportedBrief = "son 30 günde adanada sepete ekleme yapmış ancak sipariş oluşturmamış kişilere yüzde 15 indirim kuponunu tanımla"

func TestReportedBriefExcludesPeopleWhoBought(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	v, err := svc.Create(context.Background(), reportedBrief)
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Status != segment.StatusOK {
		t.Fatalf("status %s: %+v", v.Segment.Status, v.Segment)
	}

	var withBuyers, withoutBuyers int
	err = pool.QueryRow(context.Background(), `
		SELECT count(*),
		       count(*) FILTER (WHERE last_order_at IS NULL OR last_order_at < last_cart_add_at)
		FROM users
		WHERE city = 'Adana' AND last_cart_add_at >= now() - interval '30 days'`).Scan(&withBuyers, &withoutBuyers)
	if err != nil {
		t.Fatal(err)
	}
	if withoutBuyers == withBuyers {
		t.Fatalf("seed has nobody in Adana who bought after adding to the cart; this test would prove nothing (%d)", withBuyers)
	}
	if v.Segment.Audience.Matched != withoutBuyers {
		t.Fatalf("audience = %d, want %d (people who added to the cart in Adana but did not buy); without the 'did not buy' condition it would be %d",
			v.Segment.Audience.Matched, withoutBuyers, withBuyers)
	}
	if !v.Segment.Offer.Equal(offer.Offer{Kind: offer.KindPercent, Value: 15}) {
		t.Fatalf("offer = %+v", v.Segment.Offer)
	}

	// The offer survives a reload (it is stored with the rule).
	again, err := svc.Get(context.Background(), v.ID)
	if err != nil || !again.Segment.Offer.Equal(v.Segment.Offer) {
		t.Fatalf("reload: %+v %v", again.Segment, err)
	}
}

func TestUnsupportedBriefIsDecidedByTheBackendAndExplained(t *testing.T) {
	m := mock.NewScripted([]mock.Step{raw(`{"status":"ok","match":"all","conditions":[{"field":"city","op":"eq","value":"Adana"}],"explanation":"x","question":""}`)}, nil)
	svc, _ := newService(t, m)
	v, err := svc.Create(context.Background(), "Adana'daki 35 yaş üstü müşterilere %10 indirim")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != campaign.StatusDraft || v.Segment.Status != segment.StatusUnsupported || !strings.Contains(v.Segment.Explanation, "yaş") || v.Segment.Audience != nil {
		t.Fatalf("view: %+v %+v", v, v.Segment)
	}
	if n := len(m.Calls()); n != 0 {
		t.Fatalf("the model must not be asked about a brief the backend already knows it cannot serve (%d calls)", n)
	}
}

// ---- messages must carry the offer ----------------------------------------

func offerCampaign(t *testing.T, svc *campaign.Service) int64 {
	t.Helper()
	v, err := svc.Create(context.Background(), "Son 30 günde sepete ürün ekleyip almayanlara %15 indirim hatırlatması gönder")
	if err != nil || v.Segment.Status != segment.StatusOK {
		t.Fatalf("create: %v %+v", err, v)
	}
	return v.ID
}

// messageCalls are the calls of the message step (the mock also records the
// segment call that created the campaign).
func messageCalls(m *mock.Client) []mock.Call {
	var out []mock.Call
	for _, c := range m.Calls() {
		// The scripted run is the source text's; the translations are
		// answered separately (see messages.go) and are not part of these scripts.
		if c.Kind == "messages" {
			out = append(out, c)
		}
	}
	return out
}

func msgs(sms, subject, body string) string {
	return `{"sms_body":"` + sms + `","email_subject":"` + subject + `","email_body":"` + body + `"}`
}

func TestMessagesThatForgetTheOfferAreRepaired(t *testing.T) {
	forgot := msgs("{{first_name}}, sepetin seni bekliyor!", "Sepetin seni bekliyor", "Merhaba {{first_name}},\\n\\nSepetin hâlâ burada.")
	good := msgs("{{first_name}}, %15 indirim seni bekliyor!", "{{first_name}}, %15 indirim", "Merhaba {{first_name}},\\n\\nSepetinde %15 indirim var.")
	m := mock.NewScripted(nil, []mock.Step{raw(forgot), raw(good)})
	svc, pool := newService(t, m)
	id := offerCampaign(t, svc)

	var rec recorded
	if err := svc.GenerateMessages(context.Background(), id, rec.events()); err != nil {
		t.Fatal(err)
	}
	if rec.valid == nil || !strings.Contains(rec.valid.SMSBody, "%15") {
		t.Fatalf("expected the second answer to be stored, got valid=%v fallback=%v", rec.valid, rec.fb)
	}
	if total, valid := audit(t, pool, id, "messages"); total != 2 || valid != 1 {
		t.Fatalf("audit rows: %d/%d, want 2/1", total, valid)
	}
	calls := messageCalls(m)
	last := calls[len(calls)-1]
	if len(calls) != 2 || len(last.Previous) != 1 || !strings.Contains(strings.Join(last.Previous[0].Problems, " "), "%15 indirim") {
		t.Fatalf("the model must be told which offer it forgot: %+v", last.Previous)
	}
	if last.Offer.Kind != offer.KindPercent || last.Offer.Value != 15 {
		t.Fatalf("the offer must be passed to the model: %+v", last.Offer)
	}
}

func TestMessagesWithAWrongDiscountAreRepaired(t *testing.T) {
	wrong := msgs("{{first_name}}, %30 indirim seni bekliyor!", "{{first_name}}, %30 indirim", "Merhaba {{first_name}},\\n\\nSepetinde %30 indirim var.")
	good := msgs("{{first_name}}, %15 indirim seni bekliyor!", "{{first_name}}, %15 indirim", "Merhaba {{first_name}},\\n\\nSepetinde %15 indirim var.")
	svc, pool := newService(t, mock.NewScripted(nil, []mock.Step{raw(wrong), raw(good)}))
	id := offerCampaign(t, svc)
	var rec recorded
	if err := svc.GenerateMessages(context.Background(), id, rec.events()); err != nil {
		t.Fatal(err)
	}
	if rec.valid == nil || strings.Contains(rec.valid.SMSBody, "%30") {
		t.Fatalf("a discount that is not in the brief must not be stored: %+v", rec.valid)
	}
	if total, valid := audit(t, pool, id, "messages"); total != 2 || valid != 1 {
		t.Fatalf("audit rows: %d/%d", total, valid)
	}
}

func TestMessagesWithARepeatedWordAreRepaired(t *testing.T) {
	repeated := msgs("{{first_name}}, özel fırsat fırsatından %15 indirim!", "{{first_name}}, %15 indirim", "Merhaba {{first_name}},\\n\\nSepetinde %15 indirim var.")
	good := msgs("{{first_name}}, %15 indirim seni bekliyor!", "{{first_name}}, %15 indirim", "Merhaba {{first_name}},\\n\\nSepetinde %15 indirim var.")
	m := mock.NewScripted(nil, []mock.Step{raw(repeated), raw(good)})
	svc, _ := newService(t, m)
	id := offerCampaign(t, svc)
	var rec recorded
	if err := svc.GenerateMessages(context.Background(), id, rec.events()); err != nil {
		t.Fatal(err)
	}
	calls := messageCalls(m)
	if rec.valid == nil || len(calls) != 2 || len(calls[1].Previous) != 1 {
		t.Fatalf("repeated word must trigger exactly one repair round: valid=%v calls=%d", rec.valid, len(calls))
	}
	if !strings.Contains(strings.Join(calls[1].Previous[0].Problems, " "), "fırsat") {
		t.Fatalf("the feedback must name the repeated word: %v", calls[1].Previous[0].Problems)
	}
}

// When the model keeps forgetting, the person gets the built-in template, and
// that template states the offer too.
func TestFallbackAfterForgettingTheOfferStillStatesIt(t *testing.T) {
	forgot := msgs("{{first_name}}, sepetin seni bekliyor!", "Sepetin seni bekliyor", "Merhaba {{first_name}},\\n\\nSepetin hâlâ burada.")
	svc, _ := newService(t, mock.NewScripted(nil, []mock.Step{raw(forgot)}))
	id := offerCampaign(t, svc)
	var rec recorded
	if err := svc.GenerateMessages(context.Background(), id, rec.events()); err != nil {
		t.Fatal(err)
	}
	if rec.fb == nil || rec.fbMsg != campaign.FallbackInvalidOutput {
		t.Fatalf("expected fallback, got %v / %q", rec.fb, rec.fbMsg)
	}
	o := offer.Offer{Kind: offer.KindPercent, Value: 15}
	if !o.Mentions(rec.fb.SMSBody) || !o.Mentions(rec.fb.EmailSubject+" "+rec.fb.EmailBody) {
		t.Fatalf("fallback lost the offer: %+v", rec.fb)
	}
}

// The default (smart) mock must produce messages that pass every check.
func TestSmartMockMessagesFollowTheStoredOffer(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	id := offerCampaign(t, svc)
	var rec recorded
	if err := svc.GenerateMessages(context.Background(), id, rec.events()); err != nil {
		t.Fatal(err)
	}
	if rec.valid == nil {
		t.Fatalf("smart mock fell back to the template: %q", rec.fbMsg)
	}
	v, _ := svc.Get(context.Background(), id)
	if v.Content.Source != campaign.SourceLLM || len(v.Content.Report.Warnings) != 0 {
		t.Fatalf("stored content: %+v", v.Content)
	}
	var _ = content.Content{}
}

// A person may edit the text; the screen must then warn if the offer is gone.
func TestEditedContentWithoutTheOfferGetsAWarningNotASilentPass(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	id := offerCampaign(t, svc)
	v, err := saveSource(context.Background(), svc, id, (content.Content{
		SMSBody: "{{first_name}}, sepetin seni bekliyor!", EmailSubject: "Sepetin seni bekliyor", EmailBody: "Merhaba {{first_name}}, sepetin hâlâ burada.",
	}))
	if err != nil {
		t.Fatalf("editing is the person's call and must not be blocked: %v", err)
	}
	w := v.Content.Report.Warnings
	if len(w) < 2 {
		t.Fatalf("want an offer warning for the SMS and for the email, got %+v", w)
	}
	for _, i := range w {
		if i.Code != content.CodeOfferMissing || !strings.Contains(i.Message, "%15 indirim") {
			t.Errorf("unexpected warning: %+v", i)
		}
	}
	if !v.Content.Report.OK {
		t.Error("warnings must not make the report invalid")
	}
}
