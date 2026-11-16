package content

import (
	"fmt"
	"regexp"

	"smartcampaign/internal/offer"
	"smartcampaign/internal/trtext"
)

// Quality returns the soft problems of a message: the promised offer is
// missing or contradicted, or a word is repeated. They are not hard limits
// (a person may still choose to send edited text), so Validate does not
// include them; they are fed back to the model as repair feedback and shown to
// the person as warnings.
//
// The offer rules:
//   - the SMS must state it, and the email (subject or body) must state it;
//   - a percentage other than the promised one is a contradiction, and when the
//     brief promises no percentage at all, any percentage is an invention.
func (p Policy) Quality(c Content, o offer.Offer) []Issue {
	o = o.Normalize()
	out := []Issue{} // never nil: the API sends [] rather than null
	add := func(field, code, msg string) {
		out = append(out, Issue{Field: field, Code: code, Message: msg})
	}

	if !o.IsNone() {
		if !o.Mentions(c.SMSBody) {
			add(FieldSMSBody, CodeOfferMissing, fmt.Sprintf("SMS'te brief'teki teklif geçmiyor: %q aynen yazılmalı.", o.Describe()))
		}
		for _, bad := range o.Conflicts(c.SMSBody) {
			add(FieldSMSBody, CodeOfferConflict, conflictMessage("SMS", bad, o))
		}
		if !o.Mentions(c.EmailSubject + "\n" + c.EmailBody) {
			add(FieldEmailBody, CodeOfferMissing, fmt.Sprintf("E-postada (konu veya metin) brief'teki teklif geçmiyor: %q aynen yazılmalı.", o.Describe()))
		}
	} else {
		for _, bad := range o.Conflicts(c.SMSBody) {
			add(FieldSMSBody, CodeOfferConflict, conflictMessage("SMS", bad, o))
		}
	}
	for _, bad := range o.Conflicts(c.EmailSubject) {
		add(FieldEmailSubject, CodeOfferConflict, conflictMessage("E-posta konusu", bad, o))
	}
	for _, bad := range o.Conflicts(c.EmailBody) {
		add(FieldEmailBody, CodeOfferConflict, conflictMessage("E-posta metni", bad, o))
	}
	if !o.IsNone() {
		dateConflicts(o, "SMS", FieldSMSBody, c.SMSBody, add)
		dateConflicts(o, "E-posta konusu", FieldEmailSubject, c.EmailSubject, add)
		dateConflicts(o, "E-posta metni", FieldEmailBody, c.EmailBody, add)
	}

	for _, f := range []struct{ field, label, text string }{
		{FieldSMSBody, "SMS", c.SMSBody},
		{FieldEmailSubject, "E-posta konusu", c.EmailSubject},
		{FieldEmailBody, "E-posta metni", c.EmailBody},
	} {
		if a, b, ok := repeatedWords(f.text); ok {
			add(f.field, CodeRepeatedWord, fmt.Sprintf(
				"%s: %q ve %q birbirine çok yakın tekrar ediyor. Aynı kelimeyi (veya kökünü) arka arkaya kullanma; cümleyi yeniden kur.", f.label, a, b))
		}
	}
	return out
}

func conflictMessage(label, claim string, o offer.Offer) string {
	if o.IsNone() {
		return fmt.Sprintf("%s: brief'te hiçbir indirim oranı yok ama metinde %s yazıyor. Brief'te olmayan teklif uydurma.", label, claim)
	}
	if o.Kind == offer.KindPercent {
		return fmt.Sprintf("%s: metinde %s yazıyor ama brief'teki teklif %q. Başka bir indirim oranı yazma.", label, claim, o.Describe())
	}
	return fmt.Sprintf("%s: metinde %s yazıyor ama brief'teki teklif %q. Brief'te olmayan indirim oranı yazma.", label, claim, o.Describe())
}

// ---- repeated words --------------------------------------------------------

var (
	wordRe      = regexp.MustCompile(`\p{L}+`)
	sentenceSep = regexp.MustCompile(`[.!?…:;\n]+`)
)

const (
	minWordRunes    = 4 // identical words shorter than this are not flagged ("bu bu")
	minStemRunes    = 5 // shared stem length needed to call two words the same word
	repeatWindow    = 2 // a word is compared with the next two words
	stemSlackRunes  = 2 // the shared stem may be shorter than the shorter word by this much
	maxCheckedWords = 600
)

// repeatedWords finds two nearly adjacent words that are the same word or the
// same stem with different endings, inside one sentence ("özel fırsat
// fırsatından", "indirim ... indirimi"). Placeholders are ignored.
func repeatedWords(text string) (string, string, bool) {
	text = placeholderRe.ReplaceAllString(text, " ")
	checked := 0
	for _, sentence := range sentenceSep.Split(text, -1) {
		words := wordRe.FindAllString(sentence, -1)
		folded := make([]string, len(words))
		for i, w := range words {
			folded[i] = trtext.Fold(w)
		}
		for i := range words {
			if checked++; checked > maxCheckedWords {
				return "", "", false
			}
			for j := i + 1; j <= i+repeatWindow && j < len(words); j++ {
				if sameWord(folded[i], folded[j]) {
					return words[i], words[j], true
				}
			}
		}
	}
	return "", "", false
}

func sameWord(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	if len(ra) > len(rb) {
		ra, rb = rb, ra
	}
	short, long := len(ra), len(rb)
	if short == long {
		return string(ra) == string(rb) && short >= minWordRunes
	}
	if short < minStemRunes {
		return false
	}
	cp := 0
	for cp < short && ra[cp] == rb[cp] {
		cp++
	}
	return cp >= minStemRunes && cp >= short-stemSlackRunes
}

// dateConflicts flags every date in a text that is not the offer's last day.
// The brief's last day is the only date a message may state: a different one
// (or any date when the brief gives none) would promise something nobody agreed.
func dateConflicts(o offer.Offer, label, field string, text string, add func(field, code, msg string)) {
	for _, d := range o.DateConflicts(text) {
		if o.ExpiresAt == "" {
			add(field, CodeOfferConflict, fmt.Sprintf("%s: metinde %s tarihi yazıyor ama brief'te son kullanma tarihi yok. Tarih uydurma.", label, d))
		} else {
			add(field, CodeOfferConflict, fmt.Sprintf("%s: metinde %s tarihi yazıyor ama brief'teki son kullanma tarihi %s. Başka bir tarih yazma.", label, d, offer.FormatDate(o.ExpiresAt)))
		}
	}
}
