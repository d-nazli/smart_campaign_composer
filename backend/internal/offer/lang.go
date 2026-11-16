package offer

// The offer's wording in each supported language. The offer itself (kind,
// value, minimum, last day) is language independent; only these phrases change.
// The numbers, the amounts in TL and the dd.mm.yyyy date are written the same
// way in every language on purpose: that is what lets code check that all
// languages promise the same thing.

// FormatNumberIn writes 12.5 as "12,5" in Turkish and German and "12.5" in
// English.
func FormatNumberIn(v float64, lang string) string {
	s := FormatNumber(v)
	if lang == "en" {
		return replaceComma(s)
	}
	return s
}

func replaceComma(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c == ',' {
			b[i] = '.'
		}
	}
	return string(b)
}

// WhatIn is how the offer reads inside a sentence, without terms: "%15 indirim",
// "15% off", "15 % Rabatt", "free shipping". Empty for a "none" offer and for a
// coupon-only offer (the code is stated separately).
func (o Offer) WhatIn(lang string) string {
	o = o.Normalize()
	switch o.Kind {
	case KindPercent:
		switch lang {
		case "en":
			return FormatNumberIn(o.Value, lang) + "% off"
		case "de":
			return FormatNumberIn(o.Value, lang) + " % Rabatt"
		}
		return "%" + FormatNumber(o.Value) + " indirim"
	case KindAmount:
		switch lang {
		case "en":
			return FormatNumberIn(o.Value, lang) + " TL off"
		case "de":
			return FormatNumberIn(o.Value, lang) + " TL Rabatt"
		}
		return FormatNumber(o.Value) + " TL indirim"
	case KindFreeShipping:
		switch lang {
		case "en":
			return "free shipping"
		case "de":
			return "kostenloser Versand"
		}
		return "ücretsiz kargo"
	}
	return ""
}

// MinSpendPhraseIn is how the minimum basket reads: "1000 TL ve üzeri
// alışverişte", "on orders of 1000 TL and over", "ab einem Einkaufswert von
// 1000 TL". Empty when the offer has no minimum.
func (o Offer) MinSpendPhraseIn(lang string) string {
	if o.MinSpend <= 0 {
		return ""
	}
	n := FormatNumberIn(o.MinSpend, lang)
	switch lang {
	case "en":
		return "on orders of " + n + " TL and over"
	case "de":
		return "ab einem Einkaufswert von " + n + " TL"
	}
	return n + " TL ve üzeri alışverişte"
}

// ExpiryPhraseIn is "Son kullanım: 31.12.2026", "Valid until: 31.12.2026",
// "Gültig bis: 31.12.2026". Empty when there is no last day.
func (o Offer) ExpiryPhraseIn(lang string) string {
	if o.ExpiresAt == "" {
		return ""
	}
	d := FormatDate(o.ExpiresAt)
	switch lang {
	case "en":
		return "Valid until: " + d
	case "de":
		return "Gültig bis: " + d
	}
	return "Son kullanım: " + d
}

// CouponLabelIn is the words in front of a coupon code.
func CouponLabelIn(lang string) string {
	switch lang {
	case "en":
		return "Coupon code"
	case "de":
		return "Gutscheincode"
	}
	return "Kupon kodu"
}
