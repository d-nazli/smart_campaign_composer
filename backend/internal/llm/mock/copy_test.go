package mock_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/trtext"
)

// writeAll asks the mock for every language the way the service does (with the
// theme read from the brief) and checks each text as the backend would.
func writeAll(t *testing.T, p content.Policy, b string, o offer.Offer) map[string]content.Content {
	t.Helper()
	set := map[string]content.Content{}
	for _, l := range content.Languages {
		out, err := mock.New(0).Messages(context.Background(), llm.MessageRequest{
			Brief: b, Offer: o, Policy: p, Language: l.Code, Theme: brief.ThemeOf(b, l.Code)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		var c content.Content
		if err := json.Unmarshal([]byte(out.Raw), &c); err != nil {
			t.Fatalf("%s: %v", l.Code, err)
		}
		norm, rep := p.Validate(c)
		if !rep.OK {
			t.Fatalf("%q [%s]: breaks a hard rule: %+v\n%+v", b, l.Code, rep.Issues, c)
		}
		if q := p.Quality(norm, o); len(q) != 0 {
			t.Fatalf("%q [%s]: %+v\n%+v", b, l.Code, q, norm)
		}
		set[l.Code] = norm
	}
	if issues := content.Consistency(set); len(issues) != 0 {
		t.Fatalf("%q: the languages disagree: %+v\n%+v", b, issues, set)
	}
	return set
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
		tr := set["tr"]
		t.Logf("%s\n  SMS: %s\n  Konu: %s\n  EN: %s\n  DE: %s", c.brief, tr.SMSBody, tr.EmailSubject, set["en"].SMSBody, set["de"].SMSBody)
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
	if a["tr"] != b["tr"] {
		t.Fatal("same brief, different copy")
	}
	distinct := map[string]bool{}
	for _, br := range []string{"Herkese %15 indirim", "Tüm müşterilere %15 indirim gönder", "Bütün kullanıcılara %15 indirim", "Herkese %15 indirim kuponu", "Herkese yüzde 15 indirim tanımla"} {
		distinct[writeAll(t, p, br, o)["tr"].SMSBody] = true
	}
	if len(distinct) < 3 {
		t.Errorf("five wordings of the same brief gave only %d different SMS", len(distinct))
	}
}

// Formal and friendly registers, and "short" leaves out the call to action.
func TestTheCopyFollowsTheTone(t *testing.T) {
	p := defaultPolicy(t)
	o := offer.Offer{Kind: offer.KindPercent, Value: 10}
	friendly := writeAll(t, p, "Herkese samimi bir dille %10 indirim", o)["tr"]
	formal := writeAll(t, p, "Herkese resmi bir dille %10 indirim", o)["tr"]
	if !strings.Contains(formal.EmailBody, "siz") && !strings.Contains(formal.EmailBody, "Sayın") {
		t.Errorf("formal copy should say siz/Sayın: %q", formal.EmailBody)
	}
	if strings.Contains(friendly.SMSBody, "Sayın") || strings.Contains(friendly.EmailBody, "Saygılarımızla") {
		t.Errorf("friendly copy should not be formal: %+v", friendly)
	}
	short := writeAll(t, p, "Herkese kısa bir %10 indirim mesajı", o)["tr"]
	long := writeAll(t, p, "Herkese %10 indirim mesajı", o)["tr"]
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
		for lang, c := range set {
			all := trtext.Fold(c.SMSBody + " " + c.EmailSubject + " " + c.EmailBody)
			for _, bad := range []string{"kadin musteri", "hanim", "sevgili anne", "sevgili baba", "dear mum", "dear dad", "liebe mutter"} {
				if strings.Contains(all, bad) {
					t.Errorf("%q [%s] addresses the reader by gender (%q): %s", b, lang, bad, all)
				}
			}
		}
	}
}

// Combined and tiered offers, with every term, fit 160 / 80 in every language
// under the same worst-case budgets, and every part is stated.
func TestCombinedAndTieredOffersFitTheLimitsInEveryLanguage(t *testing.T) {
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
				for lang, c := range set {
					if n := p.EffectiveLength(c.SMSBody); n > content.SMSMaxRunes {
						t.Errorf("%s / %s [%s]: SMS %d", name, b, lang, n)
					}
					if n := p.EffectiveLength(c.EmailSubject); n > content.SubjectMaxRunes {
						t.Errorf("%s / %s [%s]: subject %d", name, b, lang, n)
					}
				}
				if budgets == nil && b == briefs[0] {
					t.Logf("%s:\n  TR %s\n  EN %s\n  DE %s", name, set["tr"].SMSBody, set["en"].SMSBody, set["de"].SMSBody)
				}
			}
		}
	}
}
