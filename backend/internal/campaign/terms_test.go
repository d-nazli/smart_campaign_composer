package campaign_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// The brief that was reported.
const mothersDay = "anneler gününe özel 1000 tl üzeri alışverişe 100tl indirim kullandır son kullanım 31.12.2026"

var mothersDayOffer = offer.Offer{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"}

func createBrief(t *testing.T, svc *campaign.Service, brief string) *campaign.View {
	t.Helper()
	v, err := svc.Create(context.Background(), brief)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// "1000 TL üzeri" is a term of the offer. The brief names no audience, so the
// one open question is WHO, with the understood offer shown next to it.
func TestTheReportedBriefAsksWhoAndKeepsTheOffer(t *testing.T) {
	m := mock.NewScripted([]mock.Step{raw(`{"status":"ok","match":"all","conditions":[{"field":"total_spent","op":"gt","value":1000}],"explanation":"x","question":"","offer":{"kind":"none","value":0,"code":"","min_spend":0,"expires_at":""}}`)}, nil)
	svc, pool := newService(t, m)
	v := createBrief(t, svc, mothersDay)

	s := v.Segment
	if v.Status != campaign.StatusDraft || s.Status != segment.StatusNeedsClarification || s.Audience != nil || len(s.Conditions) != 0 {
		t.Fatalf("%+v %+v", v, s)
	}
	if s.Question != "Bu kampanya kime gönderilsin? Tüm müşterilere mi?" || s.Suggestion != segment.SuggestAllCustomers {
		t.Fatalf("question %q suggestion %q", s.Question, s.Suggestion)
	}
	if !s.Offer.Equal(mothersDayOffer) {
		t.Fatalf("the understood offer must be shown: %+v", s.Offer)
	}
	if n := len(m.Calls()); n != 0 {
		t.Fatalf("a brief that names nobody is decided by code, the model was called %d times", n)
	}

	// It survives a reload (it is stored with the campaign).
	again, err := svc.Get(context.Background(), v.ID)
	if err != nil || again.Segment.Suggestion != segment.SuggestAllCustomers || !again.Segment.Offer.Equal(mothersDayOffer) {
		t.Fatalf("reload: %+v %v", again.Segment, err)
	}
	// A campaign that is only a question can not be approved.
	var se *campaign.StateError
	if _, err := svc.Approve(context.Background(), v.ID); !errors.As(err, &se) {
		t.Fatalf("approve: %v", err)
	}
	_ = pool
}

// After the person answers "send to all", the audience is every customer who
// may be contacted: the rule says so explicitly, and consent still applies.
func TestEveryoneMeansEveryCustomerWhoMayBeContacted(t *testing.T) {
	svc, pool := newService(t, mock.New(0))
	v := createBrief(t, svc, "herkese %10 indirim")
	s := v.Segment
	if s.Status != segment.StatusOK || len(s.Conditions) != 1 || s.Conditions[0].Field != "all_customers" || s.Suggestion != "" {
		t.Fatalf("%+v", s)
	}

	var users, sms, email int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*), count(*) FILTER (WHERE sms_consent AND phone IS NOT NULL), count(*) FILTER (WHERE email_consent AND email IS NOT NULL) FROM users`).
		Scan(&users, &sms, &email); err != nil {
		t.Fatal(err)
	}
	a := s.Audience
	if a.Matched != users || a.SMSReachable != sms || a.EmailReachable != email {
		t.Fatalf("audience %+v, want matched %d sms %d email %d", a, users, sms, email)
	}
	if sms >= users || email >= users {
		t.Fatal("the seed must contain people who cannot be contacted, or this test proves nothing")
	}

	// Approving queues exactly the reachable people, never someone without consent.
	if err := svc.GenerateMessages(context.Background(), v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Approve(context.Background(), v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued.SMS != sms || res.Queued.Email != email {
		t.Fatalf("queued %+v, want sms %d email %d", res.Queued, sms, email)
	}
}

func TestSendingToAllAfterTheQuestionKeepsTheTerms(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	v := createBrief(t, svc, mothersDay+"\n\nTüm müşterilere gönder.")
	if v.Segment.Status != segment.StatusOK || v.Segment.Conditions[0].Field != "all_customers" || !v.Segment.Offer.Equal(mothersDayOffer) {
		t.Fatalf("%+v", v.Segment)
	}

	var rec recorded
	if err := svc.GenerateMessages(context.Background(), v.ID, rec.events()); err != nil {
		t.Fatal(err)
	}
	if rec.valid == nil {
		t.Fatalf("the smart mock fell back: %q", rec.fbMsg)
	}
	got, _ := svc.Get(context.Background(), v.ID)
	c := got.Content
	if c.Source != campaign.SourceLLM || len(c.Report.Warnings) != 0 {
		t.Fatalf("source %s warnings %+v", c.Source, c.Report.Warnings)
	}
	for _, text := range []string{c.SMSBody, c.EmailSubject + "\n" + c.EmailBody} {
		if !strings.Contains(text, "1000 TL") || !strings.Contains(text, "31.12.2026") || !strings.Contains(text, "100 TL") {
			t.Errorf("the message must state the offer and its terms: %q", text)
		}
	}
}

// A model that leaves the last day or the minimum out is asked again, with the
// terms spelled out; if it never gets it right, the template states them.
func TestMessagesThatForgetTheTermsAreRepairedThenFallBackWithTheTerms(t *testing.T) {
	forgot := msgs("{{first_name}}, 100 TL indirim seni bekliyor!", "100 TL indirim", "Merhaba {{first_name}},\\n\\n100 TL indirim seni bekliyor.")
	good := msgs("{{first_name}}, 1000 TL ve üzeri alışverişte 100 TL indirim seni bekliyor! Son kullanım: 31.12.2026", "100 TL indirim fırsatı",
		"Merhaba {{first_name}},\\n\\n1000 TL ve üzeri alışverişte 100 TL indirim var. Son kullanım: 31.12.2026")

	m := mock.NewScripted(nil, []mock.Step{raw(forgot), raw(good)})
	svc, _ := newService(t, m)
	v := createBrief(t, svc, mothersDay+"\n\nTüm müşterilere gönder.")
	var rec recorded
	if err := svc.GenerateMessages(context.Background(), v.ID, rec.events()); err != nil {
		t.Fatal(err)
	}
	calls := messageCalls(m)
	if len(calls) != 2 || rec.valid == nil {
		t.Fatalf("calls %d, valid %v", len(calls), rec.valid)
	}
	fb := strings.Join(calls[1].Previous[0].Problems, " ")
	if !strings.Contains(fb, "31.12.2026") || !strings.Contains(fb, "1000 TL ve üzeri alışverişte") {
		t.Fatalf("the repair feedback must spell out the terms: %s", fb)
	}

	// Never repaired: the built-in template carries the terms.
	svc2, _ := newService(t, mock.NewScripted(nil, []mock.Step{raw(forgot)}))
	v2 := createBrief(t, svc2, mothersDay+"\n\nTüm müşterilere gönder.")
	var rec2 recorded
	if err := svc2.GenerateMessages(context.Background(), v2.ID, rec2.events()); err != nil {
		t.Fatal(err)
	}
	if rec2.fb == nil {
		t.Fatal("expected the fallback")
	}
	if !mothersDayOffer.Mentions(rec2.fb.SMSBody) || !mothersDayOffer.Mentions(rec2.fb.EmailSubject+"\n"+rec2.fb.EmailBody) {
		t.Fatalf("the fallback lost the terms: %+v", rec2.fb)
	}
}

// Editing the text and deleting the date is the person's call, but it is warned about.
func TestEditedTextThatLosesTheLastDayIsWarnedAbout(t *testing.T) {
	svc, _ := newService(t, mock.New(0))
	v := createBrief(t, svc, mothersDay+"\n\nTüm müşterilere gönder.")
	got, err := svc.SaveContents(context.Background(), v.ID, allLanguages(contentOf("{{first_name}}, 1000 TL ve üzeri alışverişte 100 TL indirim!", "100 TL indirim", "Merhaba {{first_name}}, 1000 TL ve üzeri alışverişte 100 TL indirim var.")))
	if err != nil {
		t.Fatal(err)
	}
	w := got.Content.Report.Warnings
	if len(w) < 2 || !strings.Contains(w[0].Message, "31.12.2026") {
		t.Fatalf("%+v", w)
	}
}

func contentOf(sms, subject, body string) content.Content {
	return content.Content{SMSBody: sms, EmailSubject: subject, EmailBody: body}
}
