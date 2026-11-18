package content

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Locale is one row of the locales table: a language for one market (BCP 47,
// "en-GB"). The list is data, never code: adding a locale is adding a row.
type Locale struct {
	Code       string `json:"code"`
	Name       string `json:"name"`        // English name, given to the model
	NativeName string `json:"native_name"` // the locale's own name, shown in the UI
	Direction  string `json:"direction"`   // "ltr" or "rtl"
	IsSource   bool   `json:"is_source"`
	// Fallbacks is the word each variable becomes when the person's value is
	// missing or too long, in this locale ({"first_name": "valued customer"}).
	Fallbacks map[string]string `json:"fallbacks"`
	// FreeShippingTerms is how this locale writes "free shipping". Empty means
	// the free-shipping promise of a translation cannot be checked.
	FreeShippingTerms []string `json:"-"`
}

// Lang is the language part of the code ("en" for "en-GB").
func (l Locale) Lang() string {
	lang, _, _ := strings.Cut(l.Code, "-")
	return lang
}

var localeCodeRe = regexp.MustCompile(`^[a-z]{2,3}-[A-Z]{2}$`)

// Locales is the set of locales, the source first and the others by code.
type Locales struct {
	list []Locale
}

// NewLocales checks a set read from the database: valid unique codes, exactly
// one source, a direction, and a fallback word for every variable that fits
// the variable's budget (so a rendered message keeps its proven length).
func NewLocales(list []Locale, p Policy) (Locales, error) {
	seen := map[string]bool{}
	var out []Locale
	sources := 0
	for _, l := range list {
		if !localeCodeRe.MatchString(l.Code) {
			return Locales{}, fmt.Errorf("locale %q: code must look like en-GB", l.Code)
		}
		if seen[l.Code] {
			return Locales{}, fmt.Errorf("locale %q appears twice", l.Code)
		}
		seen[l.Code] = true
		if l.Direction != "ltr" && l.Direction != "rtl" {
			return Locales{}, fmt.Errorf("locale %s: direction must be ltr or rtl", l.Code)
		}
		if l.IsSource {
			sources++
		}
		for _, v := range Variables {
			w := strings.TrimSpace(l.Fallbacks[v.Name])
			if w == "" {
				return Locales{}, fmt.Errorf("locale %s: no fallback word for {{%s}}", l.Code, v.Name)
			}
			if n, b := utf8.RuneCountInString(w), p.Budget(v.Name); n > b {
				return Locales{}, fmt.Errorf("locale %s: fallback %q for {{%s}} is %d characters, the budget is %d", l.Code, w, v.Name, n, b)
			}
		}
		out = append(out, l)
	}
	if sources != 1 {
		return Locales{}, fmt.Errorf("exactly one source locale is required, found %d", sources)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsSource != out[j].IsSource {
			return out[i].IsSource
		}
		return out[i].Code < out[j].Code
	})
	return Locales{list: out}, nil
}

// All lists every locale, the source first.
func (s Locales) All() []Locale { return append([]Locale(nil), s.list...) }

// Source is the locale every campaign is written in first.
func (s Locales) Source() Locale {
	if len(s.list) == 0 {
		return Locale{}
	}
	return s.list[0]
}

// Get finds a locale by its exact code.
func (s Locales) Get(code string) (Locale, bool) {
	for _, l := range s.list {
		if l.Code == code {
			return l, true
		}
	}
	return Locale{}, false
}

// Codes lists the codes, the source first.
func (s Locales) Codes() []string {
	out := make([]string, 0, len(s.list))
	for _, l := range s.list {
		out = append(out, l.Code)
	}
	return out
}

// Ordered returns the given codes in the set's order (source first, then by
// code), dropping codes that are not locales.
func (s Locales) Ordered(codes map[string]bool) []string {
	var out []string
	for _, l := range s.list {
		if codes[l.Code] {
			out = append(out, l.Code)
		}
	}
	return out
}
