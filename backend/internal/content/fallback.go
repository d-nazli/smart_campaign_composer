package content

import (
	"strings"

	"smartcampaign/internal/offer"
)

// Fallback returns the built-in template used when the LLM could not produce
// valid content. It states the campaign's offer (a template that silently
// dropped the discount would repeat the very bug the offer check exists for)
// together with its terms (minimum basket, last day), and it is itself
// validated against the policy: it can never break the SMS/subject limits,
// even if budgets are configured unusually large; in that case a
// variable-free template is returned instead.
func (p Policy) Fallback(o offer.Offer) Content {
	o = o.Normalize()
	for _, mode := range fallbackModes {
		if norm, rep := p.Validate(fallbackIn(o, mode)); rep.OK && len(p.Quality(norm, o)) == 0 {
			return norm
		}
	}
	norm, _ := p.Validate(fallbackIn(o, fallbackMinimal))
	return norm
}

func fallbackTexts(o offer.Offer, smsHello, subjectHello, bodyHello, bodySana, lookAt, visit string) Content {
	what, code := offerPhrases(o)
	minPhrase, expiry := o.MinSpendPhrase(), o.ExpiryPhrase()
	var sms, subject, body string
	if what == "" { // no incentive in the brief: do not invent one
		sms = smsHello + " özel bir fırsatımız var! Detaylar için bizi " + visit + "."
		subject = subjectHello + " özel bir fırsat var"
		body = bodyHello + "\n\n" + bodySana + " özel bir kampanyamız var. Fırsatı kaçırmamak için hemen " + lookAt + ".\n\nSevgiler"
	} else {
		// The SMS has the least room: with terms it drops the "see details" tail
		// and puts the minimum in front of the offer.
		full := what
		if minPhrase != "" {
			full = minPhrase + " " + what
		}
		sms = smsHello + " özel " + full + " fırsatı var!"
		if minPhrase == "" && expiry == "" {
			sms += " Detaylar için bizi " + visit + "."
		}
		subject = subjectHello + " özel " + what + " fırsatı"
		offerLine := what
		if minPhrase != "" {
			offerLine = what + " (" + minPhrase + " geçerli)"
		}
		body = bodyHello + "\n\n" + bodySana + " özel bir kampanyamız var: " + offerLine + "."
		if expiry != "" {
			body += " " + expiry + "."
		}
		body += " Fırsatı kaçırmamak için hemen " + lookAt + ".\n\nSevgiler"
	}
	if scope := o.ScopePhraseIn("tr"); scope != "" && what != "" {
		sms += " " + scope + "."
		body = strings.Replace(body, "\n\nSevgiler", " "+scope+".\n\nSevgiler", 1)
	}
	if code != "" {
		sms += " Kupon kodu: " + code + "."
		body = strings.Replace(body, "\n\nSevgiler", " Kupon kodu: "+code+".\n\nSevgiler", 1)
	}
	if expiry != "" && what != "" {
		sms += " " + expiry + "."
	}
	return Content{SMSBody: sms, EmailSubject: subject, EmailBody: body}
}

// offerPhrases returns how the offer reads inside a sentence ("%15 indirim",
// "%20 indirim + ücretsiz kargo", a tier with its cap) and the coupon code to
// append, if any.
func offerPhrases(o offer.Offer) (what, code string) {
	if o.Kind == offer.KindCouponCode {
		return o.Code + " kodlu indirim", ""
	}
	return o.WhatIn("tr"), o.Code
}
