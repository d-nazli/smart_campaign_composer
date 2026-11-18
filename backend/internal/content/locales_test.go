package content

import (
	"strings"
	"testing"

	"smartcampaign/internal/offer"
)

// testLocales mirrors a few rows of the locales table.
func testLocales() []Locale {
	return []Locale{
		{Code: "en-GB", Name: "English (United Kingdom)", NativeName: "English (UK)", Direction: "ltr",
			Fallbacks: map[string]string{"first_name": "valued customer", "city": "your area"}, FreeShippingTerms: []string{"free delivery", "free shipping"}},
		{Code: "tr-TR", Name: "Turkish (Türkiye)", NativeName: "Türkçe (Türkiye)", Direction: "ltr", IsSource: true,
			Fallbacks: map[string]string{"first_name": "değerli müşterimiz", "city": "bölgeniz"}, FreeShippingTerms: []string{"ücretsiz kargo"}},
		{Code: "de-DE", Name: "German (Germany)", NativeName: "Deutsch (Deutschland)", Direction: "ltr",
			Fallbacks: map[string]string{"first_name": "geschätzter Kunde", "city": "Ihrer Region"}, FreeShippingTerms: []string{"kostenloser versand"}},
		{Code: "fr-FR", Name: "French (France)", NativeName: "Français (France)", Direction: "ltr",
			Fallbacks: map[string]string{"first_name": "cher client", "city": "votre région"}, FreeShippingTerms: []string{"livraison gratuite"}},
		{Code: "ar-SA", Name: "Arabic (Saudi Arabia)", NativeName: "العربية (السعودية)", Direction: "rtl",
			Fallbacks: map[string]string{"first_name": "عميلنا العزيز", "city": "منطقتك"}},
	}
}

func mustLocales(t *testing.T) Locales {
	t.Helper()
	p, _ := NewPolicy(nil)
	ls, err := NewLocales(testLocales(), p)
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

func loc(t *testing.T, code string) Locale {
	l, ok := mustLocales(t).Get(code)
	if !ok {
		t.Fatalf("no locale %s", code)
	}
	return l
}

// ---- the locale set ------------------------------------------------------------

func TestLocalesPutTheSourceFirstAndTheRestInCodeOrder(t *testing.T) {
	ls := mustLocales(t)
	if got := strings.Join(ls.Codes(), ","); got != "tr-TR,ar-SA,de-DE,en-GB,fr-FR" {
		t.Fatalf("codes = %s", got)
	}
	if ls.Source().Code != "tr-TR" || loc(t, "en-GB").Lang() != "en" {
		t.Fatal("source / language part")
	}
	if _, ok := ls.Get("en"); ok {
		t.Fatal("only exact codes match")
	}
	if got := ls.Ordered(map[string]bool{"fr-FR": true, "tr-TR": true, "xx-XX": true}); strings.Join(got, ",") != "tr-TR,fr-FR" {
		t.Fatalf("ordered = %v", got)
	}
}

func TestABrokenLocalesTableIsRefused(t *testing.T) {
	p, _ := NewPolicy(nil)
	change := func(f func([]Locale) []Locale) []Locale { return f(testLocales()) }
	for name, list := range map[string][]Locale{
		"no source":           change(func(l []Locale) []Locale { l[1].IsSource = false; return l }),
		"two sources":         change(func(l []Locale) []Locale { l[0].IsSource = true; return l }),
		"bad code":            change(func(l []Locale) []Locale { l[0].Code = "en_GB"; return l }),
		"duplicate code":      change(func(l []Locale) []Locale { l[2].Code = "en-GB"; return l }),
		"bad direction":       change(func(l []Locale) []Locale { l[0].Direction = "up"; return l }),
		"missing fallback":    change(func(l []Locale) []Locale { delete(l[0].Fallbacks, "city"); return l }),
		"fallback too long":   change(func(l []Locale) []Locale { l[0].Fallbacks["first_name"] = "our most valued customer"; return l }),
		"blank fallback word": change(func(l []Locale) []Locale { l[0].Fallbacks["first_name"] = "  "; return l }),
	} {
		if _, err := NewLocales(list, p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A smaller budget turns a fitting fallback into one that does not fit.
	small, err := NewPolicy(map[string]int{"first_name": 18})
	if err != nil {
		t.Fatal(err)
	}
	list := testLocales()
	list[2].Fallbacks["first_name"] = "sehr geschätzter Kunde"
	if _, err := NewLocales(list, small); err == nil {
		t.Fatal("a fallback over the budget must be refused")
	}
}

// ---- variables ---------------------------------------------------------------

func TestRenderUsesTheRecipientsLocaleForMissingValues(t *testing.T) {
	p, _ := NewPolicy(nil)
	for code, want := range map[string]string{
		"tr-TR": "Merhaba değerli müşterimiz",
		"en-GB": "Merhaba valued customer",
		"de-DE": "Merhaba geschätzter Kunde",
		"ar-SA": "Merhaba عميلنا العزيز",
	} {
		if got := p.Render("Merhaba {{first_name}}", Recipient{Fallbacks: loc(t, code).Fallbacks}); got != want {
			t.Errorf("%s: %q, want %q", code, got, want)
		}
	}
	if got := p.Render("Merhaba {{first_name}}", Recipient{}); got != "Merhaba değerli müşterimiz" {
		t.Errorf("no locale words: the source word, got %q", got)
	}
	if got := p.Render("Hi {{first_name}}", Recipient{FirstName: "Ayşe", Fallbacks: loc(t, "en-GB").Fallbacks}); got != "Hi Ayşe" {
		t.Errorf("got %q", got)
	}
}

func TestVariableInfosListEveryLocalesWord(t *testing.T) {
	p, _ := NewPolicy(nil)
	infos := p.InfosFor(mustLocales(t))
	if infos[0].Name != "first_name" || infos[0].Fallbacks["fr-FR"] != "cher client" || len(infos[0].Fallbacks) != 5 {
		t.Fatalf("%+v", infos[0])
	}
	if p.Infos()[0].Fallbacks != nil {
		t.Fatal("without locales there are no per-locale words")
	}
}

// ---- a translation must say what the source says -------------------------------

var (
	trOK = Content{SMSBody: "{{first_name}}, %30 indirim seni bekliyor! Son kullanım: 31.12.2026",
		EmailSubject: "%30 indirim", EmailBody: "Merhaba {{first_name}},\n\n1000 TL ve üzeri alışverişte %30 indirim."}
	enOK = Content{SMSBody: "{{first_name}}, enjoy 30% off! Valid until: 31.12.2026",
		EmailSubject: "30% off", EmailBody: "Hi {{first_name}},\n\nEnjoy 30% off on orders of 1000 TL and over."}
	deOK = Content{SMSBody: "{{first_name}}, genießen Sie 30 % Rabatt! Gültig bis: 31.12.2026",
		EmailSubject: "30 % Rabatt", EmailBody: "Hallo {{first_name}},\n\nProfitieren Sie von 30 % Rabatt ab einem Einkaufswert von 1000 TL."}
	arOK = Content{SMSBody: "{{first_name}}، خصم 30% بانتظارك! صالح حتى 31.12.2026",
		EmailSubject: "خصم 30%", EmailBody: "مرحبًا {{first_name}}،\n\nخصم 30% على الطلبات من 1000 TL."}
)

func TestTranslationsThatSayTheSameThingAreConsistent(t *testing.T) {
	src := loc(t, "tr-TR")
	for code, c := range map[string]Content{"en-GB": enOK, "de-DE": deOK, "ar-SA": arOK} {
		if issues := ConsistentWith(src, trOK, loc(t, code), c); len(issues) != 0 {
			t.Errorf("%s: %+v", code, issues)
		}
	}
}

func TestADifferentPromiseInATranslationIsAHardError(t *testing.T) {
	change := func(c Content, f func(*Content)) Content { f(&c); return c }
	cases := []struct {
		name     string
		bad      Content
		code     string
		field    string
		mentions string
	}{
		{"other percentage in the SMS", change(enOK, func(c *Content) { c.SMSBody = strings.Replace(c.SMSBody, "30%", "20%", 1) }), "en-GB", FieldSMSBody, "indirim oranları"},
		{"other percentage in the email", change(deOK, func(c *Content) { c.EmailBody = strings.Replace(c.EmailBody, "30 %", "35 %", 1) }), "de-DE", FieldEmailBody, "indirim oranları"},
		{"other minimum basket", change(enOK, func(c *Content) { c.EmailBody = strings.Replace(c.EmailBody, "1000 TL", "500 TL", 1) }), "en-GB", FieldEmailBody, "tutarlar"},
		{"an invented amount", change(deOK, func(c *Content) { c.SMSBody += " Nur 50 TL!" }), "de-DE", FieldSMSBody, "tutarlar"},
		{"other last day", change(enOK, func(c *Content) { c.SMSBody = strings.Replace(c.SMSBody, "31.12.2026", "30.12.2026", 1) }), "en-GB", FieldSMSBody, "tarihler"},
		{"{{first_name}} lost in the SMS", change(arOK, func(c *Content) { c.SMSBody = strings.Replace(c.SMSBody, "{{first_name}}، ", "", 1) }), "ar-SA", FieldSMSBody, "değişkenler"},
		{"another placeholder", change(deOK, func(c *Content) { c.EmailBody += " Aus {{city}}." }), "de-DE", FieldEmailBody, "değişkenler"},
		{"free shipping promised in the translation only", change(enOK, func(c *Content) { c.EmailBody += " Free delivery!" }), "en-GB", FieldEmailBody, "ücretsiz kargo"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			issues := ConsistentWith(loc(t, "tr-TR"), trOK, loc(t, tt.code), tt.bad)
			if len(issues) == 0 {
				t.Fatal("expected a mismatch")
			}
			i := issues[0]
			if i.Code != CodeLocaleMismatch || i.Locale != tt.code || i.Field != tt.field || !strings.Contains(i.Message, tt.mentions) {
				t.Fatalf("got %+v, want a %s issue for %s/%s mentioning %q", i, CodeLocaleMismatch, tt.code, tt.field, tt.mentions)
			}
		})
	}
}

// Free shipping is read with each locale's own words; a locale that has none
// cannot be compared on it (and is not blamed).
func TestFreeShippingIsReadWithTheLocalesWords(t *testing.T) {
	src := loc(t, "tr-TR")
	tr := Content{SMSBody: "%20 indirim ve ücretsiz kargo!", EmailSubject: "Ücretsiz kargo", EmailBody: "%20 indirim ve ücretsiz kargo."}
	fr := Content{SMSBody: "-20 % et livraison gratuite !", EmailSubject: "Livraison gratuite", EmailBody: "-20 % et livraison gratuite."}
	if issues := ConsistentWith(src, tr, loc(t, "fr-FR"), fr); len(issues) != 0 {
		t.Fatalf("%+v", issues)
	}
	frNo := Content{SMSBody: "-20 % !", EmailSubject: "Promo", EmailBody: "-20 %."}
	if issues := ConsistentWith(src, tr, loc(t, "fr-FR"), frNo); len(issues) != 2 {
		t.Fatalf("free shipping dropped in the SMS and the email: %+v", issues)
	}
	ar := Content{SMSBody: "خصم 20%", EmailSubject: "عرض", EmailBody: "خصم 20%"}
	if issues := ConsistentWith(src, tr, loc(t, "ar-SA"), ar); len(issues) != 0 {
		t.Fatalf("ar-SA has no free-shipping words in this table: %+v", issues)
	}
}

// ---- the built-in source text ------------------------------------------------

var fallbackOffers = map[string]offer.Offer{
	"none":            {Kind: offer.KindNone},
	"percent":         {Kind: offer.KindPercent, Value: 30},
	"percent+code":    {Kind: offer.KindPercent, Value: 15, Code: "YAZ25"},
	"decimal percent": {Kind: offer.KindPercent, Value: 12.5},
	"amount":          {Kind: offer.KindAmount, Value: 100},
	"free shipping":   {Kind: offer.KindFreeShipping},
	"coupon only":     {Kind: offer.KindCouponCode, Code: "MERHABA10"},
	"terms":           {Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"},
	"terms+code":      {Kind: offer.KindPercent, Value: 30, Code: "ANNELER", MinSpend: 1000, ExpiresAt: "2026-05-10"},
	"coupon+terms":    {Kind: offer.KindCouponCode, Code: "YAZ25", MinSpend: 750, ExpiresAt: "2026-08-31"},
	// combined and tiered offers: every part and every number must survive
	"percent+free shipping": {Kind: offer.KindPercent, Value: 20, Extras: []offer.Part{{Kind: offer.KindFreeShipping}}},
	"tier":                  {Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300},
	"tier+code+last day":    {Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300, Code: "KADEME100", ExpiresAt: "2027-03-08"},
	"three parts+terms": {Kind: offer.KindPercent, Value: 10, Code: "BAYRAM10", MinSpend: 1500, ExpiresAt: "2027-04-01",
		Extras: []offer.Part{{Kind: offer.KindAmount, Value: 50}, {Kind: offer.KindFreeShipping}}},
	"capped percent": {Kind: offer.KindPercent, Value: 20, MaxDiscount: 300},
}

func TestTheBuiltInTextIsValidAndStatesTheOffer(t *testing.T) {
	p, _ := NewPolicy(nil)
	for name, o := range fallbackOffers {
		t.Run(name, func(t *testing.T) {
			c := p.Fallback(o)
			if norm, rep := p.Validate(c); !rep.OK || norm != c {
				t.Fatalf("%+v %+v", rep.Issues, c)
			}
			if q := p.Quality(c, o); len(q) != 0 {
				t.Fatalf("%+v\n%+v", q, c)
			}
			if strings.Contains(c.SMSBody+c.EmailSubject+c.EmailBody, "%s") {
				t.Fatalf("unfilled template: %+v", c)
			}
		})
	}
}

// With unusually large budgets the personalised text may not fit; the plain one is used.
func TestOversizedBudgetsMakeTheBuiltInTextPlain(t *testing.T) {
	p, err := NewPolicy(map[string]int{"first_name": 100})
	if err != nil {
		t.Fatal(err)
	}
	o := offer.Offer{Kind: offer.KindPercent, Value: 15, Code: "SUPERLONGCOUPONCODE", MinSpend: 1000, ExpiresAt: "2026-12-31"}
	c := p.Fallback(o)
	if _, rep := p.Validate(c); !rep.OK {
		t.Fatalf("%+v", rep.Issues)
	}
}

// ---- the offer's words (used by the mock's phrase books) ----------------------

func TestOfferPhrasesPerLanguage(t *testing.T) {
	o := offer.Offer{Kind: offer.KindPercent, Value: 12.5, MinSpend: 1000, ExpiresAt: "2026-12-31"}
	want := map[string][3]string{
		"tr": {"%12,5 indirim", "1000 TL ve üzeri alışverişte", "Son kullanım: 31.12.2026"},
		"en": {"12.5% off", "on orders of 1000 TL and over", "Valid until: 31.12.2026"},
		"de": {"12,5 % Rabatt", "ab einem Einkaufswert von 1000 TL", "Gültig bis: 31.12.2026"},
	}
	for lang, w := range want {
		got := [3]string{o.WhatIn(lang), o.MinSpendPhraseIn(lang), o.ExpiryPhraseIn(lang)}
		if got != w {
			t.Errorf("%s: %q, want %q", lang, got, w)
		}
		text := strings.Join(got[:], ". ")
		if !o.Mentions(text) {
			t.Errorf("%s: the offer does not read its own phrases back: %q", lang, text)
		}
	}
	for _, text := range []string{"free shipping on all orders", "Kostenloser Versand für Sie", "Versandkostenfrei!", "ücretsiz kargo"} {
		if !offer.HasFreeShipping(text) {
			t.Errorf("%q is a free shipping promise", text)
		}
	}
	if !offer.HasFreeShippingIn("Livraison GRATUITE dès maintenant", []string{"livraison gratuite"}) || offer.HasFreeShippingIn("-20 %", []string{"livraison gratuite"}) {
		t.Error("a locale's own words")
	}
}
