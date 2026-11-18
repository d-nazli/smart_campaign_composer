package brief

import (
	"regexp"

	"smartcampaign/internal/trtext"
)

// A deadline is what makes time pressure honest: a last day (the offer's
// expires_at, read from the brief) or a duration the brief itself gives ("48
// saat boyunca", "bugüne özel"). An audience window ("son 30 günde sepete
// ekleyen") is not one: it says who gets the message, not until when the offer
// holds. Without a deadline no text may hurry the reader (D106).
var deadlineRe = regexp.MustCompile(
	`\b\d+\s*(?:saat|gun|hafta|ay)\s*(?:boyunca|gecerli|sureyle|suresince)` +
		`|\b(?:bugune|bu\s+aksama|bu\s+haftaya|hafta\s+sonuna)\s+ozel` +
		`|\b(?:sadece|yalnizca|yalniz)\s+(?:bugun|bu\s+aksam|bu\s+hafta(?:\s+sonu)?|yarin)` +
		`|\b(?:gece\s+yarisina|hafta\s+sonuna|ay\s+sonuna|pazar\s+gunune)\s+kadar` +
		`|\bson\s+gun\b|\bsinirli\s+sure`)

var stockRe = regexp.MustCompile(`\bstok`)

// HasDeadline reports whether the brief gives a last day (expiresAt, the
// offer's YYYY-MM-DD; "" when none) or a duration.
func HasDeadline(text, expiresAt string) bool {
	return expiresAt != "" || deadlineRe.MatchString(trtext.Fold(text))
}

// MentionsStock reports whether the brief talks about stock at all; only then
// may a text say that stock is running out.
func MentionsStock(text string) bool { return stockRe.MatchString(trtext.Fold(text)) }

// SuggestApproaches is how the mock (and a reader of the rule) picks the
// approaches of Variant A and B from the brief: a special day is emotional, a
// deadline urgent, a cart curious, anything else a plain benefit. B takes the
// first other approach that fits. avoid is an approach that must not be used
// for A (the other variant's, when it already exists).
func SuggestApproaches(text, expiresAt, avoid string) (a, b string) {
	th := ThemeOf(text, "tr")
	deadline := HasDeadline(text, expiresAt)
	pick := func(skip ...string) string {
		order := []string{}
		switch {
		case th.Occasion != "":
			order = append(order, "duygusal")
		case deadline:
			order = append(order, "aciliyet")
		case th.Topic == "cart":
			order = append(order, "merak")
		}
		order = append(order, "fayda", "merak", "kisisel", "duygusal")
		if deadline {
			order = append(order, "aciliyet")
		}
	next:
		for _, o := range order {
			for _, s := range skip {
				if o == s {
					continue next
				}
			}
			return o
		}
		return "fayda"
	}
	a = pick(avoid)
	b = pick(a)
	return a, b
}
