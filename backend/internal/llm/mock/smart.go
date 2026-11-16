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
	rule := brief.Analyze(text).Rule()
	// The one-click suggestion is decided by code, never by a model, and the
	// strict parser rejects it in model output; the mock imitates a model.
	rule.Suggestion = ""
	raw, _ := json.Marshal(rule)
	return string(raw)
}

type messagesJSON struct {
	SMSBody      string `json:"sms_body"`
	EmailSubject string `json:"email_subject"`
	EmailBody    string `json:"email_body"`
}

// smartMessagesFor writes the copy in the requested language.
func smartMessagesFor(lang, text string, o offer.Offer) string {
	if lang == "en" || lang == "de" {
		return smartMessagesIn(lang, text, o)
	}
	return smartMessages(text, o)
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
	minP, expiry := o.MinSpendPhrase(), o.ExpiryPhrase()
	full := what // the offer with its minimum basket in front: "1000 TL ve üzeri alışverişte 100 TL indirim"
	switch {
	case what != "" && minP != "":
		full = minP + " " + what
	case what == "" && o.Kind == offer.KindCouponCode && minP != "":
		full = minP + " " + o.Code + " kodu"
	}

	var m messagesJSON
	switch {
	case cart && informal:
		m.SMSBody = "{{first_name}}, sepetindeki ürünler seni bekliyor! " + offerSentence(o, full, code, true, minP, expiry)
		m.EmailSubject = subjectFor("{{first_name}}, sepetindekiler seni bekliyor", "{{first_name}}, sepetindekilerde ", what, o)
		m.EmailBody = "Merhaba {{first_name}},\n\nSepetine eklediğin ürünler hâlâ seni bekliyor. " + offerSentence(o, full, code, true, minP, expiry) +
			"\n\nAlışverişini tamamlamak için sepetine göz atman yeterli.\n\nGörüşmek üzere!"
	case cart:
		m.SMSBody = "Sayın {{first_name}}, sepetinizdeki ürünler sizi bekliyor. " + offerSentence(o, full, code, false, minP, expiry)
		m.EmailSubject = subjectFor("Sepetinizdeki ürünler sizi bekliyor", "Sepetinizdekilerde ", what, o)
		m.EmailBody = "Sayın {{first_name}},\n\nSepetinize eklediğiniz ürünler hâlâ sizi bekliyor. " + offerSentence(o, full, code, false, minP, expiry) +
			"\n\nAlışverişinizi tamamlamak için sepetinize göz atabilirsiniz.\n\nSaygılarımızla"
	case informal:
		m.SMSBody = "{{first_name}}, sana özel " + specialLine(o, what, "seni bekliyor!") + codeSuffix(o, code)
		m.EmailSubject = subjectFor("{{first_name}}, sana özel bir fırsat var", "{{first_name}}, sana özel ", what, o)
		m.EmailBody = "Merhaba {{first_name}},\n\nSana özel bir kampanyamız var. " + offerSentence(o, full, code, true, minP, expiry) + "\n\nGörüşmek üzere!"
	default:
		m.SMSBody = "Sayın {{first_name}}, size özel " + specialLine(o, what, "sizi bekliyor.") + codeSuffix(o, code)
		m.EmailSubject = subjectFor("Size özel bir fırsat var", "Size özel ", what, o)
		m.EmailBody = "Sayın {{first_name}},\n\nSize özel bir kampanyamız var. " + offerSentence(o, full, code, false, minP, expiry) + "\n\nSaygılarımızla"
	}
	if (minP != "" || expiry != "") && !o.IsNone() {
		m.SMSBody = termsSMS(informal, cart, o, full, code, expiry)
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
func offerSentence(o offer.Offer, what, code string, informal bool, minP, expiry string) string {
	s := offerSentenceCore(o, what, code, informal, minP)
	if expiry != "" {
		s += " " + expiry + "."
	}
	return s
}

func offerSentenceCore(o offer.Offer, what, code string, informal bool, minP string) string {
	switch {
	case what != "" && informal:
		return capitalizeFirst(what) + " fırsatını kaçırma." + codeSuffix(o, code)
	case what != "":
		return capitalizeFirst(what) + " fırsatından yararlanın." + codeSuffix(o, code)
	case o.Kind == offer.KindCouponCode && informal:
		return withMin(minP, code+" koduyla fırsatı yakala.")
	case o.Kind == offer.KindCouponCode:
		return withMin(minP, code+" koduyla fırsatı yakalayın.")
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

// withMin puts the minimum basket in front of a sentence ("1000 TL ve üzeri
// alışverişte YAZ25 koduyla fırsatı yakala.").
func withMin(minP, sentence string) string {
	if minP == "" {
		return sentence
	}
	return minP + " " + sentence
}

// termsSMS is the SMS of an offer that has terms. The SMS has the least room,
// so it is one compact line: the offer with its minimum, the code, the last day.
func termsSMS(informal, cart bool, o offer.Offer, full, code, expiry string) string {
	var s string
	switch {
	case informal && cart:
		s = "{{first_name}}, sepetindeki ürünler için " + full + " seni bekliyor!"
	case informal:
		s = "{{first_name}}, " + full + " seni bekliyor!"
	case cart:
		s = "Sayın {{first_name}}, sepetinizdeki ürünler için " + full + " sizi bekliyor."
	default:
		s = "Sayın {{first_name}}, " + full + " sizi bekliyor."
	}
	s += codeSuffix(o, code)
	if expiry != "" {
		s += " " + expiry + "."
	}
	return s
}
