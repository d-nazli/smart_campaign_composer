// Package trtext holds the one Turkish-aware text normalization used by the
// segment, brief and content packages.
package trtext

import "strings"

var folder = strings.NewReplacer(
	"İ", "i", "I", "i", "ı", "i", "Ç", "c", "ç", "c", "Ğ", "g", "ğ", "g",
	"Ö", "o", "ö", "o", "Ş", "s", "ş", "s", "Ü", "u", "ü", "u",
)

// Fold lower-cases and strips Turkish diacritics so that "Istanbul",
// "istanbul" and "İSTANBUL" compare equal and "sipariş" becomes "siparis".
func Fold(s string) string {
	return strings.ToLower(strings.TrimSpace(folder.Replace(s)))
}
