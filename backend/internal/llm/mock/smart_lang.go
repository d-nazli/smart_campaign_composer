package mock

import (
	"encoding/json"
	"strings"

	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// langCopy is the wording of one non-default language of the mock "model". The
// mock only has to be believable and obey the same rules as the real model:
// state the offer, keep the placeholders and numbers identical to the Turkish
// text, stay inside the limits and never repeat a word.
type langCopy struct {
	lang                      string
	hello                     string // "Hi {{first_name}},"
	smsCart, smsPlain         string // opening line of the SMS
	enjoy                     string // "enjoy %s!" (after the opening)
	subjectCart, subjectPlain string // no-offer subjects
	subjectOffer              string // "Enjoy %s"
	bodyCart, bodyPlain       string // first sentence of the email
	enjoyBody                 string // "Enjoy %s."
	codeUse                   string // "Use code %s."
	noOfferCTA                string
	bye                       string
}

var englishCopy = langCopy{
	lang: "en", hello: "Hi {{first_name}},",
	smsCart: "your cart is waiting for you!", smsPlain: "something special is waiting for you!",
	enjoy:       "Enjoy %s!",
	subjectCart: "Your cart is waiting for you", subjectPlain: "Something special for you", subjectOffer: "enjoy %s",
	bodyCart: "The items you added to your cart are still waiting for you.", bodyPlain: "We have a special campaign for you.",
	enjoyBody: "Enjoy %s.", codeUse: "Use code %s.", noOfferCTA: "Don't miss out.", bye: "Best regards",
}

var germanCopy = langCopy{
	lang: "de", hello: "Hallo {{first_name}},",
	smsCart: "Ihr Warenkorb wartet auf Sie!", smsPlain: "etwas Besonderes wartet auf Sie!",
	enjoy:       "Profitieren Sie von %s!",
	subjectCart: "Ihr Warenkorb wartet auf Sie", subjectPlain: "Etwas Besonderes für Sie", subjectOffer: "Ihr Vorteil: %s",
	bodyCart: "Die Artikel in Ihrem Warenkorb warten noch auf Sie.", bodyPlain: "Wir haben eine besondere Aktion für Sie.",
	enjoyBody: "Profitieren Sie von %s.", codeUse: "Nutzen Sie den Code %s.", noOfferCTA: "Verpassen Sie sie nicht.", bye: "Viele Grüße",
}

// smartMessagesIn answers like a working model would in English or German.
// (Turkish has its own, richer copy in smart.go.)
func smartMessagesIn(lang, text string, o offer.Offer) string {
	cp := englishCopy
	if lang == "de" {
		cp = germanCopy
	}
	cart := strings.Contains(segment.Fold(text), "sepet")
	o = o.Normalize()

	what := o.WhatIn(cp.lang)
	minP, expiry := o.MinSpendPhraseIn(cp.lang), o.ExpiryPhraseIn(cp.lang)
	full := what
	if what != "" && minP != "" {
		full = what + " " + minP
	}
	label := offer.CouponLabelIn(cp.lang)

	var sms, subject, body string
	// The SMS: greeting, the offer with its terms, the code, the last day.
	opening := cp.smsPlain
	if cart {
		opening = cp.smsCart
	}
	switch {
	case what != "":
		sms = cp.hello + " " + strings.Replace(cp.enjoy, "%s", full, 1)
		subject = "{{first_name}}, " + strings.Replace(cp.subjectOffer, "%s", what, 1)
	case o.Kind == offer.KindCouponCode:
		codeLine := label + ": " + o.Code
		if minP != "" {
			codeLine += " " + minP
		}
		sms = cp.hello + " " + opening + " " + codeLine + "."
		subject = "{{first_name}}, " + strings.Replace(cp.subjectOffer, "%s", label+" "+o.Code, 1)
	default:
		sms = cp.hello + " " + opening
		subject = cp.subjectPlain
		if cart {
			subject = cp.subjectCart
		}
	}
	if o.Code != "" && o.Kind != offer.KindCouponCode {
		sms += " " + label + ": " + o.Code + "."
	}
	if expiry != "" {
		sms += " " + expiry + "."
	}

	first := cp.bodyPlain
	if cart {
		first = cp.bodyCart
	}
	body = cp.hello + "\n\n" + first
	switch {
	case what != "":
		body += " " + strings.Replace(cp.enjoyBody, "%s", full, 1)
		if o.Code != "" && o.Kind != offer.KindCouponCode {
			body += " " + label + ": " + o.Code + "."
		}
	case o.Kind == offer.KindCouponCode:
		body += " " + strings.Replace(cp.codeUse, "%s", o.Code, 1)
		if minP != "" {
			body += " " + minP + "."
		}
	default:
		body += " " + cp.noOfferCTA
	}
	if expiry != "" {
		body += " " + expiry + "."
	}
	body += "\n\n" + cp.bye

	raw, _ := json.Marshal(messagesJSON{SMSBody: sms, EmailSubject: subject, EmailBody: body})
	return string(raw)
}
