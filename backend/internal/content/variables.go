// Package content validates and renders the SMS/email templates.
//
// The rules here are hard guarantees, not hints to the LLM:
//   - SMS never exceeds SMSMaxRunes, the email subject never exceeds
//     SubjectMaxRunes (and is always a single line), after personalization.
//   - Only the variables in Variables can appear in a template.
//
// Lengths are counted in runes (characters), as CASE.md specifies. Real SMS
// networks count Turkish characters (ş, ğ, ı, İ ...) as UCS-2 and cap a single
// message at 70 characters; CASE.md says 160, so we count characters and note
// the difference in DECISIONS.md.
package content

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	SMSMaxRunes       = 160
	SubjectMaxRunes   = 80
	EmailBodyMaxRunes = 5000
)

// Recipient is the personalization data a template can use.
type Recipient struct {
	FirstName string
	City      string
	// Language is the recipient's language code; empty or unknown means the
	// default language. It only decides which fallback word replaces a missing
	// value (the template itself is already in the recipient's language).
	Language string
}

// Variable describes one {{name}} placeholder.
//
// Budget is a fixed allowance of characters: when a template is validated,
// every occurrence is counted as if it expanded to exactly Budget characters.
// When a message is rendered, a value that is empty or longer than the budget is
// replaced by Fallback (which always fits the budget), so the validated length
// is a true upper bound for every recipient.
type Variable struct {
	Name          string
	DefaultBudget int
	// Fallback is the word used in the default language when the value is
	// missing or too long; FallbackIn holds it for the other languages. Every
	// one of them must fit the budget.
	Fallback   string
	FallbackIn map[string]string
	Value      func(Recipient) string
}

// FallbackFor returns the replacement word in a language (the default
// language's when the language has none).
func (v Variable) FallbackFor(lang string) string {
	if w, ok := v.FallbackIn[lang]; ok && w != "" {
		return w
	}
	return v.Fallback
}

// fallbacks lists every language's replacement word, default language first.
func (v Variable) fallbacks() []string {
	out := []string{v.Fallback}
	for _, l := range Languages {
		if l.Code != DefaultLanguage {
			out = append(out, v.FallbackFor(l.Code))
		}
	}
	return out
}

// Variables is the ONE place personalization variables are defined. To add a
// variable, add an entry here (and, if recipients need it, a field on
// Recipient); the validator, renderer, LLM prompt, UI limits and env override
// (VARIABLE_BUDGET_<NAME>) all derive from this list.
var Variables = []Variable{
	{Name: "first_name", DefaultBudget: 20, Fallback: "değerli müşterimiz",
		FallbackIn: map[string]string{"en": "valued customer", "de": "geschätzter Kunde"},
		Value:      func(r Recipient) string { return r.FirstName }},
	{Name: "city", DefaultBudget: 15, Fallback: "bölgeniz",
		FallbackIn: map[string]string{"en": "your area", "de": "Ihrer Region"},
		Value:      func(r Recipient) string { return r.City }},
}

func lookup(name string) (Variable, bool) {
	for _, v := range Variables {
		if v.Name == name {
			return v, true
		}
	}
	return Variable{}, false
}

// Policy holds the effective per-variable budgets.
type Policy struct {
	budgets map[string]int
}

// NewPolicy applies overrides (variable name -> budget) on top of the defaults.
func NewPolicy(overrides map[string]int) (Policy, error) {
	p := Policy{budgets: map[string]int{}}
	for _, v := range Variables {
		p.budgets[v.Name] = v.DefaultBudget
	}
	for name, b := range overrides {
		v, ok := lookup(name)
		if !ok {
			return Policy{}, fmt.Errorf("unknown variable %q in budget overrides", name)
		}
		p.budgets[name] = b
		if b < 1 || b > 100 {
			return Policy{}, fmt.Errorf("budget for %s must be between 1 and 100, got %d", name, b)
		}
		for _, w := range v.fallbacks() {
			if n := utf8.RuneCountInString(w); b < n {
				return Policy{}, fmt.Errorf("budget for %s (%d) is smaller than its fallback %q (%d characters)", name, b, w, n)
			}
		}
	}
	return p, nil
}

// PolicyFromEnv reads VARIABLE_BUDGET_<NAME> (e.g. VARIABLE_BUDGET_FIRST_NAME).
func PolicyFromEnv(getenv func(string) string) (Policy, error) {
	overrides := map[string]int{}
	for _, v := range Variables {
		key := "VARIABLE_BUDGET_" + strings.ToUpper(v.Name)
		raw := strings.TrimSpace(getenv(key))
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Policy{}, fmt.Errorf("%s must be an integer, got %q", key, raw)
		}
		overrides[v.Name] = n
	}
	return NewPolicy(overrides)
}

// Budget returns the allowance for a variable (0 for unknown names).
func (p Policy) Budget(name string) int { return p.budgets[name] }

// VariableInfo is what the API and the LLM prompt expose about a variable.
type VariableInfo struct {
	Name     string `json:"name"`
	Budget   int    `json:"budget"`
	Fallback string `json:"fallback"`
	// Fallbacks is the replacement word in every language, by code.
	Fallbacks map[string]string `json:"fallbacks"`
}

// Infos lists the variables in definition order.
func (p Policy) Infos() []VariableInfo {
	out := make([]VariableInfo, 0, len(Variables))
	for _, v := range Variables {
		all := map[string]string{}
		for _, l := range Languages {
			all[l.Code] = v.FallbackFor(l.Code)
		}
		out = append(out, VariableInfo{Name: v.Name, Budget: p.budgets[v.Name], Fallback: v.Fallback, Fallbacks: all})
	}
	return out
}
