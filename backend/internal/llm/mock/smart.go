package mock

import (
	"encoding/json"
	"strings"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// smartSegment answers the way a flawless model would: it uses the same brief
// reader the backend verifies the real model against. The mock is a demo and
// test double, so it is deterministic; the interesting failure paths (a model
// that drops a condition or the offer) are scripted in tests with Raw steps.
func smartSegment(text string) string {
	raw, _ := json.Marshal(brief.Analyze(text).Rule())
	return string(raw)
}

type messagesJSON struct {
	SMSBody      string `json:"sms_body"`
	EmailSubject string `json:"email_subject"`
	EmailBody    string `json:"email_body"`
}

// smartMessages writes believable Turkish copy that follows the brief's tone
// and states the offer, always inside the SMS/subject limits and without
// repeating words.
func smartMessages(text string, o offer.Offer) string {
	b := segment.Fold(text)
	informal := strings.Contains(b, "samimi") || strings.Contains(b, "sen ")
	short := strings.Contains(b, "kisa")
	cart := strings.Contains(b, "sepet")
	o = o.Normalize()
	what, code := offerWhat(o)

	var m messagesJSON
	switch {
	case cart && informal:
		m.SMSBody = "{{first_name}}, sepetindeki ürünler seni bekliyor! " + offerSentence(o, what, code, true)
		m.EmailSubject = subjectFor("{{first_name}}, sepetindekiler seni bekliyor", "{{first_name}}, sepetindekilerde ", what, o)
		m.EmailBody = "Merhaba {{first_name}},\n\nSepetine eklediğin ürünler hâlâ seni bekliyor. " + offerSentence(o, what, code, true) +
			"\n\nAlışverişini tamamlamak için sepetine göz atman yeterli.\n\nGörüşmek üzere!"
	case cart:
		m.SMSBody = "Sayın {{first_name}}, sepetinizdeki ürünler sizi bekliyor. " + offerSentence(o, what, code, false)
		m.EmailSubject = subjectFor("Sepetinizdeki ürünler sizi bekliyor", "Sepetinizdekilerde ", what, o)
		m.EmailBody = "Sayın {{first_name}},\n\nSepetinize eklediğiniz ürünler hâlâ sizi bekliyor. " + offerSentence(o, what, code, false) +
			"\n\nAlışverişinizi tamamlamak için sepetinize göz atabilirsiniz.\n\nSaygılarımızla"
	case informal:
		m.SMSBody = "{{first_name}}, sana özel " + specialLine(o, what, "seni bekliyor!") + codeSuffix(o, code)
		m.EmailSubject = subjectFor("{{first_name}}, sana özel bir fırsat var", "{{first_name}}, sana özel ", what, o)
		m.EmailBody = "Merhaba {{first_name}},\n\nSana özel bir kampanyamız var. " + offerSentence(o, what, code, true) + "\n\nGörüşmek üzere!"
	default:
		m.SMSBody = "Sayın {{first_name}}, size özel " + specialLine(o, what, "sizi bekliyor.") + codeSuffix(o, code)
		m.EmailSubject = subjectFor("Size özel bir fırsat var", "Size özel ", what, o)
		m.EmailBody = "Sayın {{first_name}},\n\nSize özel bir kampanyamız var. " + offerSentence(o, what, code, false) + "\n\nSaygılarımızla"
	}
	if short {
		for _, cut := range []struct{ from, to string }{
			{"\n\nAlışverişini", "\n\nGörüşmek üzere!"},
			{"\n\nAlışverişinizi", "\n\nSaygılarımızla"},
		} {
			if i := strings.Index(m.EmailBody, cut.from); i > 0 {
				m.EmailBody = m.EmailBody[:i] + cut.to
			}
		}
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

// offerWhat is how the offer reads inside a sentence, plus the code to append.
func offerWhat(o offer.Offer) (what, code string) {
	switch o.Kind {
	case offer.KindPercent:
		return "%" + offer.FormatNumber(o.Value) + " indirim", o.Code
	case offer.KindAmount:
		return offer.FormatNumber(o.Value) + " TL indirim", o.Code
	case offer.KindFreeShipping:
		return "ücretsiz kargo", o.Code
	case offer.KindCouponCode:
		return "", o.Code
	}
	return "", ""
}

func codeSuffix(o offer.Offer, code string) string {
	if code == "" || o.Kind == offer.KindCouponCode {
		return ""
	}
	return " Kupon kodu: " + code + "."
}

// offerSentence is the sentence that states the offer in the closing tone.
func offerSentence(o offer.Offer, what, code string, informal bool) string {
	switch {
	case what != "" && informal:
		return capitalizeFirst(what) + " fırsatını kaçırma." + codeSuffix(o, code)
	case what != "":
		return capitalizeFirst(what) + " fırsatından yararlanın." + codeSuffix(o, code)
	case o.Kind == offer.KindCouponCode && informal:
		return code + " koduyla fırsatı yakala."
	case o.Kind == offer.KindCouponCode:
		return code + " koduyla fırsatı yakalayın."
	case informal:
		return "Fırsatı kaçırma."
	}
	return "Fırsatı kaçırmayın."
}

// specialLine completes "sana özel ..." / "size özel ...".
func specialLine(o offer.Offer, what, verb string) string {
	switch {
	case what != "":
		return what + " " + verb
	case o.Kind == offer.KindCouponCode:
		return o.Code + " kodu " + verb
	}
	return "bir fırsat " + verb
}

// subjectFor is the email subject: the plain one when there is no offer, else
// a subject that states it.
func subjectFor(plain, prefix, what string, o offer.Offer) string {
	switch {
	case what != "" && strings.HasSuffix(prefix, "özel "):
		return prefix + what
	case what != "":
		return prefix + what + " var"
	case o.Kind == offer.KindCouponCode && strings.HasSuffix(prefix, "özel "):
		return prefix + o.Code + " kodu"
	case o.Kind == offer.KindCouponCode:
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(prefix, "de "), "da ")) + " için " + o.Code + " kodu hazır"
	}
	return plain
}

func capitalizeFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
