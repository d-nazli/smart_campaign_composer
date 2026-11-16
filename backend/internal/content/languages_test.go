package content

import (
	"strings"
	"testing"
	"unicode/utf8"

	"smartcampaign/internal/offer"
)

// ---- the language list -----------------------------------------------------

func TestSupportedLanguages(t *testing.T) {
	if got := strings.Join(LanguageCodes(), ","); got != "tr,en,de" {
		t.Fatalf("languages = %s", got)
	}
	if DefaultLanguage != "tr" || !IsSupported("EN") || IsSupported("fr") || IsSupported("") {
		t.Fatal("IsSupported / DefaultLanguage")
	}
	for in, want := range map[string]string{"en": "en", " DE ": "de", "fr": "tr", "": "tr", "tr": "tr"} {
		if got := NormalizeLanguage(in); got != want {
			t.Errorf("NormalizeLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---- variables in every language ---------------------------------------------

func TestEveryVariableHasAFallbackInEveryLanguageThatFitsItsBudget(t *testing.T) {
	p, _ := NewPolicy(nil)
	for _, v := range Variables {
		for _, l := range Languages {
			w := v.FallbackFor(l.Code)
			if w == "" {
				t.Errorf("%s has no fallback in %s", v.Name, l.Code)
			}
			if n := utf8.RuneCountInString(w); n > p.Budget(v.Name) {
				t.Errorf("%s fallback in %s has %d characters, budget %d", v.Name, l.Code, n, p.Budget(v.Name))
			}
		}
	}
	// A budget below any language's fallback is refused, not silently overflowed.
	if _, err := NewPolicy(map[string]int{"first_name": 16}); err == nil {
		t.Fatal("a budget smaller than a fallback word must be refused")
	}
}

func TestRenderUsesTheRecipientsLanguageForMissingValues(t *testing.T) {
	p, _ := NewPolicy(nil)
	for lang, want := range map[string]string{
		"tr": "Merhaba değerli müşterimiz",
		"en": "Merhaba valued customer",
		"de": "Merhaba geschätzter Kunde",
		"":   "Merhaba değerli müşterimiz", // no language: the default
		"fr": "Merhaba değerli müşterimiz", // unknown language: the default
	} {
		if got := p.Render("Merhaba {{first_name}}", Recipient{Language: lang}); got != want {
			t.Errorf("language %q: %q, want %q", lang, got, want)
		}
	}
	// A real name is used as it is in every language.
	if got := p.Render("Hi {{first_name}}", Recipient{FirstName: "Ayşe", Language: "en"}); got != "Hi Ayşe" {
		t.Errorf("got %q", got)
	}
}

// ---- the languages must say the same thing -----------------------------------

func set(tr, en, de Content) map[string]Content {
	return map[string]Content{"tr": tr, "en": en, "de": de}
}

var (
	trOK = Content{SMSBody: "{{first_name}}, %30 indirim seni bekliyor! Son kullanım: 31.12.2026",
		EmailSubject: "%30 indirim", EmailBody: "Merhaba {{first_name}},\n\n1000 TL ve üzeri alışverişte %30 indirim."}
	enOK = Content{SMSBody: "{{first_name}}, enjoy 30% off! Valid until: 31.12.2026",
		EmailSubject: "30% off", EmailBody: "Hi {{first_name}},\n\nEnjoy 30% off on orders of 1000 TL and over."}
	deOK = Content{SMSBody: "{{first_name}}, genießen Sie 30 % Rabatt! Gültig bis: 31.12.2026",
		EmailSubject: "30 % Rabatt", EmailBody: "Hallo {{first_name}},\n\nProfitieren Sie von 30 % Rabatt ab einem Einkaufswert von 1000 TL."}
)

func TestLanguagesThatSayTheSameThingAreConsistent(t *testing.T) {
	if issues := Consistency(set(trOK, enOK, deOK)); len(issues) != 0 {
		t.Fatalf("%+v", issues)
	}
	// The way a number is written (%30, 30%, 30 %) does not matter, only its value.
	if issues := Consistency(map[string]Content{"tr": trOK, "de": deOK}); len(issues) != 0 {
		t.Fatalf("a subset of the languages is checked against the reference: %+v", issues)
	}
}

func TestADifferentPromiseInOneLanguageIsAHardError(t *testing.T) {
	change := func(c Content, f func(*Content)) Content { f(&c); return c }
	cases := []struct {
		name     string
		bad      Content
		lang     string
		field    string
		mentions string
	}{
		{"other percentage in the SMS", change(enOK, func(c *Content) { c.SMSBody = strings.Replace(c.SMSBody, "30%", "20%", 1) }), "en", FieldSMSBody, "indirim oranları"},
		{"other percentage in the email", change(deOK, func(c *Content) { c.EmailBody = strings.Replace(c.EmailBody, "30 %", "35 %", 1) }), "de", FieldEmailBody, "indirim oranları"},
		{"other minimum basket", change(enOK, func(c *Content) { c.EmailBody = strings.Replace(c.EmailBody, "1000 TL", "500 TL", 1) }), "en", FieldEmailBody, "tutarlar"},
		{"an invented amount", change(deOK, func(c *Content) { c.SMSBody += " Nur 50 TL!" }), "de", FieldSMSBody, "tutarlar"},
		{"other last day", change(enOK, func(c *Content) { c.SMSBody = strings.Replace(c.SMSBody, "31.12.2026", "30.12.2026", 1) }), "en", FieldSMSBody, "tarihler"},
		{"a date missing", change(deOK, func(c *Content) { c.SMSBody = strings.Replace(c.SMSBody, " Gültig bis: 31.12.2026", "", 1) }), "de", FieldSMSBody, "tarihler"},
		{"placeholder missing in the SMS", change(enOK, func(c *Content) { c.SMSBody = strings.Replace(c.SMSBody, "{{first_name}}, ", "", 1) }), "en", FieldSMSBody, "değişkenler"},
		{"another placeholder", change(deOK, func(c *Content) { c.EmailBody += " Aus {{city}}." }), "de", FieldEmailBody, "değişkenler"},
		{"free shipping promised in one language only", change(enOK, func(c *Content) { c.EmailBody += " Free shipping!" }), "en", FieldEmailBody, "ücretsiz kargo"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := set(trOK, enOK, deOK)
			s[tt.lang] = tt.bad
			issues := Consistency(s)
			if len(issues) == 0 {
				t.Fatal("expected a mismatch")
			}
			i := issues[0]
			if i.Code != CodeLanguageMismatch || i.Language != tt.lang || i.Field != tt.field || !strings.Contains(i.Message, tt.mentions) {
				t.Fatalf("got %+v, want a %s issue for %s/%s mentioning %q", i, CodeLanguageMismatch, tt.lang, tt.field, tt.mentions)
			}
			for _, other := range issues {
				if other.Language != tt.lang {
					t.Errorf("the languages that are fine must not be blamed: %+v", other)
				}
			}
		})
	}
}

// The reference is the default language; the others are compared with it, so a
// bad reference is reported on every other language rather than hidden.
func TestTheReferenceLanguageIsTheDefaultOne(t *testing.T) {
	badTR := trOK
	badTR.SMSBody = strings.Replace(badTR.SMSBody, "%30", "%40", 1)
	issues := Consistency(set(badTR, enOK, deOK))
	langs := map[string]bool{}
	for _, i := range issues {
		langs[i.Language] = true
	}
	if !langs["en"] || !langs["de"] || langs["tr"] {
		t.Fatalf("%+v", issues)
	}
}

// ---- the built-in text -------------------------------------------------------

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
}

func TestTheBuiltInTextIsValidAndStatesTheOfferInEveryLanguage(t *testing.T) {
	p, _ := NewPolicy(nil)
	for name, o := range fallbackOffers {
		t.Run(name, func(t *testing.T) {
			set := p.FallbackSet(o)
			if len(set) != len(Languages) {
				t.Fatalf("got %d languages", len(set))
			}
			for _, l := range Languages {
				c := set[l.Code]
				if norm, rep := p.Validate(c); !rep.OK || norm != c {
					t.Fatalf("%s: %+v %+v", l.Code, rep.Issues, c)
				}
				if q := p.Quality(c, o); len(q) != 0 {
					t.Fatalf("%s: %+v\n%+v", l.Code, q, c)
				}
				if strings.Contains(c.SMSBody+c.EmailSubject+c.EmailBody, "%s") {
					t.Fatalf("%s: unfilled template: %+v", l.Code, c)
				}
			}
			if issues := Consistency(set); len(issues) != 0 {
				t.Fatalf("the languages disagree: %+v", issues)
			}
		})
	}
}

func TestTheBuiltInTextReadsAsEachLanguage(t *testing.T) {
	p, _ := NewPolicy(nil)
	set := p.FallbackSet(offer.Offer{Kind: offer.KindPercent, Value: 30})
	for lang, want := range map[string]string{"tr": "%30 indirim", "en": "30% off", "de": "30 % Rabatt"} {
		if !strings.Contains(set[lang].SMSBody, want) {
			t.Errorf("%s SMS %q should contain %q", lang, set[lang].SMSBody, want)
		}
	}
}

// With unusually large budgets the personalised text may not fit; then EVERY
// language falls back to the plain text, so they still agree.
func TestOversizedBudgetsMakeEveryLanguagePlainTogether(t *testing.T) {
	p, err := NewPolicy(map[string]int{"first_name": 100})
	if err != nil {
		t.Fatal(err)
	}
	o := offer.Offer{Kind: offer.KindPercent, Value: 15, Code: "SUPERLONGCOUPONCODE", MinSpend: 1000, ExpiresAt: "2026-12-31"}
	set := p.FallbackSet(o)
	plain := 0
	for _, l := range Languages {
		if !strings.Contains(set[l.Code].SMSBody, "{{") {
			plain++
		}
		if _, rep := p.Validate(set[l.Code]); !rep.OK {
			t.Fatalf("%s: %+v", l.Code, rep.Issues)
		}
	}
	if plain != 0 && plain != len(Languages) {
		t.Fatalf("%d of %d languages are plain: they would not agree on placeholders", plain, len(Languages))
	}
	if issues := Consistency(set); len(issues) != 0 {
		t.Fatalf("%+v", issues)
	}
}

// ---- the offer's words -----------------------------------------------------

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
		// Whatever the language writes, the language-neutral readers find the same offer in it.
		text := strings.Join(got[:], ". ")
		if !o.Mentions(text) {
			t.Errorf("%s: the offer does not read its own phrases back: %q", lang, text)
		}
	}
	if (offer.Offer{Kind: offer.KindFreeShipping}).WhatIn("de") != "kostenloser Versand" {
		t.Error("free shipping in German")
	}
	for _, text := range []string{"free shipping on all orders", "Kostenloser Versand für Sie", "Versandkostenfrei!", "ücretsiz kargo"} {
		if !offer.HasFreeShipping(text) {
			t.Errorf("%q is a free shipping promise", text)
		}
	}
}
