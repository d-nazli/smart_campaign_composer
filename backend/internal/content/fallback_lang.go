package content

import (
	"strings"
	"unicode"

	"smartcampaign/internal/offer"
)

// fallbackIn builds the built-in text of one language. personal puts
// {{first_name}} in the greeting of the SMS, the subject and the body; the
// plain variant has no variable at all.
func fallbackIn(lang string, o offer.Offer, personal bool) Content {
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

// FallbackSet is the built-in text for every language, built so that they all
// say the same thing: either every language is personalised or none is, which
// keeps the placeholders identical across languages (see Consistency).
func (p Policy) FallbackSet(o offer.Offer) map[string]Content {
	o = o.Normalize()
	set := make(map[string]Content, len(Languages))
	ok := true
	for _, l := range Languages {
		c := fallbackIn(l.Code, o, true)
		norm, rep := p.Validate(c)
		if !rep.OK || len(p.Quality(norm, o)) > 0 {
			ok = false
			break
		}
		set[l.Code] = norm
	}
	if ok {
		return set
	}
	for _, l := range Languages {
		norm, _ := p.Validate(fallbackIn(l.Code, o, false))
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
