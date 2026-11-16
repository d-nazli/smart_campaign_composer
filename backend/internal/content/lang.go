package content

import "strings"

// Language is one language a campaign is written in.
type Language struct {
	Code    string // stored in users.language and campaign_contents.language
	Name    string // English name, used in the LLM prompt
	Display string // the language's own name, shown in the UI
}

// Languages is the ONE place the supported languages are defined, in display
// order. The first is the default and the reference: the other languages must
// say the same thing as it does (see Consistency). To add a language, add it
// here, give the variables a fallback in it (variables.go), add its fallback
// wording (fallback.go) and its mock copy, and widen the CHECK constraint on
// users.language (a migration).
var Languages = []Language{
	{Code: "tr", Name: "Turkish", Display: "Türkçe"},
	{Code: "en", Name: "English", Display: "English"},
	{Code: "de", Name: "German", Display: "Deutsch"},
}

// DefaultLanguage is the reference language, and the one a person without a
// (known) language receives.
const DefaultLanguage = "tr"

// LanguageCodes lists the supported codes in order.
func LanguageCodes() []string {
	out := make([]string, 0, len(Languages))
	for _, l := range Languages {
		out = append(out, l.Code)
	}
	return out
}

// IsSupported reports whether code is a supported language.
func IsSupported(code string) bool {
	_, ok := LanguageByCode(code)
	return ok
}

// LanguageByCode finds a language by its code.
func LanguageByCode(code string) (Language, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, l := range Languages {
		if l.Code == code {
			return l, true
		}
	}
	return Language{}, false
}

// NormalizeLanguage returns the code of a supported language, or the default
// for an empty or unknown one.
func NormalizeLanguage(code string) string {
	if l, ok := LanguageByCode(code); ok {
		return l.Code
	}
	return DefaultLanguage
}
