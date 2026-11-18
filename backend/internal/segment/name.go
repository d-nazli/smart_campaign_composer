package segment

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxNameRunes is the longest campaign name the model may give.
const MaxNameRunes = 40

// normalizeName trims a name and folds its inner whitespace to single spaces.
func normalizeName(s string) string { return strings.Join(strings.Fields(s), " ") }

// NameProblems checks a campaign name as the model wrote it: present, one line,
// at most MaxNameRunes characters, no {{placeholders}}. Every problem is a
// sentence fed back to the model.
func NameProblems(name string) []string {
	var out []string
	if strings.ContainsAny(name, "\r\n") {
		out = append(out, "name tek satır olmalı")
	}
	n := normalizeName(name)
	switch {
	case n == "":
		out = append(out, fmt.Sprintf("name boş olamaz: kampanyaya en fazla %d karakterlik kısa bir ad ver", MaxNameRunes))
	case utf8.RuneCountInString(n) > MaxNameRunes:
		out = append(out, fmt.Sprintf("name en fazla %d karakter olmalı, %d karakter verildi", MaxNameRunes, utf8.RuneCountInString(n)))
	}
	if strings.ContainsAny(n, "{}") {
		out = append(out, "name süslü parantez veya {{değişken}} içeremez")
	}
	return out
}
