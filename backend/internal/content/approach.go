package content

import (
	"fmt"
	"strings"
)

// An approach is the one deliberate angle a text takes (D105). The model picks
// it for each A/B variant from this fixed list, so the difference between A and
// B is a named choice that can be checked and compared, not an accident of
// wording. The values are stored and sent as they are (ASCII, Turkish words).
const (
	ApproachUrgency   = "aciliyet" // time pressure: only with a last day or a duration in the brief
	ApproachEmotional = "duygusal" // the feeling of the occasion or the relationship
	ApproachBenefit   = "fayda"    // what the reader gains: the offer itself, plainly
	ApproachCuriosity = "merak"    // a question or a hint that makes the reader look
	ApproachPersonal  = "kisisel"  // speaks to the reader's own situation (their cart, their visit)
)

// Approaches is the list in a fixed order.
var Approaches = []string{ApproachUrgency, ApproachEmotional, ApproachBenefit, ApproachCuriosity, ApproachPersonal}

// ApproachMeanings says what each approach means, for the model's prompt.
var ApproachMeanings = map[string]string{
	ApproachUrgency:   "time pressure from the offer's own last day or duration; allowed ONLY when the brief gives one",
	ApproachEmotional: "the feeling of the occasion or of the relationship with the customer",
	ApproachBenefit:   "what the reader gains, the offer stated plainly and early",
	ApproachCuriosity: "a question or a hint that makes the reader want to look",
	ApproachPersonal:  "the reader's own situation (their cart, their last visit), addressed directly",
}

// IsApproach reports whether s is one of the fixed approaches.
func IsApproach(s string) bool {
	for _, a := range Approaches {
		if a == s {
			return true
		}
	}
	return false
}

// ApproachProblem says why an approach cannot be used ("" when it can):
// unknown, the same as the other variant's, or urgency without a deadline.
func ApproachProblem(approach, other string, deadline bool) string {
	switch {
	case !IsApproach(approach):
		return fmt.Sprintf("approach %q listede yok; şunlardan biri olmalı: %s", approach, strings.Join(Approaches, ", "))
	case other != "" && approach == other:
		return fmt.Sprintf("approach %q diğer varyantın yaklaşımıyla aynı; farklı bir yaklaşım seç", approach)
	case approach == ApproachUrgency && !deadline:
		return "approach \"aciliyet\" seçilemez: brief'te son tarih ya da süre yok"
	}
	return ""
}

// Claims a text may make only when the brief backs them. Matched in lower case.
var (
	timeClaims = []string{"son gün", "son fırsat", "son şans", "sınırlı süre", "kısa süreliğine", "süre doluyor", "süre dolmadan",
		"acele et", "acele edin", "bugün bitiyor", "yarın bitiyor", "saatler kaldı", "son saatler", "sadece bugün", "yalnızca bugün",
		"bugüne özel"}
	stockClaims = []string{"stoklar", "stokla", "stokta", "tükeniyor", "tükenmeden", "son ürün"}
)

// CodeUnsupportedClaim marks a sentence that claims something the brief does not say.
const CodeUnsupportedClaim = "unsupported_claim"

// UnsupportedClaims finds time pressure in a text whose brief gives no last day
// or duration, and stock scarcity in a text whose brief never mentions stock:
// the reader would be told something nobody promised.
func UnsupportedClaims(c Content, deadline, stock bool) []Issue {
	var out []Issue
	for _, f := range []struct{ field, label, text string }{
		{FieldSMSBody, "SMS", c.SMSBody}, {FieldEmailSubject, "E-posta konusu", c.EmailSubject}, {FieldEmailBody, "E-posta metni", c.EmailBody},
	} {
		low := strings.ToLower(f.text)
		if !deadline {
			for _, w := range timeClaims {
				if strings.Contains(low, w) {
					out = append(out, Issue{Field: f.field, Code: CodeUnsupportedClaim,
						Message: fmt.Sprintf("%s %q diyor ama brief'te son tarih ya da süre yok: süre baskısı uydurulamaz", f.label, w)})
				}
			}
		}
		if !stock {
			for _, w := range stockClaims {
				if strings.Contains(low, w) {
					out = append(out, Issue{Field: f.field, Code: CodeUnsupportedClaim,
						Message: fmt.Sprintf("%s %q diyor ama brief stoktan söz etmiyor: stok iddiası uydurulamaz", f.label, w)})
				}
			}
		}
	}
	return out
}
