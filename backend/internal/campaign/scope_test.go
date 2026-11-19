package campaign_test

import (
	"context"
	"strings"
	"testing"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

const scopeBrief = "Kış ürünlerinde geçerli 200 TL indirim, kupon kodu KIS200"

var scopeOffer = offer.Offer{Kind: offer.KindAmount, Value: 200, Code: "KIS200", Scope: "kış ürünleri"}

// The products are the offer's scope: the audience is everyone (the same
// number as "herkese"), nothing is asked, and the source text and every
// translation say what the offer is valid on.
func TestAScopeBriefGoesToEveryoneAndIntoEveryMessage(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	ctx := context.Background()

	v, err := svc.Create(ctx, scopeBrief)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != campaign.StatusSegmentReady || v.Segment.Status != segment.StatusOK || v.Segment.Question != "" {
		t.Fatalf("a scope is never a question: %+v", v.Segment)
	}
	if len(v.Segment.Conditions) != 1 || v.Segment.Conditions[0].Field != "all_customers" || !v.Segment.Offer.Equal(scopeOffer) {
		t.Fatalf("rule: %+v offer %+v", v.Segment.Conditions, v.Segment.Offer)
	}
	all, err := svc.Create(ctx, "Herkese 200 TL indirim, kupon kodu KIS200")
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Audience.Matched != all.Segment.Audience.Matched {
		t.Fatalf("the scope narrowed the audience: %d, everyone is %d", v.Segment.Audience.Matched, all.Segment.Audience.Matched)
	}

	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	v, err = svc.Get(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	src := v.Content.Content
	if v.Content.Source != campaign.SourceLLM || len(v.Content.Report.Warnings) != 0 ||
		!scopeOffer.Mentions(src.SMSBody) || !scopeOffer.Mentions(src.EmailSubject+"\n"+src.EmailBody) {
		t.Fatalf("the source text must state the scope: %+v %+v", src, v.Content.Report.Warnings)
	}
	for _, code := range []string{"en-GB", "de-DE"} {
		tr := v.Translations[code]
		if tr == nil || !strings.Contains(tr.SMSBody, "kış ürünleri") || !strings.Contains(tr.EmailBody, "kış ürünleri") {
			t.Errorf("%s keeps the scope: %+v", code, tr)
		}
	}
	if !strings.Contains(v.Translations["en-GB"].SMSBody, "Applies to: kış ürünleri") || !strings.Contains(v.Translations["de-DE"].SMSBody, "Gilt für: kış ürünleri") {
		t.Errorf("en=%q de=%q", v.Translations["en-GB"].SMSBody, v.Translations["de-DE"].SMSBody)
	}
	if len(v.Mismatches) != 0 {
		t.Errorf("mismatches: %+v", v.Mismatches)
	}
}

// A text that leaves the products out is sent back once, naming the scope;
// a model that never writes it gets the template, which states it.
func TestMessagesWithoutTheScopeAreRepaired(t *testing.T) {
	t.Parallel()
	forgot := msgs("{{first_name}}, 200 TL indirim seni bekliyor! Kupon kodu: KIS200.", "{{first_name}}, 200 TL indirim", "Merhaba {{first_name}},\\n\\n200 TL indirim var. Kupon kodu: KIS200.")
	good := msgs("{{first_name}}, kış ürünlerinde 200 TL indirim! Kupon kodu: KIS200.", "{{first_name}}, kış ürünlerinde 200 TL indirim", "Merhaba {{first_name}},\\n\\nKış ürünlerinde 200 TL indirim var. Kupon kodu: KIS200.")
	m := mock.NewScripted(nil, []mock.Step{raw(forgot), raw(good)})
	svc, _ := newService(t, m)
	v, err := svc.Create(context.Background(), scopeBrief)
	if err != nil {
		t.Fatal(err)
	}
	var rec recorded
	if err := svc.GenerateMessages(context.Background(), v.ID, rec.events()); err != nil {
		t.Fatal(err)
	}
	calls := messageCalls(m)
	if rec.valid == nil || len(calls) != 2 || !strings.Contains(strings.Join(calls[1].Previous[0].Problems, " "), "geçerli ürünler: kış ürünleri") {
		t.Fatalf("one repair naming the scope: valid=%v calls=%d", rec.valid, len(calls))
	}
	if calls[1].Offer.Scope != "kış ürünleri" {
		t.Fatalf("the scope must reach the model: %+v", calls[1].Offer)
	}

	svc, _ = newService(t, mock.NewScripted(nil, []mock.Step{raw(forgot)}))
	v, err = svc.Create(context.Background(), scopeBrief)
	if err != nil {
		t.Fatal(err)
	}
	rec = recorded{}
	if err := svc.GenerateMessages(context.Background(), v.ID, rec.events()); err != nil {
		t.Fatal(err)
	}
	if rec.fb == nil || !scopeOffer.Mentions(rec.fb.SMSBody) || !scopeOffer.Mentions(rec.fb.EmailSubject+"\n"+rec.fb.EmailBody) {
		t.Fatalf("the template must state the scope: %+v", rec.fb)
	}
}

// Choosing people by the products they bought needs product data: it is said
// out loud, the model is not asked and no audience is built.
func TestPeopleChosenByProductsAreUnsupported(t *testing.T) {
	t.Parallel()
	m := mock.NewScripted([]mock.Step{raw(`{"status":"ok","match":"all","conditions":[{"field":"all_customers","op":"eq","value":true}],"explanation":"x","question":""}`)}, nil)
	svc, _ := newService(t, m)
	v, err := svc.Create(context.Background(), "Kış ürünü almış kişilere %10 indirim")
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Status != segment.StatusUnsupported || !strings.Contains(v.Segment.Explanation, "ürün satın alma") || v.Segment.Audience != nil {
		t.Fatalf("%+v", v.Segment)
	}
	if n := len(m.Calls()); n != 0 {
		t.Fatalf("the model must not be asked (%d calls)", n)
	}
}
