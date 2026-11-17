package content

import (
	"strings"
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

// fallbackIn builds the built-in text of one language in one form.
func fallbackIn(lang string, o offer.Offer, mode int) Content {
	if mode == fallbackMinimal {
		return minimalIn(lang, o)
	}
	personal := mode == fallbackPersonal
	switch lang {
	case "en":
		return fallbackFromWords(o, personal, englishWords)
	case "de":
		return fallbackFromWords(o, personal, germanWords)
	}
	if personal {
		return fallbackTexts(o, "Merhaba {{first_name}}, sana", "{{first_name}}, sana", "Merhaba {{first_name}},", "Sana", "göz at", "ziyaret et")
	}
	return fallbackTexts(o, "Merhaba, size", "Size", "Merhaba,", "Size", "göz atın", "ziyaret edin")
}

// minimalIn is the shortest text that still states everything: the offer with
// its minimum basket, the code and the last day. The body is the plain one.
func minimalIn(lang string, o offer.Offer) Content {
	plain := fallbackIn(lang, o, fallbackPlain)
	what, minP := o.WhatIn(lang), o.MinSpendPhraseIn(lang)
	label := offer.CouponLabelIn(lang)
	headline := o.HeadlineIn(lang)
	if what == "" && o.Kind == offer.KindCouponCode {
		what, headline = label+": "+o.Code, label+" "+o.Code
	}
	if what == "" { // no incentive: the plain text is already short
		return plain
	}
	full := joinSpace(what, minP)
	if lang == "tr" && minP != "" {
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

// FallbackSet is the built-in text for every language, built so that they all
// say the same thing: every language uses the same form, which keeps the
// placeholders identical across languages (see Consistency).
func (p Policy) FallbackSet(o offer.Offer) map[string]Content {
	o = o.Normalize()
	for _, mode := range fallbackModes {
		set := make(map[string]Content, len(Languages))
		ok := true
		for _, l := range Languages {
			norm, rep := p.Validate(fallbackIn(l.Code, o, mode))
			if !rep.OK || len(p.Quality(norm, o)) > 0 {
				ok = false
				break
			}
			set[l.Code] = norm
		}
		if ok {
			return set
		}
	}
	set := make(map[string]Content, len(Languages))
	for _, l := range Languages {
		norm, _ := p.Validate(fallbackIn(l.Code, o, fallbackMinimal))
		set[l.Code] = norm
	}
	return set
}

// fallbackWords is everything a language's built-in text needs. The sentences
// are plain on purpose: they have to pass the same checks as model output.
type fallbackWords struct {
	hello, helloPlain string // greeting with its comma: "Hi {{first_name}},"
	// In the subject the greeting is just the name: "{{first_name}}, ..." and
	// nothing in front of the plain variant (which is capitalised).
	smsNoOffer   string // after the greeting: "we have a special offer for you!"
	smsTail      string // " Come take a look." (dropped when the SMS is short of room)
	smsOffer     string // after the greeting, %s = the offer: "enjoy %s!"
	subjectNo    string // "a special offer for you"
	subjectOffer string // "enjoy %s"
	bodyNoOffer  string // "We have a special campaign for you. Don't miss out, take a look today."
	bodyOffer    string // "We have a special campaign for you: %s."
	bodyTail     string // " Don't miss out, take a look today."
	bye          string
	// join puts the minimum basket next to the offer in this language.
	join func(what, minPhrase string) string
	lang string
}

var englishWords = fallbackWords{
	lang: "en", hello: "Hi {{first_name}},", helloPlain: "Hi,",
	smsNoOffer: "we have a special offer for you!", smsTail: " Come take a look.", smsOffer: "enjoy %s!",
	subjectNo: "a special offer for you", subjectOffer: "enjoy %s",
	bodyNoOffer: "We have a special campaign for you. Don't miss out, take a look today.",
	bodyOffer:   "We have a special campaign for you: %s.", bodyTail: " Don't miss out, take a look today.",
	bye:  "Best regards",
	join: func(what, minP string) string { return joinSpace(what, minP) }, // "30% off on orders of 1000 TL and over"
}

var germanWords = fallbackWords{
	lang: "de", hello: "Hallo {{first_name}},", helloPlain: "Hallo,",
	smsNoOffer: "wir haben ein besonderes Angebot für Sie!", smsTail: " Schauen Sie vorbei.", smsOffer: "freuen Sie sich auf %s!",
	subjectNo: "ein besonderes Angebot für Sie", subjectOffer: "Ihr Angebot: %s",
	bodyNoOffer: "Wir haben eine besondere Aktion für Sie. Verpassen Sie sie nicht und schauen Sie noch heute vorbei.",
	bodyOffer:   "Wir haben eine besondere Aktion für Sie: %s.", bodyTail: " Verpassen Sie sie nicht und schauen Sie noch heute vorbei.",
	bye:  "Viele Grüße",
	join: func(what, minP string) string { return joinSpace(what, minP) }, // "30 % Rabatt ab einem Einkaufswert von 1000 TL"
}

func joinSpace(a, b string) string {
	if b == "" {
		return a
	}
	return a + " " + b
}

func fallbackFromWords(o offer.Offer, personal bool, w fallbackWords) Content {
	what := o.WhatIn(w.lang)
	code := o.Code
	if o.Kind == offer.KindCouponCode {
		code = "" // a coupon-only offer states its code as the offer itself
	}
	minP, expiry := o.MinSpendPhraseIn(w.lang), o.ExpiryPhraseIn(w.lang)
	label := offer.CouponLabelIn(w.lang)

	hello := w.helloPlain
	subjectHello := ""
	if personal {
		hello, subjectHello = w.hello, "{{first_name}}, "
	}
	subject := func(core string) string {
		if personal {
			return subjectHello + core
		}
		return upperFirst(core)
	}

	var sms, subj, body string
	switch {
	case what == "" && o.Kind == offer.KindCouponCode: // the code is the offer
		full := joinSpace(label+" "+o.Code, minP)
		sms = hello + " " + strings.TrimSuffix(w.smsNoOffer, "!") + ": " + full + "!"
		subj = subject(w.subjectNo)
		body = hello + "\n\n" + w.bodyNoOffer + " " + full + "."
	case what == "": // no incentive in the brief: do not invent one
		sms = hello + " " + w.smsNoOffer + w.smsTail
		subj = subject(w.subjectNo)
		body = hello + "\n\n" + w.bodyNoOffer
	default:
		full := w.join(what, minP)
		sms = hello + " " + sprintf1(w.smsOffer, full)
		if minP == "" && expiry == "" {
			sms += w.smsTail
		}
		subj = subject(sprintf1(w.subjectOffer, what))
		body = hello + "\n\n" + sprintf1(w.bodyOffer, full)
		if expiry != "" {
			body += " " + expiry + "."
		}
		body += w.bodyTail
	}
	if code != "" {
		sms += " " + label + ": " + code + "."
		body += " " + label + ": " + code + "."
	}
	if expiry != "" && what != "" {
		sms += " " + expiry + "."
	}
	if what == "" && o.Kind == offer.KindCouponCode && expiry != "" {
		sms += " " + expiry + "."
		body += " " + expiry + "."
	}
	return Content{SMSBody: sms, EmailSubject: subj, EmailBody: body + "\n\n" + w.bye}
}

func sprintf1(format, arg string) string { return strings.Replace(format, "%s", arg, 1) }

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
