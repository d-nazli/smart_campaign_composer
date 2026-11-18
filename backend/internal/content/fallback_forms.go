package content

import (
	"unicode"

	"smartcampaign/internal/offer"
)

// The built-in text comes in three forms, tried in this order: personalised
// ({{first_name}} in the SMS greeting, the subject and the body), plain (no
// variable at all) and minimal (the offer and its terms, nothing else: for an
// offer with many parts and terms that would not fit 160 / 80 otherwise).
const (
	fallbackPersonal = iota
	fallbackPlain
	fallbackMinimal
)

var fallbackModes = []int{fallbackPersonal, fallbackPlain, fallbackMinimal}

// fallbackIn builds the built-in source-locale (Turkish) text in one form.
// Translations are never built from templates: they come from the translator.
func fallbackIn(o offer.Offer, mode int) Content {
	if mode == fallbackMinimal {
		return minimalIn(o)
	}
	if mode == fallbackPersonal {
		return fallbackTexts(o, "Merhaba {{first_name}}, sana", "{{first_name}}, sana", "Merhaba {{first_name}},", "Sana", "göz at", "ziyaret et")
	}
	return fallbackTexts(o, "Merhaba, size", "Size", "Merhaba,", "Size", "göz atın", "ziyaret edin")
}

// minimalIn is the shortest text that still states everything: the offer with
// its minimum basket, the code and the last day. The body is the plain one.
func minimalIn(o offer.Offer) Content {
	const lang = "tr"
	plain := fallbackIn(o, fallbackPlain)
	what, minP := o.WhatIn(lang), o.MinSpendPhraseIn(lang)
	label := offer.CouponLabelIn(lang)
	headline := o.HeadlineIn(lang)
	if what == "" && o.Kind == offer.KindCouponCode {
		what, headline = label+": "+o.Code, label+" "+o.Code
	}
	if what == "" { // no incentive: the plain text is already short
		return plain
	}
	full := what
	if minP != "" {
		full = minP + " " + what
	}
	sms := upperFirst(full) + "!"
	if o.Code != "" && o.Kind != offer.KindCouponCode {
		sms += " " + label + ": " + o.Code + "."
	}
	if e := o.ExpiryPhraseIn(lang); e != "" {
		sms += " " + e + "."
	}
	return Content{SMSBody: sms, EmailSubject: upperFirst(headline), EmailBody: plain.EmailBody}
}

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
