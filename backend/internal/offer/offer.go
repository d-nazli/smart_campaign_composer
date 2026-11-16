// Package offer describes the incentive a campaign brief promises (for example
// "%15 discount" or "free shipping") and checks message texts against it.
//
// The offer is extracted at the segment step and travels with the segment rule.
// The message step must state it in both the SMS and the email; the backend
// verifies that, so a model that forgets the discount is caught by code.
package offer

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"smartcampaign/internal/trtext"
)

// Kind is the type of incentive.
type Kind string

const (
	KindNone         Kind = "none"
	KindPercent      Kind = "percent"       // Value is 1..100
	KindAmount       Kind = "amount"        // Value is a TRY amount
	KindFreeShipping Kind = "free_shipping" // no value
	KindCouponCode   Kind = "coupon_code"   // Code is the coupon code
)

// Kinds lists every valid kind (used by the JSON schema and prompts).
var Kinds = []Kind{KindNone, KindPercent, KindAmount, KindFreeShipping, KindCouponCode}

// Offer is the incentive. A percent or amount offer may also carry a Code.
type Offer struct {
	Kind  Kind    `json:"kind"`
	Value float64 `json:"value"`
	Code  string  `json:"code"`
	// MinSpend is the smallest basket (TRY) the offer applies to; 0 means no
	// minimum. It is a TERM OF THE OFFER, never an audience condition.
	MinSpend float64 `json:"min_spend"`
	// ExpiresAt is the last day the offer can be used, YYYY-MM-DD; empty means
	// the brief gives none.
	ExpiresAt string `json:"expires_at"`
}

const (
	MaxPercent = 100
	MaxAmount  = 1_000_000
)

var codeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

// IsNone reports whether the brief promises nothing.
func (o Offer) IsNone() bool { return o.Kind == "" || o.Kind == KindNone }

// Normalize upper-cases the code and clears fields the kind does not use.
func (o Offer) Normalize() Offer {
	if o.IsNone() {
		return Offer{Kind: KindNone}
	}
	o.Code = strings.ToUpper(strings.TrimSpace(o.Code))
	o.ExpiresAt = strings.TrimSpace(o.ExpiresAt)
	switch o.Kind {
	case KindFreeShipping, KindCouponCode:
		o.Value = 0
	}
	return o
}

// Validate checks that the fields fit the kind.
func (o Offer) Validate() error {
	switch o.Kind {
	case "", KindNone:
		if o.Value != 0 || strings.TrimSpace(o.Code) != "" || o.MinSpend != 0 || strings.TrimSpace(o.ExpiresAt) != "" {
			return fmt.Errorf("offer.kind \"none\" iken value 0, code, min_spend ve expires_at boş olmalı")
		}
	case KindPercent:
		if o.Value <= 0 || o.Value > MaxPercent {
			return fmt.Errorf("offer.value yüzde için 0'dan büyük ve en fazla %d olmalı (verilen %s)", MaxPercent, FormatNumber(o.Value))
		}
	case KindAmount:
		if o.Value <= 0 || o.Value > MaxAmount {
			return fmt.Errorf("offer.value TL tutarı için 0'dan büyük ve en fazla %d olmalı (verilen %s)", MaxAmount, FormatNumber(o.Value))
		}
	case KindFreeShipping:
		if o.Value != 0 {
			return fmt.Errorf("offer.kind \"free_shipping\" iken value 0 olmalı")
		}
	case KindCouponCode:
		if o.Value != 0 {
			return fmt.Errorf("offer.kind \"coupon_code\" iken value 0 olmalı")
		}
		if strings.TrimSpace(o.Code) == "" {
			return fmt.Errorf("offer.kind \"coupon_code\" için code dolu olmalı")
		}
	default:
		return fmt.Errorf("offer.kind %q geçersiz; geçerli değerler: none, percent, amount, free_shipping, coupon_code", o.Kind)
	}
	if c := strings.TrimSpace(o.Code); c != "" && !codeRe.MatchString(c) {
		return fmt.Errorf("offer.code %q geçersiz; 3-32 karakter, yalnızca harf, rakam, - ve _", c)
	}
	return termsError(o)
}

// Equal compares two offers after normalization.
func (o Offer) Equal(p Offer) bool {
	a, b := o.Normalize(), p.Normalize()
	return a.Kind == b.Kind && math.Abs(a.Value-b.Value) < 1e-9 && a.Code == b.Code &&
		math.Abs(a.MinSpend-b.MinSpend) < 1e-9 && a.ExpiresAt == b.ExpiresAt
}

// Describe renders the offer for people and for repair feedback, in Turkish.
func (o Offer) Describe() string {
	o = o.Normalize()
	var s string
	switch o.Kind {
	case KindPercent:
		s = "%" + FormatNumber(o.Value) + " indirim"
	case KindAmount:
		s = FormatNumber(o.Value) + " TL indirim"
	case KindFreeShipping:
		s = "ücretsiz kargo"
	case KindCouponCode:
		s = "kupon kodu " + o.Code
	default:
		return "teklif yok"
	}
	var terms []string
	if o.Code != "" && o.Kind != KindCouponCode {
		terms = append(terms, "kupon kodu "+o.Code)
	}
	if p := o.MinSpendPhrase(); p != "" {
		terms = append(terms, p)
	}
	if o.ExpiresAt != "" {
		terms = append(terms, "son kullanım "+FormatDate(o.ExpiresAt))
	}
	if len(terms) > 0 {
		s += " (" + strings.Join(terms, ", ") + ")"
	}
	return s
}

// FormatNumber writes 15 as "15" and 12.5 as "12,5" (Turkish decimal comma).
func FormatNumber(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', -1, 64), ".", ",", 1)
}

// ParseNumber reads "15", "12,5", "12.5" and "1.000" (thousands) into a float.
func ParseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	switch {
	case strings.Contains(s, ".") && strings.Contains(s, ","): // 1.250,50
		s = strings.Replace(strings.ReplaceAll(s, ".", ""), ",", ".", 1)
	case thousandsRe.MatchString(s): // 1.000
		s = strings.ReplaceAll(s, ".", "")
	default:
		s = strings.Replace(s, ",", ".", 1)
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

const numberPattern = `\d+(?:[.,]\d+)*`

var thousandsRe = regexp.MustCompile(`^\d{1,3}(?:\.\d{3})+$`)

var (
	percentRes = []*regexp.Regexp{
		regexp.MustCompile(`%\s*(` + numberPattern + `)`),
		regexp.MustCompile(`(` + numberPattern + `)\s*%`),
		regexp.MustCompile(`\byuzde\s*(` + numberPattern + `)`),
	}
	amountRes = []*regexp.Regexp{
		regexp.MustCompile(`(` + numberPattern + `)\s*(?:tl|try|₺|lira)`),
		regexp.MustCompile(`₺\s*(` + numberPattern + `)`),
	}
	// Free shipping, as it is written in each supported language (folded).
	freeShippingPhrases = []string{
		"ucretsiz kargo", "kargo ucretsiz", "bedava kargo", "kargo bedava", "ucretsiz gonderim", "ucretsiz teslimat", // tr
		"free shipping", "free delivery", "shipping is free", "ships free", // en
		"kostenloser versand", "kostenlosen versand", "kostenlose lieferung", "kostenfreier versand", "versandkostenfrei", "gratis versand", // de
	}
)

func numbers(res []*regexp.Regexp, text string) []float64 {
	var out []float64
	for _, re := range res {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if v, ok := ParseNumber(m[1]); ok {
				out = append(out, v)
			}
		}
	}
	return out
}

// Percents lists every percentage written in text ("%15", "15%", "yüzde 15").
func Percents(text string) []float64 { return numbers(percentRes, trtext.Fold(text)) }

// Amounts lists every TRY amount written in text ("100 TL", "₺100").
func Amounts(text string) []float64 { return numbers(amountRes, trtext.Fold(text)) }

// HasFreeShipping reports whether text promises free shipping.
func HasFreeShipping(text string) bool {
	f := trtext.Fold(text)
	for _, p := range freeShippingPhrases {
		if strings.Contains(f, p) {
			return true
		}
	}
	return false
}

func hasValue(list []float64, v float64) bool {
	for _, x := range list {
		if math.Abs(x-v) < 1e-9 {
			return true
		}
	}
	return false
}

// Mentions reports whether text states the offer. A "none" offer is always
// satisfied. When the offer has a Code, the code must be present as well.
func (o Offer) Mentions(text string) bool {
	o = o.Normalize()
	ok := true
	switch o.Kind {
	case KindPercent:
		ok = hasValue(Percents(text), o.Value)
	case KindAmount:
		ok = hasValue(Amounts(text), o.Value)
	case KindFreeShipping:
		ok = HasFreeShipping(text)
	}
	if ok && o.Code != "" {
		ok = strings.Contains(strings.ToUpper(text), o.Code)
	}
	if ok && o.MinSpend > 0 {
		ok = hasValue(Amounts(text), o.MinSpend)
	}
	if ok && o.ExpiresAt != "" {
		ok = containsString(Dates(text), o.ExpiresAt)
	}
	return ok
}

// Conflicts lists claims in text that contradict the offer: a percentage other
// than the promised one (or any percentage when no percentage was promised).
func (o Offer) Conflicts(text string) []string {
	o = o.Normalize()
	var out []string
	seen := map[float64]bool{}
	for _, p := range Percents(text) {
		if seen[p] || (o.Kind == KindPercent && math.Abs(p-o.Value) < 1e-9) {
			continue
		}
		seen[p] = true
		out = append(out, "%"+FormatNumber(p))
	}
	return out
}
