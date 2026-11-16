package offer

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"smartcampaign/internal/trtext"
)

// Dates, minimum spend and expiry: the TERMS of an offer. They belong to the
// offer, never to the audience: "1000 TL üzeri alışverişe 100 TL indirim" says
// who may use the discount, not who receives the message.

// DateMatch is a date written in a text.
type DateMatch struct {
	ISO        string // "2026-12-31"; empty when Valid is false
	Valid      bool   // false for impossible dates such as 31.02.2026
	Start, End int    // byte offsets in the folded text
}

var monthNumbers = map[string]int{
	"ocak": 1, "subat": 2, "mart": 3, "nisan": 4, "mayis": 5, "haziran": 6,
	"temmuz": 7, "agustos": 8, "eylul": 9, "ekim": 10, "kasim": 11, "aralik": 12,
}

const monthPattern = `(?:ocak|subat|mart|nisan|mayis|haziran|temmuz|agustos|eylul|ekim|kasim|aralik)`

var (
	dmyRe       = regexp.MustCompile(`\b(\d{1,2})[./-](\d{1,2})[./-](\d{4})\b`)
	isoRe       = regexp.MustCompile(`\b(\d{4})-(\d{1,2})-(\d{1,2})\b`)
	textDateRe  = regexp.MustCompile(`\b(\d{1,2})\s+(` + monthPattern + `)\w*\s+(\d{4})\b`)
	dayMonthRe  = regexp.MustCompile(`\b(\d{1,2})\s+` + monthPattern + `\w*`)
	yearAfterRe = regexp.MustCompile(`^\s+\d{4}\b`)
)

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// buildDate checks that the day exists and returns its ISO form.
func buildDate(y, m, d int) (string, bool) {
	if y < 2000 || y > 2100 {
		return "", false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != m || t.Day() != d {
		return "", false
	}
	return t.Format("2006-01-02"), true
}

// FindDates lists every full date (with a year) written in text, in order:
// 31.12.2026, 31/12/2026, 31-12-2026, 2026-12-31, "31 Aralık 2026". Impossible
// dates (31.02.2026) are reported with Valid false so callers can say so
// instead of silently ignoring them. Offsets refer to the folded text.
func FindDates(text string) []DateMatch {
	f := trtext.Fold(text)
	var out []DateMatch
	add := func(loc []int, y, m, d int) {
		iso, ok := buildDate(y, m, d)
		out = append(out, DateMatch{ISO: iso, Valid: ok, Start: loc[0], End: loc[1]})
	}
	for _, m := range dmyRe.FindAllStringSubmatchIndex(f, -1) {
		add(m[0:2], atoi(f[m[6]:m[7]]), atoi(f[m[4]:m[5]]), atoi(f[m[2]:m[3]]))
	}
	for _, m := range isoRe.FindAllStringSubmatchIndex(f, -1) {
		add(m[0:2], atoi(f[m[2]:m[3]]), atoi(f[m[4]:m[5]]), atoi(f[m[6]:m[7]]))
	}
	for _, m := range textDateRe.FindAllStringSubmatchIndex(f, -1) {
		word := f[m[4]:m[5]]
		add(m[0:2], atoi(f[m[6]:m[7]]), monthNumbers[word], atoi(f[m[2]:m[3]]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// Dates lists the distinct valid dates in text as ISO strings, in order.
func Dates(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range FindDates(text) {
		if m.Valid && !seen[m.ISO] {
			seen[m.ISO] = true
			out = append(out, m.ISO)
		}
	}
	return out
}

// FindDayMonths finds "31 Aralık" written without a year. Such a date is
// ambiguous (which year?), so the brief reader asks instead of guessing. Spans
// are byte offsets in the folded text.
func FindDayMonths(text string) [][2]int {
	f := trtext.Fold(text)
	var out [][2]int
	for _, m := range dayMonthRe.FindAllStringIndex(f, -1) {
		if yearAfterRe.MatchString(f[m[1]:]) {
			continue // a full date, found by FindDates
		}
		out = append(out, [2]int{m[0], m[1]})
	}
	return out
}

// FormatDate writes 2026-12-31 as 31.12.2026 (the way Turkish messages read).
func FormatDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return ""
	}
	return t.Format("02.01.2006")
}

func validISODate(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}

// MinSpendPhrase is how the minimum reads in a sentence: "1000 TL ve üzeri
// alışverişte". Empty when the offer has no minimum.
func (o Offer) MinSpendPhrase() string { return o.MinSpendPhraseIn("tr") }

// ExpiryPhrase is "Son kullanım: 31.12.2026". Empty when there is no expiry.
func (o Offer) ExpiryPhrase() string { return o.ExpiryPhraseIn("tr") }

// DateConflicts lists the dates (dd.mm.yyyy) in text that are not the offer's
// expiry. When the brief gives no expiry, any date is an invention.
func (o Offer) DateConflicts(text string) []string {
	o = o.Normalize()
	var out []string
	for _, d := range Dates(text) {
		if d != o.ExpiresAt {
			out = append(out, FormatDate(d))
		}
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func termsError(o Offer) error {
	if o.MinSpend < 0 || o.MinSpend > MaxAmount {
		return fmt.Errorf("offer.min_spend 0 ile %d TL arasında olmalı (verilen %s)", MaxAmount, FormatNumber(o.MinSpend))
	}
	if e := strings.TrimSpace(o.ExpiresAt); e != "" && !validISODate(e) {
		return fmt.Errorf("offer.expires_at %q geçersiz; YYYY-MM-DD biçiminde gerçek bir tarih olmalı (örneğin 2026-12-31) ya da boş bırakılmalı", e)
	}
	return nil
}
