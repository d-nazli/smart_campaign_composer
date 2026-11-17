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
		// terms of the offer: a minimum basket and a last day
		{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"},
		{Kind: offer.KindPercent, Value: 10, MinSpend: 1500.5},
		{Kind: offer.KindFreeShipping, ExpiresAt: "2027-01-05"},
		{Kind: offer.KindCouponCode, Code: "YAZ25", MinSpend: 500, ExpiresAt: "2026-12-31"},
		{Kind: offer.KindPercent, Value: 15, Code: "YAZ25", MinSpend: 1000, ExpiresAt: "2026-12-31"},
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

// ---- the terms and the audience, as the prompts put them -----------------------

func TestSegmentPromptSaysTermsBelongToTheOfferNotTheAudience(t *testing.T) {
	seg := llm.SegmentSystemPrompt()
	for _, want := range []string{
		"alışveriş tutarı şartları teklife aittir, kitleye değil",
		"min_spend", "expires_at", "all_customers",
		"Bu kampanya kime gönderilsin? Tüm müşterilere mi?",
		"1000 TL üzeri alışverişe 100 TL indirim",
	} {
		if !strings.Contains(seg, want) {
			t.Errorf("segment system prompt should contain %q", want)
		}
	}
}

func TestMessagesPromptCarriesTheTerms(t *testing.T) {
	o := offer.Offer{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"}
	user := llm.MessagesUserPrompt("brief", "Tüm müşteriler", o)
	for _, want := range []string{"100 TL indirim", "1000 TL ve üzeri alışverişte", "31.12.2026"} {
		if !strings.Contains(user, want) {
			t.Errorf("messages user prompt should contain %q:\n%s", want, user)
		}
	}
	msg := llm.MessagesSystemPrompt(policy(t))
	for _, want := range []string{"minimum", "last day", "dd.mm.yyyy"} {
		if !strings.Contains(msg, want) {
			t.Errorf("messages system prompt should explain %q", want)
		}
	}
}

// ---- special days, combined and tiered offers, as the prompts put them -----------

func TestSegmentPromptExplainsSpecialDaysAndCombinedAndTieredOffers(t *testing.T) {
	seg := llm.SegmentSystemPrompt()
	for _, want := range []string{
		"SPECIAL DAYS", "never its audience", "gender", "kadın müşterilere özel",
		"COMBINED OFFERS", "extras", "ya da", "needs_clarification",
		"TIERED DISCOUNTS", "step_spend", "max_discount",
		"names a special day, the audience is everyone",
	} {
		if !strings.Contains(seg, want) {
			t.Errorf("segment system prompt should contain %q", want)
		}
	}
	if strings.Contains(seg, "age, gender") {
		t.Error("gender is a supported field now; the prompt must not call it unsupported")
	}
}

func TestMessagesPromptsCarryTheThemeAndEveryPartOfTheOffer(t *testing.T) {
	o := offer.Offer{Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300, Extras: []offer.Part{{Kind: offer.KindFreeShipping}}}
	theme := llm.Theme{Occasion: "womens_day", OccasionName: "Kadınlar Günü", Topic: llm.TopicCart, Tones: []string{llm.ToneFriendly, llm.ToneShort}}
	user := llm.MessagesUserPromptFor(llm.MessageRequest{Brief: "b", Explanation: "tüm müşteriler", Offer: o, Theme: theme})
	for _, want := range []string{"her 1000 TL alışverişe 100 TL indirim", "toplamda en fazla 300 TL", "ücretsiz kargo",
		"special day: Kadınlar Günü", "topic: cart", "tone: friendly, short", "never the audience"} {
		if !strings.Contains(user, want) {
			t.Errorf("messages user prompt should contain %q:\n%s", want, user)
		}
	}
	en := llm.MessagesUserPromptFor(llm.MessageRequest{Brief: "b", Offer: o, Language: "en", Theme: theme})
	if !strings.Contains(en, "100 TL off every 1000 TL (up to 300 TL in total) + free shipping") {
		t.Errorf("the English wording of every part must be given:\n%s", en)
	}
	if plain := llm.MessagesUserPromptFor(llm.MessageRequest{Brief: "b", Offer: o}); strings.Contains(plain, "Theme") {
		t.Errorf("no theme, no theme line:\n%s", plain)
	}
	sys := llm.MessagesSystemPrompt(policy(t))
	for _, want := range []string{"EVERY part", "step, the discount per step and the cap", "THEME", "do not assume the reader's gender", "stock lines"} {
		if !strings.Contains(sys, want) {
			t.Errorf("messages system prompt should contain %q", want)
		}
	}
}
