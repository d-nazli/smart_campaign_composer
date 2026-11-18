package mock

import (
	"encoding/json"
	"hash/fnv"
	"strings"
	"unicode"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/offer"
)

// The mock's copywriter. It writes like a working model would: in the
// requested language, about the brief's special day or topic, in its tone,
// stating every part of the offer with its terms, and never over 160 / 80.
//
// How it stays varied AND inside the rules: it assembles candidate texts from
// phrase banks (greeting × hook × call to action for the SMS; subject ×
// opening × offer sentence × sign-off for the email), starts at a position
// picked from a hash of the brief, and takes the first candidate that passes
// the same checks the backend applies to model output (Validate + Quality).
// The banks are tried most specific first (the special day, then the topic,
// then general phrases), and when the offer and its terms leave little room
// the terms are written in their short form ("Kod:", "1500 TL üzeri"), like a
// person would. Different briefs get different texts; the same brief always
// gets the same text, so tests and demos are reproducible.

// pair is one phrase in the informal ("sen") and the formal ("siz") register.
// English and German have one register; their pairs repeat the phrase.
type pair [2]string

func (p pair) in(formal bool) string {
	if formal && p[1] != "" {
		return p[1]
	}
	return p[0]
}

func same(s string) pair { return pair{s, s} }

type kit struct {
	lang      string
	greetSMS  []pair // "{{first_name}}," (always with the placeholder)
	greetBody []pair // "Merhaba {{first_name}},"
	hooks     map[string][]pair
	opens     map[string][]pair
	ctas      map[string][]pair
	// offerSentence states the offer in the email: %s = the offer with its minimum.
	offerSentence []pair
	// subjectOffer is a subject around the main part of the offer (%s).
	subjectOffer []pair
	signoff      []pair
	// join puts the minimum basket next to the offer, in this language's order.
	join func(what, minP string) string
	// The short forms of the terms, for an SMS that is short of room.
	shortCode  string                // "Kod"
	shortMin   func(n string) string // "1500 TL üzeri"
	shortUntil func(d string) string // "Son gün 31.12.2026"
}

// smartMessagesFor writes the source text (the source locale's phrase book).
func smartMessagesFor(req llm.MessageRequest) string {
	k, ok := kitFor(req.Locale.Lang())
	if !ok {
		k = turkishKit
	}
	th := req.Theme
	if th.Topic == "" && th.Occasion == "" && len(th.Tones) == 0 {
		th = brief.ThemeOf(req.Brief, k.lang) // a caller that did not read the theme
	}
	return toJSON(k.write(req.Policy, req.Offer.Normalize(), th, seedOf(req.Brief)))
}

// PseudoPrefix marks a mock translation that is only a copy of the source.
func PseudoPrefix(code string) string { return "[" + code + "] " }

// smartTranslate is the mock translator. For a locale whose language has a
// phrase book (en, de) it writes the campaign with that book and keeps it only
// if it passes the limits and says exactly what the source says (the source
// may have been edited by hand). Otherwise, and for every other locale, it
// returns the source text marked with the locale ("[fr-FR] ..."): numbers,
// dates and {{placeholders}} are then the source's own, and a new locale works
// without code. The mark is dropped from a field it would push over a limit.
func smartTranslate(req llm.TranslateRequest) string {
	if k, ok := kitFor(req.Target.Lang()); ok && req.Target.Lang() != req.SourceLocale.Lang() {
		th := brief.ThemeOf(req.Brief, k.lang)
		c := k.write(req.Policy, req.Offer.Normalize(), th, seedOf(req.Brief))
		if norm, rep := req.Policy.Validate(c); rep.OK && len(content.ConsistentWith(req.SourceLocale, req.Source, req.Target, norm)) == 0 {
			return toJSON(norm)
		}
	}
	mark := PseudoPrefix(req.Target.Code)
	c := req.Source
	if t := mark + c.SMSBody; req.Policy.EffectiveLength(t) <= content.SMSMaxRunes {
		c.SMSBody = t
	}
	if t := mark + c.EmailSubject; req.Policy.EffectiveLength(t) <= content.SubjectMaxRunes {
		c.EmailSubject = t
	}
	c.EmailBody = mark + c.EmailBody
	return toJSON(c)
}

func toJSON(c content.Content) string {
	raw, _ := json.Marshal(messagesJSON{SMSBody: c.SMSBody, EmailSubject: c.EmailSubject, EmailBody: c.EmailBody})
	return string(raw)
}

type messagesJSON struct {
	SMSBody      string `json:"sms_body"`
	EmailSubject string `json:"email_subject"`
	EmailBody    string `json:"email_body"`
}

// kitFor returns the phrase book of a language ("en" for every en-* locale).
func kitFor(lang string) (kit, bool) {
	switch lang {
	case "tr":
		return turkishKit, true
	case "en":
		return englishKit, true
	case "de":
		return germanKit, true
	}
	return kit{}, false
}

func seedOf(s string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(s)))
	return int(h.Sum32() & 0x7fffffff)
}

var giftDays = map[string]bool{
	brief.OccasionMothersDay: true, brief.OccasionFathersDay: true, brief.OccasionValentines: true,
	brief.OccasionTeachersDay: true, brief.OccasionNewYear: true,
}

// offerText is the offer as one language writes it, in a long and a short form.
type offerText struct {
	full, tail           string // "1000 TL ve üzeri alışverişte %20 indirim", " Kupon kodu: X. Son kullanım: 31.12.2026."
	shortFull, shortTail string
	headline             string // the main part, for a subject
	none                 bool
}

func (k kit) offerText(o offer.Offer) offerText {
	what := o.WhatIn(k.lang)
	label := offer.CouponLabelIn(k.lang)
	t := offerText{headline: o.HeadlineIn(k.lang)}
	minP, shortMin := o.MinSpendPhraseIn(k.lang), ""
	if o.MinSpend > 0 {
		shortMin = k.shortMin(offer.FormatNumberIn(o.MinSpend, k.lang))
	}
	switch {
	case what != "":
		t.full, t.shortFull = k.join(what, minP), k.join(what, shortMin)
	case o.Kind == offer.KindCouponCode:
		t.full, t.shortFull = k.join(label+": "+o.Code, minP), k.join(k.shortCode+": "+o.Code, shortMin)
		t.headline = label + " " + o.Code
	default:
		t.none = true
	}
	if o.Code != "" && o.Kind != offer.KindCouponCode {
		t.tail += " " + label + ": " + o.Code + "."
		t.shortTail += " " + k.shortCode + ": " + o.Code + "."
	}
	if o.ExpiresAt != "" {
		t.tail += " " + o.ExpiryPhraseIn(k.lang) + "."
		t.shortTail += " " + k.shortUntil(offer.FormatDate(o.ExpiresAt)) + "."
	}
	return t
}

func (k kit) write(p content.Policy, o offer.Offer, th llm.Theme, seed int) content.Content {
	formal := th.Has(llm.ToneFormal) || (k.lang == "tr" && !th.Has(llm.ToneFriendly) && !th.Has(llm.ToneExcited))
	short := th.Has(llm.ToneShort)
	ot := k.offerText(o)

	// The hook banks, most specific first. Tone hooks join the first bank.
	var tones []pair
	for _, t := range []string{llm.ToneExcited, llm.ToneElegant} {
		if th.Has(t) {
			tones = append(tones, k.hooks["tone:"+t]...)
		}
	}
	if th.Has(llm.ToneUrgent) && o.ExpiresAt != "" {
		tones = append(tones, k.hooks["tone:"+llm.ToneUrgent]...)
	}
	var banks [][]pair
	var openBanks [][]pair
	for _, key := range []string{th.Occasion, th.Topic, llm.TopicGeneral} {
		if key == "" || (key == llm.TopicGeneral && len(banks) > 0 && th.Topic == llm.TopicGeneral) {
			continue
		}
		if h := k.hooks[key]; len(h) > 0 {
			banks = append(banks, h)
		}
		if op := k.opens[key]; len(op) > 0 {
			openBanks = append(openBanks, op)
		}
	}
	if len(banks) == 0 {
		banks = [][]pair{k.hooks[llm.TopicGeneral]}
	}
	if len(openBanks) == 0 {
		openBanks = [][]pair{k.opens[llm.TopicGeneral]}
	}
	banks[0] = append(append([]pair(nil), tones...), banks[0]...)
	banks = append(banks, k.hooks[llm.TopicGeneral]) // the last resort

	ctaKeys := []string{"general"}
	if th.Topic == llm.TopicCart {
		ctaKeys = append([]string{"cart"}, ctaKeys...)
	}
	if giftDays[th.Occasion] {
		ctaKeys = append([]string{"gift"}, ctaKeys...)
	}
	var ctas []pair
	for _, key := range ctaKeys {
		ctas = append(ctas, k.ctas[key]...)
	}

	sms := k.pickSMS(p, o, ot, banks, ctas, formal, short, seed)
	subject, body := k.pickEmail(p, o, ot, banks, openBanks, ctas, formal, short, seed/7)
	return content.Content{SMSBody: sms, EmailSubject: subject, EmailBody: body}
}

// pickSMS: greeting × hook × call to action, bank by bank, long terms first.
func (k kit) pickSMS(p content.Policy, o offer.Offer, ot offerText, banks [][]pair, ctas []pair, formal, short bool, seed int) string {
	nc := len(ctas) + 1 // the last choice is "no call to action"
	if short {
		nc = 1
	}
	compose := func(greet, hook, full, tail, cta string) string {
		s := greet + " " + hook
		if ot.none {
			s += "!"
		} else {
			s += ": " + full + "!" + tail
		}
		if cta != "" {
			s += " " + cta
		}
		return s
	}
	for _, hooks := range banks {
		for _, form := range [][2]string{{ot.full, ot.tail}, {ot.shortFull, ot.shortTail}} {
			total := len(k.greetSMS) * len(hooks) * nc
			for i := 0; i < total; i++ {
				n := (seed + i) % total
				g, h, c := n%len(k.greetSMS), (n/len(k.greetSMS))%len(hooks), n/(len(k.greetSMS)*len(hooks))
				cta := ""
				if !short && c < len(ctas) {
					cta = ctas[c].in(formal)
				}
				s := compose(k.greetSMS[g].in(formal), hooks[h].in(formal), form[0], form[1], cta)
				if fieldOK(p, o, content.FieldSMSBody, content.Content{SMSBody: s}) {
					return s
				}
			}
		}
	}
	// Nothing fits with a hook: the name and the offer in its short form.
	return strings.TrimSuffix(k.greetSMS[0].in(formal), ",") + ": " + ot.shortFull + "!" + ot.shortTail
}

// pickEmail: subject × opening × offer sentence × sign-off, bank by bank.
func (k kit) pickEmail(p content.Policy, o offer.Offer, ot offerText, banks, openBanks [][]pair, ctas []pair, formal, short bool, seed int) (string, string) {
	for bi, hooks := range banks {
		var subjects []string
		for _, h := range hooks {
			hk := h.in(formal)
			if ot.headline != "" {
				subjects = append(subjects, "{{first_name}}, "+hk+": "+ot.headline, upperFirstIn(k.lang, hk)+": "+ot.headline)
			} else {
				subjects = append(subjects, "{{first_name}}, "+hk, upperFirstIn(k.lang, hk))
			}
		}
		if ot.headline != "" && bi == len(banks)-1 {
			for _, so := range k.subjectOffer {
				subjects = append(subjects, strings.Replace(so.in(formal), "%s", ot.headline, 1))
			}
		}
		opens := openBanks[min(bi, len(openBanks)-1)]
		nOpen := len(opens)
		if short {
			nOpen = 1
		}
		total := len(subjects) * nOpen * len(k.offerSentence) * len(k.signoff) * len(k.greetBody) * len(ctas)
		for i := 0; i < total; i++ {
			n := (seed + i) % total
			pick := func(size int) int { r := n % size; n /= size; return r }
			sb, op, os, so, gb, ct := pick(len(subjects)), pick(nOpen), pick(len(k.offerSentence)), pick(len(k.signoff)), pick(len(k.greetBody)), pick(len(ctas))
			body := k.greetBody[gb].in(formal) + "\n\n"
			if !short {
				body += opens[op].in(formal)
			}
			switch {
			case ot.none:
			case o.Kind == offer.KindCouponCode:
				body += " " + upperFirstIn(k.lang, ot.full) + "." + ot.tail
			default:
				body += " " + upperFirstIn(k.lang, strings.Replace(k.offerSentence[os].in(formal), "%s", ot.full, 1)) + ot.tail
			}
			body = strings.Replace(body, "\n\n ", "\n\n", 1)
			body += "\n\n" + ctas[ct].in(formal) + "\n\n" + k.signoff[so].in(formal)
			if fieldOK(p, o, content.FieldEmailSubject, content.Content{EmailSubject: subjects[sb], EmailBody: body}) {
				return subjects[sb], body
			}
		}
	}
	subject := upperFirstIn(k.lang, ot.headline)
	if subject == "" {
		subject = upperFirstIn(k.lang, banks[0][0].in(formal))
	}
	return subject, k.greetBody[0].in(formal) + "\n\n" + upperFirstIn(k.lang, ot.full) + "." + ot.tail + "\n\n" + k.signoff[0].in(formal)
}

// fieldOK applies the backend's checks to one field. For the email, subject
// and body are checked together, as the backend reads the offer from both.
func fieldOK(p content.Policy, o offer.Offer, field string, c content.Content) bool {
	if c.EmailSubject == "" {
		c.EmailSubject = "x"
	}
	if c.EmailBody == "" {
		c.EmailBody = "x"
	}
	if c.SMSBody == "" {
		c.SMSBody = "x"
	}
	email := field == content.FieldEmailSubject || field == content.FieldEmailBody
	mine := func(f string) bool {
		return f == field || (email && (f == content.FieldEmailSubject || f == content.FieldEmailBody))
	}
	norm, rep := p.Validate(c)
	for _, i := range rep.Issues {
		if mine(i.Field) {
			return false
		}
	}
	for _, i := range p.Quality(norm, o) {
		if mine(i.Field) {
			return false
		}
	}
	return true
}

// upperFirstIn capitalises the first letter the way the language does (in
// Turkish "ilk" becomes "İlk", not "Ilk"). A leading placeholder is kept.
func upperFirstIn(lang, s string) string {
	r := []rune(s)
	if len(r) == 0 || strings.HasPrefix(s, "{{") {
		return s
	}
	switch {
	case lang == "tr" && r[0] == 'i':
		r[0] = 'İ'
	case lang == "tr" && r[0] == 'ı':
		r[0] = 'I'
	default:
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}
