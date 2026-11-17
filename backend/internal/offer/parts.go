package offer

import (
	"fmt"
	"sort"
	"strings"
)

// Combined and tiered offers.
//
// A brief may promise several incentives that go TOGETHER ("%20 indirim +
// ücretsiz kargo"): they are one offer with up to MaxParts parts, each of a
// different kind. The main part is Kind/Value, the others are Extras. After
// Normalize the parts are in a fixed order (percent, amount, free shipping),
// so "ücretsiz kargo ve %20" and "%20 ve ücretsiz kargo" are the same offer.
//
// A tiered discount ("her 1000 TL'lik alışverişe 100 TL, toplamda en fazla
// 300 TL") is an amount part with StepSpend (the discount is given for every
// StepSpend TL of the basket) and MaxDiscount (the total never exceeds it).
// MaxDiscount on a percent part is the largest TL amount the percentage can
// reach ("%20 indirim, en fazla 300 TL").

// MaxParts is how many incentives one offer may combine.
const MaxParts = 3

// Part is one incentive of a combined offer.
type Part struct {
	Kind  Kind    `json:"kind"`
	Value float64 `json:"value"`
}

// kindOrder is the canonical order of the parts.
var kindOrder = map[Kind]int{KindPercent: 0, KindAmount: 1, KindFreeShipping: 2}

// PartKinds are the kinds a part may have (a coupon code is not a part: it is
// stated next to the parts, in Code).
var PartKinds = []Kind{KindPercent, KindAmount, KindFreeShipping}

// Parts lists every incentive of the offer, main part first, in canonical
// order. A coupon-only or "none" offer has no parts.
func (o Offer) Parts() []Part {
	o = o.Normalize()
	if o.IsNone() || o.Kind == KindCouponCode {
		return nil
	}
	return append([]Part{{Kind: o.Kind, Value: o.Value}}, o.Extras...)
}

// part returns the part of a kind, if the offer has one.
func (o Offer) part(k Kind) (Part, bool) {
	for _, p := range o.Parts() {
		if p.Kind == k {
			return p, true
		}
	}
	return Part{}, false
}

// IsTiered reports a discount given per StepSpend TL of the basket.
func (o Offer) IsTiered() bool { return o.Normalize().StepSpend > 0 }

// normalizeParts puts the main part and the extras in canonical order. It
// returns the new main part, the extras (nil when there are none) and false
// when no real part is left (a coupon-only or empty offer).
func normalizeParts(o Offer) (Part, []Part, bool) {
	var parts []Part
	seen := map[Kind]bool{}
	add := func(p Part) {
		if _, ok := kindOrder[p.Kind]; !ok || seen[p.Kind] {
			return
		}
		if p.Kind == KindFreeShipping {
			p.Value = 0
		}
		seen[p.Kind] = true
		parts = append(parts, p)
	}
	add(Part{Kind: o.Kind, Value: o.Value})
	for _, e := range o.Extras {
		add(e)
	}
	if len(parts) == 0 {
		return Part{}, nil, false
	}
	sort.SliceStable(parts, func(i, j int) bool { return kindOrder[parts[i].Kind] < kindOrder[parts[j].Kind] })
	if len(parts) == 1 {
		return parts[0], nil, true
	}
	return parts[0], parts[1:], true
}

// partsError checks the extras and the tier terms. It runs on the offer as the
// model wrote it, before Normalize.
func partsError(o Offer) error {
	if len(o.Extras) > 0 && (o.Kind == KindNone || o.Kind == "" || o.Kind == KindCouponCode) {
		return fmt.Errorf("offer.extras yalnızca kind percent, amount veya free_shipping iken kullanılabilir; ana teklifi kind/value alanına yaz, kupon kodunu code alanına")
	}
	if len(o.Extras) > MaxParts-1 {
		return fmt.Errorf("offer en fazla %d parçalı olabilir (kind + en fazla %d extras), %d extras verildi", MaxParts, MaxParts-1, len(o.Extras))
	}
	kinds := map[Kind]bool{o.Kind: true}
	for i, e := range o.Extras {
		if _, ok := kindOrder[e.Kind]; !ok {
			return fmt.Errorf("offer.extras[%d].kind %q geçersiz; percent, amount veya free_shipping olmalı", i, e.Kind)
		}
		if kinds[e.Kind] {
			return fmt.Errorf("offer.extras[%d]: %s türünden ikinci bir teklif olamaz; aynı türde iki farklı teklif çelişkidir", i, e.Kind)
		}
		kinds[e.Kind] = true
		if err := valueError(fmt.Sprintf("offer.extras[%d]", i), e.Kind, e.Value); err != nil {
			return err
		}
	}
	if o.StepSpend < 0 || o.StepSpend > MaxAmount {
		return fmt.Errorf("offer.step_spend 0 ile %d TL arasında olmalı (verilen %s)", MaxAmount, FormatNumber(o.StepSpend))
	}
	if o.MaxDiscount < 0 || o.MaxDiscount > MaxAmount {
		return fmt.Errorf("offer.max_discount 0 ile %d TL arasında olmalı (verilen %s)", MaxAmount, FormatNumber(o.MaxDiscount))
	}
	hasPercent, hasAmount := kinds[KindPercent], kinds[KindAmount]
	amount := o.Value
	for _, e := range o.Extras {
		if e.Kind == KindAmount {
			amount = e.Value
		}
	}
	if o.StepSpend > 0 {
		switch {
		case !hasAmount:
			return fmt.Errorf("offer.step_spend (her %s TL'de) yalnızca TL tutarında indirimle kullanılabilir (\"her 1000 TL'ye 100 TL\"); kademe yoksa 0 yaz", FormatNumber(o.StepSpend))
		case hasPercent:
			return fmt.Errorf("offer.step_spend yüzde indirimle birlikte kullanılamaz; kademeli TL indirimi tek başına (gerekirse ücretsiz kargoyla) verilmeli")
		case amount >= o.StepSpend:
			return fmt.Errorf("offer: her %s TL'ye %s TL indirim olamaz; indirim (value) kademe tutarından (step_spend) küçük olmalı", FormatNumber(o.StepSpend), FormatNumber(amount))
		}
	}
	if o.MaxDiscount > 0 {
		switch {
		case hasPercent && hasAmount:
			return fmt.Errorf("offer.max_discount hem yüzde hem TL indirimi varken hangisine ait olduğu belli değil")
		case !hasPercent && !hasAmount:
			return fmt.Errorf("offer.max_discount (en fazla %s TL) yalnızca yüzde ya da kademeli TL indirimiyle kullanılabilir", FormatNumber(o.MaxDiscount))
		case hasAmount && o.StepSpend == 0:
			return fmt.Errorf("offer.max_discount sabit bir TL indiriminde anlamsız; yalnızca kademeli indirimde (step_spend) toplam üst sınırdır")
		case hasAmount && o.MaxDiscount < amount:
			return fmt.Errorf("offer.max_discount (%s TL) tek kademenin indiriminden (%s TL) küçük olamaz", FormatNumber(o.MaxDiscount), FormatNumber(amount))
		}
	}
	return nil
}

func valueError(path string, k Kind, v float64) error {
	switch k {
	case KindPercent:
		if v <= 0 || v > MaxPercent {
			return fmt.Errorf("%s.value yüzde için 0'dan büyük ve en fazla %d olmalı (verilen %s)", path, MaxPercent, FormatNumber(v))
		}
	case KindAmount:
		if v <= 0 || v > MaxAmount {
			return fmt.Errorf("%s.value TL tutarı için 0'dan büyük ve en fazla %d olmalı (verilen %s)", path, MaxAmount, FormatNumber(v))
		}
	case KindFreeShipping:
		if v != 0 {
			return fmt.Errorf("%s: free_shipping iken value 0 olmalı", path)
		}
	}
	return nil
}

func partsEqual(a, b []Part) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || !near(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

// partPhraseIn is one part as it reads in a sentence. extra marks a TL amount
// that comes on top of a percentage ("ekstra 50 TL indirim").
func (o Offer) partPhraseIn(p Part, lang string, extra bool) string {
	switch p.Kind {
	case KindPercent:
		switch lang {
		case "en":
			return FormatNumberIn(p.Value, lang) + "% off"
		case "de":
			return FormatNumberIn(p.Value, lang) + " % Rabatt"
		}
		return "%" + FormatNumber(p.Value) + " indirim"
	case KindAmount:
		v := FormatNumberIn(p.Value, lang)
		if o.StepSpend > 0 {
			step := FormatNumberIn(o.StepSpend, lang)
			switch lang {
			case "en":
				return v + " TL off every " + step + " TL"
			case "de":
				return v + " TL Rabatt je " + step + " TL Einkaufswert"
			}
			return "her " + step + " TL alışverişe " + v + " TL indirim"
		}
		switch lang {
		case "en":
			if extra {
				return "an extra " + v + " TL off"
			}
			return v + " TL off"
		case "de":
			if extra {
				return "zusätzlich " + v + " TL Rabatt"
			}
			return v + " TL Rabatt"
		}
		if extra {
			return "ekstra " + v + " TL indirim"
		}
		return v + " TL indirim"
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

// CapPhraseIn is the upper limit of the discount: "toplamda en fazla 300 TL"
// (tiered), "en fazla 300 TL" (percent). Empty when there is none.
func (o Offer) CapPhraseIn(lang string) string {
	o = o.Normalize()
	if o.MaxDiscount <= 0 {
		return ""
	}
	n := FormatNumberIn(o.MaxDiscount, lang)
	tiered := o.StepSpend > 0
	switch lang {
	case "en":
		if tiered {
			return "up to " + n + " TL in total"
		}
		return "up to " + n + " TL"
	case "de":
		if tiered {
			return "insgesamt höchstens " + n + " TL"
		}
		return "höchstens " + n + " TL"
	}
	if tiered {
		return "toplamda en fazla " + n + " TL"
	}
	return "en fazla " + n + " TL"
}

// HeadlineIn is the main part alone, without its cap and without the other
// parts: what fits a short subject line ("%20 indirim", "her 1000 TL
// alışverişe 100 TL indirim"). Empty for a coupon-only or "none" offer.
func (o Offer) HeadlineIn(lang string) string {
	o = o.Normalize()
	parts := o.Parts()
	if len(parts) == 0 {
		return ""
	}
	return o.partPhraseIn(parts[0], lang, false)
}

// whatIn joins the parts: "%20 indirim (en fazla 300 TL) + ücretsiz kargo".
func (o Offer) whatIn(lang string) string {
	o = o.Normalize()
	parts := o.Parts()
	if len(parts) == 0 {
		return ""
	}
	_, hasPercent := o.part(KindPercent)
	phrases := make([]string, 0, len(parts))
	for i, p := range parts {
		s := o.partPhraseIn(p, lang, p.Kind == KindAmount && hasPercent)
		if i == 0 {
			if c := o.CapPhraseIn(lang); c != "" {
				s += " (" + c + ")"
			}
		}
		phrases = append(phrases, s)
	}
	return strings.Join(phrases, " + ")
}
