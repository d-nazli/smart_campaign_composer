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
	// StepSpend makes an amount discount tiered: Value TL for every StepSpend TL
	// of the basket. 0 = not tiered. See parts.go.
	StepSpend float64 `json:"step_spend"`
	// MaxDiscount caps the discount in TL: the total of a tiered discount, or the
	// most a percentage can take off. 0 = no cap.
	MaxDiscount float64 `json:"max_discount"`
	// Extras are the other incentives promised together with the main one
	// ("%20 indirim + ücretsiz kargo"); each of a different kind.
	Extras []Part `json:"extras"`
}

const (
	MaxPercent = 100
	MaxAmount  = 1_000_000
)

var codeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

// IsNone reports whether the brief promises nothing.
func (o Offer) IsNone() bool { return o.Kind == "" || o.Kind == KindNone }

// Normalize upper-cases the code, clears fields the kind does not use and puts
// the parts of a combined offer in canonical order (the first one becomes the
// main part), so two offers that promise the same compare Equal.
func (o Offer) Normalize() Offer {
	if o.IsNone() {
		return Offer{Kind: KindNone}
	}
	o.Code = strings.ToUpper(strings.TrimSpace(o.Code))
	o.ExpiresAt = strings.TrimSpace(o.ExpiresAt)
	if o.Kind == KindCouponCode {
		o.Value, o.Extras = 0, nil
		return o
	}
	main, extras, ok := normalizeParts(o)
	if !ok {
		return Offer{Kind: KindNone}
	}
	o.Kind, o.Value, o.Extras = main.Kind, main.Value, extras
	return o
}

// Validate checks that the fields fit the kind.
func (o Offer) Validate() error {
	switch o.Kind {
	case "", KindNone:
		if o.Value != 0 || strings.TrimSpace(o.Code) != "" || o.MinSpend != 0 || strings.TrimSpace(o.ExpiresAt) != "" ||
			o.StepSpend != 0 || o.MaxDiscount != 0 || len(o.Extras) != 0 {
			return fmt.Errorf("offer.kind \"none\" iken value 0, code, min_spend, expires_at, step_spend, max_discount ve extras boş olmalı")
		}
	case KindPercent, KindAmount:
		if err := valueError("offer", o.Kind, o.Value); err != nil {
			return err
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
	if err := partsError(o); err != nil {
		return err
	}
	return termsError(o)
}

// Equal compares two offers after normalization.
func (o Offer) Equal(p Offer) bool {
	a, b := o.Normalize(), p.Normalize()
	return a.Kind == b.Kind && near(a.Value, b.Value) && a.Code == b.Code &&
		near(a.MinSpend, b.MinSpend) && a.ExpiresAt == b.ExpiresAt &&
		near(a.StepSpend, b.StepSpend) && near(a.MaxDiscount, b.MaxDiscount) && partsEqual(a.Extras, b.Extras)
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Describe renders the offer for people and for repair feedback, in Turkish.
func (o Offer) Describe() string {
	o = o.Normalize()
	var s string
	switch o.Kind {
	case KindPercent, KindAmount, KindFreeShipping:
		s = o.whatIn("tr")
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
// satisfied. Every part of a combined offer must be present, and so must every
// number of its terms: the code, the minimum basket, the tier step and cap,
// the last day.
func (o Offer) Mentions(text string) bool {
	o = o.Normalize()
	ok := true
	for _, p := range o.Parts() {
		switch p.Kind {
		case KindPercent:
			ok = ok && hasValue(Percents(text), p.Value)
		case KindAmount:
			ok = ok && hasValue(Amounts(text), p.Value)
		case KindFreeShipping:
			ok = ok && HasFreeShipping(text)
		}
	}
	if ok && o.StepSpend > 0 {
		ok = hasValue(Amounts(text), o.StepSpend)
	}
	if ok && o.MaxDiscount > 0 {
		ok = hasValue(Amounts(text), o.MaxDiscount)
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
// than the promised one (or any percentage when no percentage was promised),
// and a TL amount that is none of the offer's own numbers (its discount, its
// minimum basket, its tier step or its cap). A message that adds "for example
// 3000 TL" or "worth 500 TL" states something nobody agreed.
func (o Offer) Conflicts(text string) []string {
	o = o.Normalize()
	var out []string
	seen := map[float64]bool{}
	promised, hasPercent := o.part(KindPercent)
	for _, p := range Percents(text) {
		if seen[p] || (hasPercent && near(p, promised.Value)) {
			continue
		}
		seen[p] = true
		out = append(out, "%"+FormatNumber(p))
	}
	allowed := []float64{o.MinSpend, o.StepSpend, o.MaxDiscount}
	if a, ok := o.part(KindAmount); ok {
		allowed = append(allowed, a.Value)
	}
	seenAmount := map[float64]bool{}
	for _, a := range Amounts(text) {
		if seenAmount[a] || (a > 0 && hasValue(allowed, a)) {
			continue
		}
		seenAmount[a] = true
		out = append(out, FormatNumber(a)+" TL")
	}
	return out
}
