package content

import (
	"strings"

	"smartcampaign/internal/offer"
)

// Fallback returns the built-in template used when the LLM could not produce
// valid content. It states the campaign's offer (a template that silently
// dropped the discount would repeat the very bug the offer check exists for)
// and it is itself validated against the policy: it can never break the
// SMS/subject limits, even if budgets are configured unusually large; in that
// case a variable-free template is returned instead.
func (p Policy) Fallback(o offer.Offer) Content {
	o = o.Normalize()
	personal := fallbackTexts(o, "Merhaba {{first_name}}, sana", "{{first_name}}, sana", "Merhaba {{first_name}},", "Sana", "göz at", "ziyaret et")
	if norm, rep := p.Validate(personal); rep.OK && len(p.Quality(norm, o)) == 0 {
		return norm
	}
	plain := fallbackTexts(o, "Merhaba, size", "Size", "Merhaba,", "Size", "göz atın", "ziyaret edin")
	norm, _ := p.Validate(plain)
	return norm
}

func fallbackTexts(o offer.Offer, smsHello, subjectHello, bodyHello, bodySana, lookAt, visit string) Content {
	what, code := offerPhrases(o)
	var sms, subject, body string
	if what == "" { // no incentive in the brief: do not invent one
		sms = smsHello + " özel bir fırsatımız var! Detaylar için bizi " + visit + "."
		subject = subjectHello + " özel bir fırsat var"
		body = bodyHello + "\n\n" + bodySana + " özel bir kampanyamız var. Fırsatı kaçırmamak için hemen " + lookAt + ".\n\nSevgiler"
	} else {
		sms = smsHello + " özel " + what + " fırsatı var! Detaylar için bizi " + visit + "."
		subject = subjectHello + " özel " + what + " fırsatı"
		body = bodyHello + "\n\n" + bodySana + " özel bir kampanyamız var: " + what + ". Fırsatı kaçırmamak için hemen " + lookAt + ".\n\nSevgiler"
	}
	if code != "" {
		sms += " Kupon kodu: " + code + "."
		body = strings.Replace(body, "\n\nSevgiler", " Kupon kodu: "+code+".\n\nSevgiler", 1)
	}
	return Content{SMSBody: sms, EmailSubject: subject, EmailBody: body}
}

// offerPhrases returns how the offer reads inside a sentence ("%15 indirim",
// "ücretsiz kargo") and the coupon code to append, if any.
func offerPhrases(o offer.Offer) (what, code string) {
	switch o.Kind {
	case offer.KindPercent:
		return "%" + offer.FormatNumber(o.Value) + " indirim", o.Code
	case offer.KindAmount:
		return offer.FormatNumber(o.Value) + " TL indirim", o.Code
	case offer.KindFreeShipping:
		return "ücretsiz kargo", o.Code
	case offer.KindCouponCode:
		return o.Code + " kodlu indirim", ""
	}
	return "", ""
}
