package segment

import "strings"

// KnownCities are the canonical city names in the database. LLM output is
// mapped onto this list, so a typo or invented city is a validation error and
// never a silent "zero results".
var KnownCities = []string{
	"İstanbul", "Ankara", "İzmir", "Bursa", "Antalya", "Adana", "Konya", "Gaziantep", "Kocaeli", "Mersin",
	"Eskişehir", "Trabzon", "Samsun", "Kayseri", "Diyarbakır", "Muğla", "Denizli", "Tekirdağ", "Balıkesir",
}

var cityByFold = func() map[string]string {
	m := make(map[string]string, len(KnownCities))
	for _, c := range KnownCities {
		m[foldTR(c)] = c
	}
	return m
}()

// foldTR lower-cases and strips Turkish diacritics so that "Istanbul",
// "istanbul" and "İSTANBUL" compare equal.
var trFolder = strings.NewReplacer(
	"İ", "i", "I", "i", "ı", "i", "Ç", "c", "ç", "c", "Ğ", "g", "ğ", "g",
	"Ö", "o", "ö", "o", "Ş", "s", "ş", "s", "Ü", "u", "ü", "u",
)

func foldTR(s string) string {
	return strings.ToLower(strings.TrimSpace(trFolder.Replace(s)))
}

// Fold exposes the Turkish-insensitive folding (used by the mock LLM to find
// city names inside a brief).
func Fold(s string) string { return foldTR(s) }

// CanonicalCity maps any spelling of a known city to its canonical name.
func CanonicalCity(s string) (string, bool) {
	c, ok := cityByFold[foldTR(s)]
	return c, ok
}
