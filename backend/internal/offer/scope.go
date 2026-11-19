package offer

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"smartcampaign/internal/trtext"
)

// Scope: the products or the category an offer applies to ("kış ürünleri",
// "seçili ürünler", "ayakkabı kategorisi"). Free text, written as the brief
// writes it. It is a TERM of the offer and goes into the messages; it never
// selects who receives them. The customer database has no product data, so
// "people who bought winter products" is not something a scope can express.

// MaxScopeRunes is the longest scope accepted.
const MaxScopeRunes = 60

// normalizeScope trims the scope and puts it on one line with single spaces.
func normalizeScope(s string) string { return strings.Join(strings.Fields(s), " ") }

// scopeError checks the scope's form; an empty scope is always fine.
func scopeError(o Offer) error {
	s := normalizeScope(o.Scope)
	if s == "" {
		return nil
	}
	if o.IsNone() {
		return fmt.Errorf("offer.kind \"none\" iken scope boş olmalı (kapsam yalnızca bir teklifin kapsamıdır)")
	}
	if n := utf8.RuneCountInString(s); n > MaxScopeRunes {
		return fmt.Errorf("offer.scope en fazla %d karakter olmalı (verilen %d)", MaxScopeRunes, n)
	}
	if strings.ContainsAny(o.Scope, "{}<>\n\r") {
		return fmt.Errorf("offer.scope düz metin olmalı: süslü parantez, < > ve satır sonu içeremez")
	}
	return nil
}

// ScopePhraseIn is the sentence that states the scope: "Geçerli ürünler: kış
// ürünleri", "Applies to: kış ürünleri", "Gilt für: kış ürünleri". The scope
// itself is copied as it is written. Empty when there is no scope.
func (o Offer) ScopePhraseIn(lang string) string {
	s := normalizeScope(o.Scope)
	if s == "" {
		return ""
	}
	switch lang {
	case "en":
		return "Applies to: " + s
	case "de":
		return "Gilt für: " + s
	}
	return "Geçerli ürünler: " + s
}

// MentionsScope reports whether text names the scope. Turkish adds endings
// ("kış ürünleri" → "kış ürünlerinde"), so the folded scope must appear as the
// start of the words; a final p, ç, t or k may have softened ("kulaklık" →
// "kulaklığa") and is not required. An offer without a scope always passes.
func (o Offer) MentionsScope(text string) bool {
	s := trtext.Fold(normalizeScope(o.Scope))
	if s == "" {
		return true
	}
	f := strings.Join(strings.Fields(trtext.Fold(text)), " ")
	if r := []rune(s); len(r) > 3 && strings.ContainsRune("pctk", r[len(r)-1]) {
		s = string(r[:len(r)-1])
	}
	return strings.Contains(f, s)
}

func sameScope(a, b string) bool {
	return trtext.Fold(normalizeScope(a)) == trtext.Fold(normalizeScope(b))
}
