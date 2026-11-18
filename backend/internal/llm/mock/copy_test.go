package mock_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/trtext"
)

// Test locales: the source and one locale per language of the sentence bank,
// with the free-shipping words of the locales table.
var (
	trTR = content.Locale{Code: "tr-TR", Name: "Turkish (Türkiye)", Direction: "ltr", IsSource: true, FreeShippingTerms: []string{"ücretsiz kargo"}}
	enGB = content.Locale{Code: "en-GB", Name: "English (United Kingdom)", Direction: "ltr", FreeShippingTerms: []string{"free shipping", "free delivery"}}
	deDE = content.Locale{Code: "de-DE", Name: "German (Germany)", Direction: "ltr", FreeShippingTerms: []string{"kostenloser versand", "versandkostenfrei"}}
	frFR = content.Locale{Code: "fr-FR", Name: "French (France)", Direction: "ltr", FreeShippingTerms: []string{"livraison gratuite"}}
	esMX = content.Locale{Code: "es-MX", Name: "Spanish (Mexico)", Direction: "ltr", FreeShippingTerms: []string{"envío gratis", "envío gratuito"}}
	itIT = content.Locale{Code: "it-IT", Name: "Italian (Italy)", Direction: "ltr", FreeShippingTerms: []string{"spedizione gratuita"}}
	nlNL = content.Locale{Code: "nl-NL", Name: "Dutch (Netherlands)", Direction: "ltr", FreeShippingTerms: []string{"gratis verzending"}}
	ptBR = content.Locale{Code: "pt-BR", Name: "Portuguese (Brazil)", Direction: "ltr", FreeShippingTerms: []string{"frete grátis"}}
	arSA = content.Locale{Code: "ar-SA", Name: "Arabic (Saudi Arabia)", Direction: "rtl", FreeShippingTerms: []string{"شحن مجاني"}}

	targets = []content.Locale{enGB, deDE, frFR, esMX, itIT, nlNL, ptBR, arSA}
)

func decodeChecked(t *testing.T, p content.Policy, raw, what string) content.Content {
	t.Helper()
	var c content.Content
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("%s: %v\n%s", what, err, raw)
	}
	norm, rep := p.Validate(c)
	if !rep.OK {
		t.Fatalf("%s: breaks a hard rule: %+v\n%+v", what, rep.Issues, c)
	}
	return norm
}

// writeAll writes the source text the way the service does (theme read from
// the brief) and translates it into every test locale, checking each text as
// the backend would: limits, and the translations saying what the source says.
func writeAll(t *testing.T, p content.Policy, b string, o offer.Offer) map[string]content.Content {
	t.Helper()
	m := mock.New(0)
	out, err := m.Messages(context.Background(), llm.MessageRequest{Brief: b, Offer: o, Policy: p, Locale: trTR, Theme: brief.ThemeOf(b, "tr")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	src := decodeChecked(t, p, out.Raw, b+" [tr-TR]")
	if q := p.Quality(src, o); len(q) != 0 {
		t.Fatalf("%q [tr-TR]: %+v\n%+v", b, q, src)
	}
	set := map[string]content.Content{trTR.Code: src}
	for _, l := range targets {
		c := translateChecked(t, m, p, src, l, b, o)
		set[l.Code] = c
	}
	return set
}

// translateChecked translates src into l and checks it as the backend does
// (limits; the same facts as the source), and that no Turkish is left in it.
func translateChecked(t *testing.T, m *mock.Client, p content.Policy, src content.Content, l content.Locale, b string, o offer.Offer) content.Content {
	t.Helper()
	out, err := m.Translate(context.Background(), llm.TranslateRequest{Source: src, SourceLocale: trTR, Target: l, Policy: p, Brief: b, Offer: o}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := decodeChecked(t, p, out.Raw, b+" ["+l.Code+"]")
	if issues := content.ConsistentWith(trTR, src, l, c); len(issues) != 0 {
		t.Fatalf("%q [%s]: the translation disagrees with the source: %+v\n%+v\n%+v", b, l.Code, issues, src, c)
	}
	if w := turkishIn(c); w != "" {
		t.Fatalf("%q [%s]: Turkish %q in the translation:\n%+v", b, l.Code, w, c)
	}
	return c
}

// turkishIn returns a Turkish letter or word found in a translation, or "".
func turkishIn(c content.Content) string {
	all := c.SMSBody + " " + c.EmailSubject + " " + c.EmailBody
	for _, r := range "ğĞşŞıİ" {
		if strings.ContainsRune(all, r) {
			return string(r)
		}
	}
	// Words only: a coupon code (BAYRAM10) is the source's data, not Turkish text.
	var words []string
	for _, w := range strings.FieldsFunc(trtext.Fold(all), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if !strings.ContainsAny(w, "0123456789") {
			words = append(words, w)
		}
	}
	folded := " " + strings.Join(words, " ") + " "
	for _, w := range []string{"indirim", "kargo", "ucretsiz", "ozel", "firsat", "kupon", "kodu", "kod", "son", "kullanim", "gun",
		"merhaba", "selam", "sayin", "sana", "size", "seni", "sizi", "sepet", "sepetini", "alisveris", "alisverise", "uzeri", "ve", "ile",
		"hemen", "simdi", "bekliyor", "sevgiler", "saygilarimizla", "gorusmek", "uzere", "bayram", "yarin", "magazamiz", "kapali"} {
		if strings.Contains(folded, " "+w+" ") {
			return w
		}
	}
	return ""
}

func defaultPolicy(t *testing.T) content.Policy {
	t.Helper()
	p, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Each brief gets copy about ITS subject: the special day is named, a cart
// reminder talks about the cart, a win-back says "we missed you". The texts
// differ from brief to brief instead of repeating one template.
func TestTheCopyFollowsTheBriefsThemeAndVaries(t *testing.T) {
	p := defaultPolicy(t)
	pct20 := offer.Offer{Kind: offer.KindPercent, Value: 20}
	cases := []struct {
		brief string
		o     offer.Offer
		want  []string // folded words the Turkish SMS or subject must contain (any of them)
	}{
		{"Kadınlar Günü'ne özel %20 indirim. Samimi olsun.", pct20, []string{"kadinlar gunu", "8 mart"}},
		{"Anneler Günü'ne özel ücretsiz kargo", offer.Offer{Kind: offer.KindFreeShipping}, []string{"anneler gunu", "anne"}},
		{"Babalar Günü'ne özel erkek müşterilere 100 TL indirim", offer.Offer{Kind: offer.KindAmount, Value: 100}, []string{"babalar gunu", "baba"}},
		{"Sevgililer Günü'ne özel %25 indirim, heyecanlı bir dille", offer.Offer{Kind: offer.KindPercent, Value: 25}, []string{"sevgililer", "14 subat", "sevdik"}},
		{"Black Friday'e özel %40 indirim", offer.Offer{Kind: offer.KindPercent, Value: 40}, []string{"black friday", "indirim gunu"}},
		{"Yılbaşına özel tüm müşterilere %30 indirim", offer.Offer{Kind: offer.KindPercent, Value: 30}, []string{"yeni yil", "yilbasi"}},
		{"Ramazan Bayramı'na özel ücretsiz kargo", offer.Offer{Kind: offer.KindFreeShipping}, []string{"bayram"}},
		{"Son 30 günde sepete ürün ekleyip almayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun.", offer.Offer{Kind: offer.KindPercent, Value: 30}, []string{"sepet"}},
		{"Son 60 günde sipariş vermemiş müşterilere resmi bir dille %15 indirim", offer.Offer{Kind: offer.KindPercent, Value: 15}, []string{"ozledik", "gorusemedik", "aramizda"}},
		{"Hiç alışveriş yapmamış müşterilere hoş geldin indirimi %10", offer.Offer{Kind: offer.KindPercent, Value: 10}, []string{"hos geldin", "ilk alisveris"}},
		{"Toplam harcaması 5000 TL'den fazla olanlara teşekkür olarak 200 TL indirim", offer.Offer{Kind: offer.KindAmount, Value: 200}, []string{"tesekkur", "tercih", "sadik"}},
	}
	seenSMS, seenSubject := map[string]bool{}, map[string]bool{}
	for _, c := range cases {
		set := writeAll(t, p, c.brief, c.o)
		tr := set["tr-TR"]
		t.Logf("%s\n  SMS: %s\n  Konu: %s\n  EN: %s\n  DE: %s", c.brief, tr.SMSBody, tr.EmailSubject, set["en-GB"].SMSBody, set["de-DE"].SMSBody)
		shown := trtext.Fold(tr.SMSBody + " " + tr.EmailSubject)
		found := false
		for _, w := range c.want {
			found = found || strings.Contains(shown, w)
		}
		if !found {
			t.Errorf("%q: the copy does not talk about the brief's subject (%v):\n%s\n%s", c.brief, c.want, tr.SMSBody, tr.EmailSubject)
		}
		// The skeleton without the offer and the name: what actually varies.
		skeleton := strings.NewReplacer("{{first_name}}", "", c.o.WhatIn("tr"), "").Replace(tr.SMSBody)
		seenSMS[skeleton], seenSubject[tr.EmailSubject] = true, true
	}
	if len(seenSMS) < len(cases)-1 || len(seenSubject) < len(cases)-1 {
		t.Errorf("the copy repeats itself: %d distinct SMS and %d distinct subjects for %d briefs", len(seenSMS), len(seenSubject), len(cases))
	}
}

// The same brief always gets the same text; a different brief with the same
// offer and theme gets a different one.
func TestTheCopyIsReproducibleButNotOneTemplate(t *testing.T) {
	p := defaultPolicy(t)
	o := offer.Offer{Kind: offer.KindPercent, Value: 15}
	a := writeAll(t, p, "Herkese %15 indirim", o)
	b := writeAll(t, p, "Herkese %15 indirim", o)
	if a["tr-TR"] != b["tr-TR"] {
		t.Fatal("same brief, different copy")
	}
	distinct := map[string]bool{}
	for _, br := range []string{"Herkese %15 indirim", "Tüm müşterilere %15 indirim gönder", "Bütün kullanıcılara %15 indirim", "Herkese %15 indirim kuponu", "Herkese yüzde 15 indirim tanımla"} {
		distinct[writeAll(t, p, br, o)["tr-TR"].SMSBody] = true
	}
	if len(distinct) < 3 {
		t.Errorf("five wordings of the same brief gave only %d different SMS", len(distinct))
	}
}

// Formal and friendly registers, and "short" leaves out the call to action.
func TestTheCopyFollowsTheTone(t *testing.T) {
	p := defaultPolicy(t)
	o := offer.Offer{Kind: offer.KindPercent, Value: 10}
	friendly := writeAll(t, p, "Herkese samimi bir dille %10 indirim", o)["tr-TR"]
	formal := writeAll(t, p, "Herkese resmi bir dille %10 indirim", o)["tr-TR"]
	if !strings.Contains(formal.EmailBody, "siz") && !strings.Contains(formal.EmailBody, "Sayın") {
		t.Errorf("formal copy should say siz/Sayın: %q", formal.EmailBody)
	}
	if strings.Contains(friendly.SMSBody, "Sayın") || strings.Contains(friendly.EmailBody, "Saygılarımızla") {
		t.Errorf("friendly copy should not be formal: %+v", friendly)
	}
	short := writeAll(t, p, "Herkese kısa bir %10 indirim mesajı", o)["tr-TR"]
	long := writeAll(t, p, "Herkese %10 indirim mesajı", o)["tr-TR"]
	if len([]rune(short.EmailBody)) >= len([]rune(long.EmailBody)) {
		t.Errorf("short copy is not shorter:\n%q\n%q", short.EmailBody, long.EmailBody)
	}
}

// A special day sets the theme only: everyone gets the message, so nobody is
// addressed as a woman, a mother or a father.
func TestASpecialDayNeverAssumesTheReadersGender(t *testing.T) {
	p := defaultPolicy(t)
	for _, b := range []string{"Kadınlar Günü'ne özel %20 indirim", "Anneler Günü'ne özel %20 indirim", "Babalar Günü'ne özel %20 indirim"} {
		set := writeAll(t, p, b, offer.Offer{Kind: offer.KindPercent, Value: 20})
		for code, c := range set {
			all := trtext.Fold(c.SMSBody + " " + c.EmailSubject + " " + c.EmailBody)
			for _, bad := range []string{"kadin musteri", "hanim", "sevgili anne", "sevgili baba", "dear mum", "dear dad", "liebe mutter"} {
				if strings.Contains(all, bad) {
					t.Errorf("%q [%s] addresses the reader by gender (%q): %s", b, code, bad, all)
				}
			}
		}
	}
}

// Combined and tiered offers, with every term, fit 160 / 80 in every locale
// under the same worst-case budgets, and every part is stated.
func TestCombinedAndTieredOffersFitTheLimitsInEveryLocale(t *testing.T) {
	free := offer.Part{Kind: offer.KindFreeShipping}
	offers := map[string]offer.Offer{
		"tier":                        {Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300},
		"tier + free shipping":        {Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300, Extras: []offer.Part{free}},
		"tier + code + last day":      {Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300, Code: "KADEME100", ExpiresAt: "2027-03-08"},
		"percent + free shipping":     {Kind: offer.KindPercent, Value: 20, Extras: []offer.Part{free}},
		"three parts":                 {Kind: offer.KindPercent, Value: 10, Extras: []offer.Part{{Kind: offer.KindAmount, Value: 50}, free}},
		"three parts + every term":    {Kind: offer.KindPercent, Value: 10, Extras: []offer.Part{{Kind: offer.KindAmount, Value: 50}, free}, Code: "BAYRAM10", MinSpend: 1500, ExpiresAt: "2027-04-01"},
		"capped percent + free + day": {Kind: offer.KindPercent, Value: 20, MaxDiscount: 300, Extras: []offer.Part{free}, ExpiresAt: "2027-03-08"},
	}
	briefs := []string{
		"Kadınlar Günü'ne özel kampanya, samimi olsun",
		"Son 30 günde sepete ürün ekleyip almayanlara resmi hatırlatma",
		"Herkese kısa bir kampanya mesajı",
	}
	for _, budgets := range []map[string]int{nil, {"first_name": 30, "city": 20}} {
		p, err := content.NewPolicy(budgets)
		if err != nil {
			t.Fatal(err)
		}
		for name, o := range offers {
			for _, b := range briefs {
				set := writeAll(t, p, b, o)
				for code, c := range set {
					if n := p.EffectiveLength(c.SMSBody); n > content.SMSMaxRunes {
						t.Errorf("%s / %s [%s]: SMS %d", name, b, code, n)
					}
					if n := p.EffectiveLength(c.EmailSubject); n > content.SubjectMaxRunes {
						t.Errorf("%s / %s [%s]: subject %d", name, b, code, n)
					}
				}
				if budgets == nil && b == briefs[0] {
					t.Logf("%s:\n  TR %s\n  EN %s\n  DE %s", name, set["tr-TR"].SMSBody, set["en-GB"].SMSBody, set["de-DE"].SMSBody)
				}
			}
		}
	}
}
