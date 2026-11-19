package offer

import (
	"strings"
	"testing"
)

func TestScopeValidation(t *testing.T) {
	ok := []Offer{
		{Kind: KindAmount, Value: 200, Scope: "kış ürünleri"},
		{Kind: KindPercent, Value: 20, Scope: ""},
		{Kind: KindCouponCode, Code: "KIS200", Scope: "seçili ürünler"},
		{Kind: KindFreeShipping, Scope: strings.Repeat("a", MaxScopeRunes)},
	}
	for _, o := range ok {
		if err := o.Validate(); err != nil {
			t.Errorf("%+v: %v", o, err)
		}
	}
	bad := map[string]Offer{
		"too long":          {Kind: KindAmount, Value: 200, Scope: strings.Repeat("ş", MaxScopeRunes+1)},
		"a placeholder":     {Kind: KindAmount, Value: 200, Scope: "{{first_name}} ürünleri"},
		"a line break":      {Kind: KindAmount, Value: 200, Scope: "kış\nürünleri"},
		"html":              {Kind: KindAmount, Value: 200, Scope: "<b>kış</b>"},
		"a scope, no offer": {Kind: KindNone, Scope: "kış ürünleri"},
	}
	for name, o := range bad {
		if err := o.Validate(); err == nil || !strings.Contains(err.Error(), "scope") {
			t.Errorf("%s: must be rejected naming scope, got %v", name, err)
		}
	}
}

func TestScopeIsPartOfTheOffer(t *testing.T) {
	a := Offer{Kind: KindAmount, Value: 200, Scope: "  Kış   ürünleri "}
	if n := a.Normalize(); n.Scope != "Kış ürünleri" {
		t.Fatalf("normalized scope %q", n.Scope)
	}
	if !a.Equal(Offer{Kind: KindAmount, Value: 200, Scope: "kış ürünleri"}) {
		t.Error("the same scope in another case is the same offer")
	}
	if a.Equal(Offer{Kind: KindAmount, Value: 200}) || a.Equal(Offer{Kind: KindAmount, Value: 200, Scope: "yaz ürünleri"}) {
		t.Error("another scope is another offer")
	}
	if d := a.Describe(); !strings.Contains(d, "geçerli ürünler: Kış ürünleri") {
		t.Errorf("Describe must state the scope: %q", d)
	}
	if got := (Offer{Kind: KindNone, Scope: "x"}).Normalize(); got.Scope != "" {
		t.Error("no offer, no scope")
	}
}

// A text states the scope with Turkish endings; a text that does not name the
// products does not state the offer.
func TestMentionsScope(t *testing.T) {
	o := Offer{Kind: KindAmount, Value: 200, Scope: "kış ürünleri"}
	for _, text := range []string{"Kış ürünlerinde 200 TL indirim!", "KIŞ ÜRÜNLERİ için 200 TL indirim", "Geçerli ürünler: kış ürünleri. 200 TL indirim"} {
		if !o.Mentions(text) {
			t.Errorf("%q states the offer", text)
		}
	}
	for _, text := range []string{"200 TL indirim!", "Yaz ürünlerinde 200 TL indirim", "Kış ürünlerinde 300 TL indirim"} {
		if o.Mentions(text) {
			t.Errorf("%q does not state the offer", text)
		}
	}
	soft := Offer{Kind: KindPercent, Value: 10, Scope: "kulaklık"}
	if !soft.Mentions("Kulaklığa %10 indirim") {
		t.Error("a final k may soften")
	}
	if !(Offer{Kind: KindPercent, Value: 10}).MentionsScope("anything") {
		t.Error("no scope: always mentioned")
	}
}

func TestScopePhrase(t *testing.T) {
	o := Offer{Kind: KindAmount, Value: 200, Scope: "kış ürünleri"}
	for lang, want := range map[string]string{"tr": "Geçerli ürünler: kış ürünleri", "en": "Applies to: kış ürünleri", "de": "Gilt für: kış ürünleri"} {
		if got := o.ScopePhraseIn(lang); got != want {
			t.Errorf("%s: %q, want %q", lang, got, want)
		}
	}
	if (Offer{Kind: KindAmount, Value: 200}).ScopePhraseIn("tr") != "" {
		t.Error("no scope, no phrase")
	}
}
