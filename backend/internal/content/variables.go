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
	// Fallbacks is the recipient's locale's word for each variable whose value
	// is missing or too long (Locale.Fallbacks); a variable without one uses
	// Variable.Fallback.
	Fallbacks map[string]string
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
	// Fallback is the source locale's word used when the value is missing or
	// too long and the recipient's locale has none. Each locale has its own word
	// (Locale.Fallbacks); every one of them must fit the budget (NewLocales).
	Fallback string
	Value    func(Recipient) string
}

// FallbackFor returns the word for a recipient whose value cannot be used.
func (v Variable) FallbackFor(r Recipient) string {
	if w := r.Fallbacks[v.Name]; w != "" {
		return w
	}
	return v.Fallback
}

// Variables is the ONE place personalization variables are defined. To add a
// variable, add an entry here (and, if recipients need it, a field on
// Recipient); the validator, renderer, LLM prompt, UI limits and env override
// (VARIABLE_BUDGET_<NAME>) all derive from this list.
var Variables = []Variable{
	{Name: "first_name", DefaultBudget: 20, Fallback: "değerli müşterimiz",
		Value: func(r Recipient) string { return r.FirstName }},
	{Name: "city", DefaultBudget: 15, Fallback: "bölgeniz",
		Value: func(r Recipient) string { return r.City }},
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
		if n := utf8.RuneCountInString(v.Fallback); b < n {
			return Policy{}, fmt.Errorf("budget for %s (%d) is smaller than its fallback %q (%d characters)", name, b, v.Fallback, n)
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
	// Fallbacks is the replacement word in every locale, by code (empty when
	// the locales are not known to the caller).
	Fallbacks map[string]string `json:"fallbacks,omitempty"`
}

// Infos lists the variables in definition order.
func (p Policy) Infos() []VariableInfo { return p.InfosFor(Locales{}) }

// InfosFor is Infos with every locale's fallback word.
func (p Policy) InfosFor(ls Locales) []VariableInfo {
	out := make([]VariableInfo, 0, len(Variables))
	for _, v := range Variables {
		info := VariableInfo{Name: v.Name, Budget: p.budgets[v.Name], Fallback: v.Fallback}
		for _, l := range ls.All() {
			if info.Fallbacks == nil {
				info.Fallbacks = map[string]string{}
			}
			info.Fallbacks[l.Code] = v.FallbackFor(Recipient{Fallbacks: l.Fallbacks})
		}
		out = append(out, info)
	}
	return out
}
