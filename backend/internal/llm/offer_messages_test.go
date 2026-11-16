package llm_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
)

// The smart mock stands in for the model: for every kind of offer and every
// tone it must write messages that pass the same checks the backend applies
// to the real model.
func TestSmartMockMessagesPassEveryCheckForEveryOffer(t *testing.T) {
	p := policy(t)
	offers := []offer.Offer{
		{Kind: offer.KindNone},
		{Kind: offer.KindPercent, Value: 15},
		{Kind: offer.KindPercent, Value: 12.5},
		{Kind: offer.KindAmount, Value: 100},
		{Kind: offer.KindAmount, Value: 1000},
		{Kind: offer.KindFreeShipping},
		{Kind: offer.KindCouponCode, Code: "HOSGELDIN10"},
		{Kind: offer.KindPercent, Value: 15, Code: "YAZ25"},
	}
	briefs := []string{
		exampleBrief,
		"Sepetini terk edenlere resmi bir hatırlatma gönder",
		"Yeni sezon duyurusu",
		"Samimi bir teşekkür mesajı",
		"Sepetteki ürünler için kısa hatırlatma",
		"Sepetteki ürünler için kısa ve samimi hatırlatma",
	}
	for _, o := range offers {
		for _, b := range briefs {
			out, err := mock.New(0).Messages(context.Background(), llm.MessageRequest{Brief: b, Offer: o, Policy: p}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var c content.Content
			if err := json.Unmarshal([]byte(out.Raw), &c); err != nil {
				t.Fatalf("%q: %v", b, err)
			}
			norm, rep := p.Validate(c)
			if !rep.OK {
				t.Errorf("[%s] %q: hard rules: %+v", o.Describe(), b, rep.Issues)
			}
			if q := p.Quality(norm, o); len(q) != 0 {
				t.Errorf("[%s] %q: quality: %+v\n%s", o.Describe(), b, q, out.Raw)
			}
		}
	}
}

// The wording of the report: "özel fırsat fırsatından" must never come back.
func TestSmartMockNeverRepeatsAWordInTheSMS(t *testing.T) {
	p := policy(t)
	for _, b := range []string{"Sepeti terk edenlere resmi hatırlatma", "Sepeti terk edenlere hatırlatma gönder"} {
		out, _ := mock.New(0).Messages(context.Background(), llm.MessageRequest{Brief: b, Policy: p}, nil)
		if strings.Contains(strings.ToLower(out.Raw), "fırsat fırsatından") {
			t.Errorf("%q: %s", b, out.Raw)
		}
	}
}

// ---- prompts tell the model about the offer --------------------------------

func TestPromptsCarryTheOffer(t *testing.T) {
	user := llm.MessagesUserPrompt("brief", "Adana'daki müşteriler", offer.Offer{Kind: offer.KindPercent, Value: 15})
	if !strings.Contains(user, "%15 indirim") {
		t.Errorf("messages prompt must state the offer the texts have to contain:\n%s", user)
	}
	if none := llm.MessagesUserPrompt("brief", "x", offer.Offer{}); strings.Contains(none, "%") {
		t.Errorf("no offer: the prompt must not invent one:\n%s", none)
	}
	seg := llm.SegmentSystemPrompt()
	for _, want := range []string{"offer", "free_shipping", "coupon_code", "silently"} {
		if !strings.Contains(seg, want) {
			t.Errorf("segment system prompt should mention %q", want)
		}
	}
	msg := llm.MessagesSystemPrompt(policy(t))
	for _, want := range []string{"offer", "repeat"} {
		if !strings.Contains(strings.ToLower(msg), want) {
			t.Errorf("messages system prompt should mention %q", want)
		}
	}
}
