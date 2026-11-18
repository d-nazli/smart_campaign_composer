package mock

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/offer"
)

// Every line of the bank has all eight translations, each with exactly the
// slots of its Turkish (the same numbers, dates, codes, offer and placeholder).
func TestEveryBankLineIsInEveryLanguageWithTheSameSlots(t *testing.T) {
	slotsOf := func(s string) []string {
		var out []string
		for _, m := range slotRe.FindAllString(s, -1) {
			out = append(out, slotName(m))
		}
		slices.Sort(out)
		return out
	}
	for _, e := range bank() {
		want := slices.Clone(e.slots)
		slices.Sort(want)
		for _, lang := range bankLangs {
			s := e.out[lang]
			if strings.TrimSpace(s) == "" {
				t.Errorf("%q has no %s translation", e.tr, lang)
				continue
			}
			if got := slotsOf(s); !slices.Equal(got, want) {
				t.Errorf("%q [%s] %q: slots %v, want %v", e.tr, lang, s, got, want)
			}
		}
		if e.short != nil && !slices.Equal(e.short.slots, e.slots) {
			t.Errorf("%q: the short form %q has other slots", e.tr, e.short.tr)
		}
	}
	if len(bank()) < 100 {
		t.Fatalf("only %d sentences", len(bank()))
	}
}

// The mock writes its Turkish only from the bank: every word of every text it
// can produce (every offer, theme, tone and variant, and the built-in
// template) is read as a bank sentence or a fact. Nothing is left over.
func TestTheMockSourceIsMadeOfBankSentencesOnly(t *testing.T) {
	p, _ := content.NewPolicy(nil)
	free := offer.Part{Kind: offer.KindFreeShipping}
	offers := []offer.Offer{
		{Kind: offer.KindNone},
		{Kind: offer.KindPercent, Value: 30},
		{Kind: offer.KindPercent, Value: 12.5, Code: "YAZ25"},
		{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"},
		{Kind: offer.KindFreeShipping},
		{Kind: offer.KindCouponCode, Code: "MERHABA10"},
		{Kind: offer.KindCouponCode, Code: "ANNELER", MinSpend: 750, ExpiresAt: "2026-08-31"},
		{Kind: offer.KindPercent, Value: 20, MaxDiscount: 300, Extras: []offer.Part{free}, ExpiresAt: "2027-03-08"},
		{Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300, Code: "KADEME100", ExpiresAt: "2027-03-08"},
		{Kind: offer.KindPercent, Value: 10, Extras: []offer.Part{{Kind: offer.KindAmount, Value: 50}, free}, Code: "BAYRAM10", MinSpend: 1500, ExpiresAt: "2027-04-01"},
	}
	briefs := []string{
		"Tüm müşterilere kampanya duyurusu gönder.",
		"Sepetini unutanlara samimi ve kısa bir hatırlatma gönder.",
		"Sepetinde ürün bırakan müşterilere resmi bir hatırlatma gönder.",
		"Kadınlar Günü'ne özel, heyecanlı bir dille",
		"Anneler Günü'ne özel zarif bir mesaj",
		"Babalar Günü'ne özel", "Sevgililer Günü'ne özel", "Öğretmenler Günü'ne özel", "23 Nisan'a özel",
		"Yılbaşına özel", "Black Friday'e özel", "Ramazan Bayramı'na özel", "Kurban Bayramı'na özel",
		"Son 60 günde sipariş vermemiş müşterilere", "Hiç alışveriş yapmamış müşterilere hoş geldin", "Toplam harcaması 5000 TL'den fazla olanlara teşekkür",
		"Herkese acil, son gün yaklaşıyor",
	}
	check := func(what, text string) {
		t.Helper()
		if _, unknown := parseField(text); len(unknown) > 0 {
			t.Errorf("%s: words outside the bank %q in\n%s", what, unknown, text)
		}
	}
	for _, o := range offers {
		for _, b := range briefs {
			for _, v := range []string{"A", "B"} {
				var m messagesJSON
				raw := smartMessagesFor(llm.MessageRequest{Brief: b, Offer: o, Policy: p, Locale: content.Locale{Code: "tr-TR"}, Theme: brief.ThemeOf(b, "tr"), Variant: v})
				mustDecode(t, raw, &m)
				for _, f := range []string{m.SMSBody, m.EmailSubject, m.EmailBody} {
					check(b+" / "+v, f)
				}
			}
		}
		fb := p.Fallback(o)
		for _, f := range []string{fb.SMSBody, fb.EmailSubject, fb.EmailBody} {
			check("template", f)
		}
	}
}

// The minimum basket goes after the offer in every other language.
func TestTheMinimumFollowsTheOfferInTranslation(t *testing.T) {
	p, _ := content.NewPolicy(nil)
	c := translateContent(content.Content{
		SMSBody:      "{{first_name}}, sana özel: 1000 TL ve üzeri alışverişte %20 indirim (en fazla 300 TL) + ücretsiz kargo! Kupon kodu: YAZ20. Son kullanım: 31.12.2026.",
		EmailSubject: "Sana özel: %20 indirim",
		EmailBody:    "Merhaba {{first_name}},\n\n1000 TL ve üzeri alışverişte %20 indirim seni bekliyor.\n\nSevgiler",
	}, "en", p)
	want := content.Content{
		SMSBody:      "{{first_name}}, just for you: 20% off (up to 300 TL) + free shipping on orders of 1000 TL and over! Coupon code: YAZ20. Valid until: 31.12.2026.",
		EmailSubject: "Just for you: 20% off",
		EmailBody:    "Hello {{first_name}},\n\n20% off on orders of 1000 TL and over is waiting for you.\n\nBest wishes",
	}
	if c != want {
		t.Fatalf("got  %+v\nwant %+v", c, want)
	}
}

// A translation longer than the source is shortened like a translator would:
// short terms first, then without the call to action, and the punctuation of
// what was left out goes with it.
func TestALongTranslationIsShortenedCleanly(t *testing.T) {
	p, _ := content.NewPolicy(nil)
	src := "Sayın {{first_name}}, sepetinizdekiler sizi bekliyor: her 1000 TL alışverişe 100 TL indirim (toplamda en fazla 300 TL) + ücretsiz kargo! Şimdi göz atın."
	for _, lang := range bankLangs {
		got := translateContent(content.Content{SMSBody: src}, lang, p).SMSBody
		if n := p.EffectiveLength(got); n > content.SMSMaxRunes {
			t.Errorf("%s: %d characters: %s", lang, n, got)
		}
		if strings.Contains(got, "!.") || strings.Contains(got, " .") || strings.Contains(got, ",:") {
			t.Errorf("%s: broken punctuation: %s", lang, got)
		}
	}
}

func mustDecode(t *testing.T, raw string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
}
