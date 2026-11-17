package brief

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
	"smartcampaign/internal/trtext"
)

// Ground is the objective check of a rule the REAL model wrote (D87): every
// city, number, amount, code and date in it must be written in the brief.
// It does not judge the interpretation (whether "did not buy" was meant, which
// of two readings is right): that is the model's call. It only catches what
// the model could not have read in the brief, because it is not there.
//
// Day counts may be converted from the brief's own unit ("2 hafta" = 14, "bir
// ay" = 30, "48 saat" = 2), and 0 and 1 are always allowed (they carry no
// fact: "has ever added to the cart" is gte 0, "has ordered" is gt 0).
func Ground(text string, r segment.Rule) []string {
	if r.Status != segment.StatusOK {
		return nil
	}
	g := newGrounds(text)
	var problems []string
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	for _, c := range r.Conditions {
		switch c.Field {
		case "city":
			var cities []string
			switch v := c.Value.(type) {
			case string:
				cities = []string{v}
			case []string:
				cities = v
			}
			for _, city := range cities {
				if !g.cities[city] {
					bad("Kuralda %q şehri var ama brief'te bu şehir geçmiyor. Brief'te yazmayan şehir ekleme.", city)
				}
			}
		case "days_since_last_cart_add", "days_since_last_order", "days_since_last_order_or_never":
			if n, ok := c.Value.(int64); ok && n > 1 && !g.days[float64(n)] && !g.numbers[float64(n)] {
				bad("Kuralda %s %s %d var ama brief'te %d gün ya da ona denk bir süre (hafta, ay, yıl) geçmiyor. Süreyi brief'teki sayıdan çıkar.", c.Field, c.Op, n, n)
			}
		case "order_count":
			if n, ok := c.Value.(int64); ok && n > 1 && !g.numbers[float64(n)] {
				bad("Kuralda order_count %s %d var ama brief'te %d sayısı geçmiyor.", c.Op, n, n)
			}
		case "total_spent":
			if v, ok := number(c.Value); ok && !g.numbers[v] {
				bad("Kuralda total_spent %s %s var ama brief'te bu tutar geçmiyor.", c.Op, offer.FormatNumber(v))
			}
		case "gender":
			if !g.gender {
				bad("Kuralda gender koşulu var ama brief'te cinsiyetle ilgili bir kelime yok (bir özel günün adı, ör. \"Kadınlar Günü\", sayılmaz).")
			}
		}
	}

	o := r.Offer.Normalize()
	for _, p := range o.Parts() {
		switch p.Kind {
		case offer.KindPercent, offer.KindAmount:
			if !g.numbers[p.Value] {
				bad("Teklifte %s var ama brief'te %s sayısı geçmiyor. Teklifi brief'te yazan sayıyla kur.", describePart(p), offer.FormatNumber(p.Value))
			}
		case offer.KindFreeShipping:
			if !g.shipping {
				bad("Teklifte ücretsiz kargo var ama brief'te kargodan söz edilmiyor.")
			}
		}
	}
	for _, t := range []struct {
		name  string
		value float64
	}{{"min_spend", o.MinSpend}, {"step_spend", o.StepSpend}, {"max_discount", o.MaxDiscount}} {
		if t.value > 0 && !g.numbers[t.value] {
			bad("Teklifte %s %s var ama brief'te bu tutar geçmiyor.", t.name, offer.FormatNumber(t.value))
		}
	}
	if o.Code != "" && !strings.Contains(strings.ToUpper(text), o.Code) {
		bad("Teklifte %q kodu var ama brief'te bu kod geçmiyor. Kod uydurma; brief'te kod yoksa code boş olmalı.", o.Code)
	}
	if o.ExpiresAt != "" && !g.dates[o.ExpiresAt] {
		bad("Teklifte son kullanma tarihi %s var ama brief'te bu tarih (yılıyla) geçmiyor. Tarih uydurma; yıl yazılmamışsa needs_clarification ile sor.", offer.FormatDate(o.ExpiresAt))
	}
	return problems
}

func describePart(p offer.Part) string {
	if p.Kind == offer.KindPercent {
		return "%" + offer.FormatNumber(p.Value) + " indirim"
	}
	return offer.FormatNumber(p.Value) + " TL indirim"
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// grounds is what the brief itself states.
type grounds struct {
	numbers, days    map[float64]bool
	cities           map[string]bool
	dates            map[string]bool
	gender, shipping bool
}

var (
	digitNumberRe = regexp.MustCompile(`\d+(?:[.,]\d+)*`)
	wordNumberRe  = regexp.MustCompile(`\b` + numWords + `\b`)
	numberUnitRe  = regexp.MustCompile(`\b(` + num + `)\s*(gun|hafta|ay|yil|saat)\w*`)
	// "son hafta", "bu ay", "geçen hafta": one unit, read by the model as that many days.
	bareUnitRe = regexp.MustCompile(`\b(?:son|bu|gecen|onceki|gelecek)\s+(gun|hafta|ay|yil)\w*`)
	shippingRe = regexp.MustCompile(`\b(?:kargo\w*|gonderim\w*|teslimat\w*|shipping)`)
)

func newGrounds(text string) grounds {
	f := trtext.Fold(text)
	g := grounds{numbers: map[float64]bool{}, days: map[float64]bool{}, cities: map[string]bool{}, dates: map[string]bool{}}
	for _, m := range digitNumberRe.FindAllString(f, -1) {
		if v, ok := offer.ParseNumber(m); ok {
			g.numbers[v] = true
		}
		// "1.5" may also be read as two numbers by a careless writer; both count.
		for _, part := range strings.FieldsFunc(m, func(r rune) bool { return r == '.' || r == ',' }) {
			if v, ok := offer.ParseNumber(part); ok {
				g.numbers[v] = true
			}
		}
	}
	for _, m := range wordNumberRe.FindAllString(f, -1) {
		if n, ok := parseCount(m); ok {
			g.numbers[float64(n)] = true
		}
	}
	for _, m := range numberUnitRe.FindAllStringSubmatch(f, -1) {
		n, ok := parseCount(m[1])
		if !ok {
			continue
		}
		if m[2] == "saat" {
			g.days[math.Ceil(float64(n)/24)] = true
			continue
		}
		g.days[float64(n*unitDays(m[2]))] = true
	}
	for _, m := range bareUnitRe.FindAllStringSubmatch(f, -1) {
		g.days[float64(unitDays(m[1]))] = true
	}
	for _, d := range offer.Dates(f) {
		g.dates[d] = true
	}
	var tmp Intent
	tmp.analyzeCities(f)
	for _, c := range append(tmp.Included, tmp.Excluded...) {
		g.cities[c] = true
	}
	// A gender word that is only part of a special day's name does not count.
	_, spans := findOccasions(f)
	masked := []byte(f)
	for _, sp := range spans {
		for i := sp[0]; i < sp[1]; i++ {
			masked[i] = ' '
		}
	}
	g.gender = genderWordRe.Match(masked)
	g.shipping = shippingRe.MatchString(f) || offer.HasFreeShipping(f)
	return g
}

// SaysEveryone reports whether the brief itself addresses everyone ("herkese",
// "tüm müşterilere", "kadın ve erkek müşterilere"). When a model chooses
// all_customers without it, the audience step says the audience was not given.
func SaysEveryone(text string) bool {
	f := trtext.Fold(text)
	return everyoneRe.MatchString(f) || bothGendersRe.MatchString(f)
}
