package brief

import "testing"

func TestHasDeadline(t *testing.T) {
	for text, want := range map[string]bool{
		"Son 30 günde sepete ürün ekleyip almayanlara %30 indirim": false, // an audience window
		"Tüm müşterilere %10 indirim":                              false,
		"48 saat boyunca geçerli %20 indirim, tüm müşterilere":     true,
		"Bugüne özel %15 indirim gönder, herkese":                  true,
		"Sadece bu hafta sonu ücretsiz kargo, tüm müşterilere":     true,
		"Hafta sonuna kadar %10 indirim":                           true,
		"Kampanya 3 gün geçerli, %25 indirim":                      true,
	} {
		if got := HasDeadline(text, ""); got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
	if !HasDeadline("Tüm müşterilere %10 indirim", "2026-12-31") {
		t.Error("an expiry date is a deadline")
	}
}

// The mock's choice: special day → duygusal, deadline → aciliyet, cart →
// merak, otherwise fayda; B takes another one, and urgency only with a deadline.
func TestSuggestApproaches(t *testing.T) {
	for _, c := range []struct{ text, expires, a, b string }{
		{"Anneler Günü'ne özel %20 indirim, tüm müşterilere", "", "duygusal", "fayda"},
		{"Tüm müşterilere %10 indirim, son kullanım 31.12.2026", "2026-12-31", "aciliyet", "fayda"},
		{"Son 30 günde sepete ürün ekleyip almayanlara %30 indirim", "", "merak", "fayda"},
		{"Tüm müşterilere %10 indirim", "", "fayda", "merak"},
	} {
		a, b := SuggestApproaches(c.text, c.expires, "")
		if a != c.a || b != c.b {
			t.Errorf("%q: %s/%s, want %s/%s", c.text, a, b, c.a, c.b)
		}
	}
	if a, _ := SuggestApproaches("Son 30 günde sepete ürün ekleyip almayanlara %30 indirim", "", "merak"); a != "fayda" {
		t.Errorf("avoiding merak: %s", a)
	}
	for _, avoid := range []string{"fayda", "merak", "kisisel", "duygusal"} {
		if a, _ := SuggestApproaches("Tüm müşterilere %10 indirim", "", avoid); a == "aciliyet" || a == avoid {
			t.Errorf("avoiding %s without a deadline: %s", avoid, a)
		}
	}
}

func TestMentionsStock(t *testing.T) {
	if MentionsStock("Tüm müşterilere %10 indirim") || !MentionsStock("Stoklarla sınırlı %10 indirim") {
		t.Fatal("stock")
	}
}
