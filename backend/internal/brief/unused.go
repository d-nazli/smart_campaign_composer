package brief

import (
	"fmt"
	"strings"

	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
	"smartcampaign/internal/trtext"
)

// Unused is the other direction of Ground (D89): every TL amount and every
// percentage the brief writes must end up somewhere in the rule or the offer.
// A model that reads "1000 TL'ye varan siparişte 300 TL indirim" as a plain
// 300 TL discount has dropped a term the customer would be held to; nothing in
// its rule is invented, so Ground alone cannot see it. This check does not say
// WHAT the amount is (minimum basket, cap, tier, spending audience): it only
// says it was not used, and the model decides.
//
// Dates ("31.12.2026", "31 Aralık") and special days ("23 Nisan", "8 Mart") are
// taken out first: they are not amounts. Day counts are not checked here (they
// carry no TL or % sign).

// UnusedValue is one amount or percentage of the brief that the rule leaves out.
type UnusedValue struct {
	Value   float64
	Percent bool
}

func (u UnusedValue) String() string {
	if u.Percent {
		return "%" + offer.FormatNumber(u.Value)
	}
	return offer.FormatNumber(u.Value) + " TL"
}

// UnusedValues lists the brief's TL amounts and percentages that appear
// nowhere in an "ok" rule.
func UnusedValues(text string, r segment.Rule) []UnusedValue {
	if r.Status != segment.StatusOK {
		return nil
	}
	f := maskedForAmounts(text)

	usedAmounts := map[float64]bool{}
	usedPercents := map[float64]bool{}
	o := r.Offer.Normalize()
	for _, p := range o.Parts() {
		switch p.Kind {
		case offer.KindPercent:
			usedPercents[p.Value] = true
		case offer.KindAmount:
			usedAmounts[p.Value] = true
		}
	}
	for _, v := range []float64{o.MinSpend, o.StepSpend, o.MaxDiscount} {
		if v > 0 {
			usedAmounts[v] = true
		}
	}
	for _, c := range r.Conditions {
		if c.Field == "total_spent" {
			if v, ok := number(c.Value); ok {
				usedAmounts[v] = true
			}
		}
	}

	var out []UnusedValue
	seen := map[UnusedValue]bool{}
	add := func(u UnusedValue) {
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	for _, p := range offer.Percents(f) {
		if !usedPercents[p] {
			add(UnusedValue{Value: p, Percent: true})
		}
	}
	for _, m := range wordPctRe.FindAllStringSubmatch(f, -1) {
		if n, ok := parseCount(m[1]); ok && !usedPercents[float64(n)] {
			add(UnusedValue{Value: float64(n), Percent: true})
		}
	}
	for _, a := range offer.Amounts(f) {
		if !usedAmounts[a] {
			add(UnusedValue{Value: a})
		}
	}
	return out
}

// maskedForAmounts is the folded brief without its dates and special days.
func maskedForAmounts(text string) string {
	f := trtext.Fold(text)
	b := []byte(f)
	blankSpan := func(a, z int) {
		for i := a; i < z && i < len(b); i++ {
			b[i] = ' '
		}
	}
	for _, m := range offer.FindDates(f) {
		blankSpan(m.Start, m.End)
	}
	for _, sp := range offer.FindDayMonths(f) {
		blankSpan(sp[0], sp[1])
	}
	_, spans := findOccasions(f)
	for _, sp := range spans {
		blankSpan(sp[0], sp[1])
	}
	return string(b)
}

// unusedFeedback is the repair feedback: what was left out, and where it can go.
func unusedFeedback(us []UnusedValue) []string {
	var out []string
	for _, u := range us {
		if u.Percent {
			out = append(out, fmt.Sprintf("Brief'te %s geçiyor ama ne kuralda ne teklifte kullanılmış. Brief'teki her yüzde bir yere oturmalı: indirim oranıysa offer (kind percent ya da extras). Ne için olduğu belli değilse needs_clarification ile sor.", u))
			continue
		}
		out = append(out, fmt.Sprintf("Brief'te %s geçiyor ama ne kuralda ne teklifte kullanılmış. Brief'teki her TL tutarı bir yere oturmalı: minimum sepet tutarıysa offer.min_spend (\"%s'ye varan / ve üzeri alışverişte\"), indirim tavanıysa offer.max_discount, kademe ise offer.step_spend, indirimin kendisiyse offer.value, müşterinin toplam harcamasıysa total_spent koşulu. Ne için olduğu belli değilse needs_clarification ile sor.", u, u))
	}
	return out
}

// unusedQuestion is what the person is asked when the model never used them:
// the choices, in the brief's own numbers.
func unusedQuestion(us []UnusedValue) string {
	u := us[0]
	var q string
	if u.Percent {
		q = fmt.Sprintf("Brief'teki %s ne için? İndirim oranıysa \"%s indirim\" diye yazın.", u, u)
	} else {
		n := offer.FormatNumber(u.Value)
		q = fmt.Sprintf("Brief'teki %s TL ne için: minimum sepet tutarı mı (\"%s TL ve üzeri alışverişte\"), indirimin üst sınırı mı (\"en fazla %s TL indirim\"), yoksa müşterinin toplam harcaması mı (\"toplam harcaması %s TL'den fazla olanlar\")? Brief'i buna göre yazıp tekrar deneyin.", n, n, n, n)
	}
	if len(us) > 1 {
		var rest []string
		for _, x := range us[1:] {
			rest = append(rest, x.String())
		}
		q += " Kullanılmayan diğer değerler: " + strings.Join(rest, ", ") + "."
	}
	return clip(q, segment.MaxQuestion)
}
