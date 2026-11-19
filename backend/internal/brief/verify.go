package brief

import (
	"fmt"
	"sort"
	"strings"

	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

// Verify checks a valid "ok" rule against what the brief asks for and returns
// one problem per part that was left out, changed or invented. The messages
// are written for the model: they say what is missing and what to write.
// Rules that are not "ok" are not verified: saying "I cannot do this" is
// always visible to the person.
func Verify(in Intent, r segment.Rule) []string {
	if r.Status != segment.StatusOK {
		return nil
	}
	var problems []string
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	in.verifyCities(r, bad)
	in.verifyPurchase(r, bad)
	in.verifyWindow(r, bad)

	if !r.Offer.Equal(in.Offer) {
		if in.Offer.IsNone() {
			bad("Brief'te bir teklif geçmiyor ama offer alanında %q var; offer.kind \"none\", value 0, code \"\", min_spend 0 ve expires_at \"\" olmalı.", r.Offer.Describe())
		} else {
			bad("Brief'teki teklif %q ama offer alanı %q. offer alanını brief'teki teklifle aynen doldur (kind, value, code, min_spend, expires_at, step_spend, max_discount, extras, scope). Teklifin geçerli olduğu ürünler (ör. \"kış ürünlerinde geçerli\") scope'a yazılır: \"kış ürünleri\". Alışveriş tutarı şartı (ör. \"1000 TL üzeri alışverişe\") teklife aittir: min_spend; son kullanma tarihi expires_at (YYYY-MM-DD). Birlikte verilen farklı türden teklifler (ör. \"%%20 indirim + ücretsiz kargo\") tek teklifin parçalarıdır: biri kind/value, diğerleri extras. \"Her 1000 TL'ye 100 TL\" kademeli indirimdir: kind amount, value 100, step_spend 1000; \"toplamda en fazla 300 TL\" max_discount 300.", in.Offer.Describe(), r.Offer.Describe())
		}
	}

	in.verifyEveryone(r, bad)
	in.verifyGender(r, bad)
	in.verifyAge(r, bad)
	in.verifySpend(r, bad)
	if r.Match == segment.MatchAny && !containsAny(in.folded, " veya ", " ya da ") {
		bad("Brief \"veya\" demiyor ama match \"any\"; koşulların hepsi birlikte geçerli olmalı, match \"all\" kullan.")
	}
	return problems
}

func has(r segment.Rule, field string) bool {
	for _, c := range r.Conditions {
		if c.Field == field {
			return true
		}
	}
	return false
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func (in Intent) verifyCities(r segment.Rule, bad func(string, ...any)) {
	inc, exc := map[string]bool{}, map[string]bool{}
	for _, c := range r.Conditions {
		if c.Field != "city" {
			continue
		}
		target := inc
		if c.Op == segment.OpNeq || c.Op == segment.OpNotIn {
			target = exc
		}
		switch v := c.Value.(type) {
		case string:
			target[v] = true
		case []string:
			for _, s := range v {
				target[s] = true
			}
		}
	}
	compare := func(want []string, got map[string]bool, verb string) {
		wantSet := map[string]bool{}
		for _, w := range want {
			wantSet[w] = true
		}
		var missing, extra []string
		for _, w := range want {
			if !got[w] {
				missing = append(missing, w)
			}
		}
		for g := range got {
			if !wantSet[g] {
				extra = append(extra, g)
			}
		}
		sort.Strings(extra)
		if len(missing) > 0 {
			bad("Brief şehir koşulu içeriyor (%s %s) ama kuralda bu şehir koşulu eksik. city alanıyla ekle.", strings.Join(missing, ", "), verb)
		}
		if len(extra) > 0 {
			bad("Kuralda brief'te geçmeyen şehir var: %s. Yalnızca brief'te yazan şehirleri kullan.", strings.Join(extra, ", "))
		}
	}
	compare(in.Included, inc, "için")
	compare(in.Excluded, exc, "hariç")
}

func (in Intent) verifyPurchase(r segment.Rule, bad func(string, ...any)) {
	phrase := in.NegationPhrase
	if phrase == "" {
		phrase = "satın almamış"
	}
	purchased, hasPurchased := boolCond(r, "purchased_since_last_cart_add")
	switch {
	case in.Cart && in.NotPurchased:
		if !hasPurchased || purchased {
			bad("Brief %q diyor (sepete ekleyip satın almamış kişiler) ama kuralda purchased_since_last_cart_add eq false koşulu yok. Satın alanları dışarıda bırakmak için bu koşulu ekle.", phrase)
		}
	case in.Cart && in.Purchased:
		if !hasPurchased || !purchased {
			bad("Brief sepete ekleyip satın alanları istiyor ama kuralda purchased_since_last_cart_add eq true koşulu yok; ekle.")
		}
	default:
		if hasPurchased {
			bad("Brief sepete ekleme sonrası satın alma durumundan söz etmiyor ama kuralda purchased_since_last_cart_add koşulu var; brief'te olmayan koşulu kaldır.")
		}
	}

	// Without a cart, "did not buy" means order_count = 0 and "bought" means orders exist.
	orderCount := intCond(r, "order_count")
	switch {
	case !in.Cart && in.NotPurchased && in.HasWindow:
		if len(orderCount) > 0 {
			bad("Brief %q diyor; bu \"hiç sipariş vermemiş\" demek değil, belirli bir sürede sipariş vermemiş demek. order_count koşulunu kaldır; days_since_last_order_or_never gt %d kullan.", in.WindowPhrase, in.WindowDays)
		}
	case !in.Cart && in.NotPurchased:
		if !orderCountIsZero(orderCount) {
			bad("Brief %q diyor (hiç sipariş vermemiş kişiler) ama kuralda order_count eq 0 koşulu yok; ekle.", phrase)
		}
	case !in.Cart && in.Purchased && !in.HasWindow:
		if !orderCountIsPositive(orderCount) {
			bad("Brief sipariş verenleri istiyor ama kuralda order_count gt 0 koşulu yok; ekle.")
		}
	default:
		if len(orderCount) > 0 {
			bad("Brief toplam sipariş sayısından söz etmiyor ama kuralda order_count koşulu var; brief'te olmayan koşulu kaldır.")
		}
	}
}

func (in Intent) verifyWindow(r segment.Rule, bad func(string, ...any)) {
	cart := intCond(r, "days_since_last_cart_add")
	order := intCond(r, "days_since_last_order")

	switch {
	case in.Cart && in.HasWindow:
		if !hasWindow(cart, in.WindowDays) {
			bad("Brief %q diyor ama kuralda days_since_last_cart_add lte %d koşulu yok veya değeri yanlış (%s). Süreyi %d gün olarak ekle.", in.WindowPhrase, in.WindowDays, describe(cart), in.WindowDays)
		}
	case in.Cart:
		for _, c := range cart {
			if c.op == segment.OpLte || c.op == segment.OpLt {
				bad("Brief bir zaman aralığı vermiyor ama kuralda days_since_last_cart_add %s %d var; brief'te olmayan süreyi kaldır.", c.op, c.value)
			}
		}
	default:
		if len(cart) > 0 {
			bad("Brief sepete eklemeden söz etmiyor ama kuralda days_since_last_cart_add koşulu var; kaldır.")
		}
	}

	// "Did not order in the last N days" (no cart in the brief): people who never
	// ordered must be included, so only days_since_last_order_or_never is right.
	orNever := intCond(r, "days_since_last_order_or_never")
	noOrderWindow := !in.Cart && in.NotPurchased && in.HasWindow
	if noOrderWindow {
		if !hasNoOrderWindow(orNever, in.WindowDays) {
			bad("Brief %q diyor ama kuralda days_since_last_order_or_never gt %d koşulu yok veya değeri yanlış (%s). Hiç sipariş vermemişleri de kapsaması için days_since_last_order gt DEĞİL, days_since_last_order_or_never gt %d yaz.", in.WindowPhrase, in.WindowDays, describe(orNever), in.WindowDays)
		}
		if len(order) > 0 {
			bad("Kuralda days_since_last_order koşulu var; bu hiç sipariş vermemişleri dışarıda bırakır. Kaldır, yerine days_since_last_order_or_never gt %d kullan.", in.WindowDays)
		}
	} else if len(orNever) > 0 {
		bad("Brief belirli bir sürede sipariş vermemiş kişileri istemiyor ama kuralda days_since_last_order_or_never koşulu var; brief'te olmayan koşulu kaldır.")
	}

	switch {
	case noOrderWindow:
		// handled above
	case !in.Cart && in.Purchased && in.HasWindow:
		if !hasWindow(order, in.WindowDays) {
			bad("Brief %q diyor ama kuralda days_since_last_order lte %d koşulu yok veya değeri yanlış (%s). Süreyi %d gün olarak ekle.", in.WindowPhrase, in.WindowDays, describe(order), in.WindowDays)
		}
	default:
		if len(order) > 0 {
			bad("Brief son sipariş zamanından söz etmiyor ama kuralda days_since_last_order koşulu var; brief'te olmayan koşulu kaldır.")
		}
	}
}

type intCondition struct {
	op    string
	value int64
}

func intCond(r segment.Rule, field string) []intCondition {
	var out []intCondition
	for _, c := range r.Conditions {
		if c.Field == field {
			if v, ok := c.Value.(int64); ok {
				out = append(out, intCondition{c.Op, v})
			}
		}
	}
	return out
}

func boolCond(r segment.Rule, field string) (value, found bool) {
	for _, c := range r.Conditions {
		if c.Field == field {
			if v, ok := c.Value.(bool); ok && c.Op == segment.OpEq {
				return v, true
			}
		}
	}
	return false, false
}

func hasWindow(conds []intCondition, days int) bool {
	for _, c := range conds {
		if c.op == segment.OpLte && c.value == int64(days) {
			return true
		}
	}
	return false
}

func hasNoOrderWindow(conds []intCondition, days int) bool {
	for _, c := range conds {
		if c.op == segment.OpGt && c.value == int64(days) {
			return true
		}
	}
	return false
}

func orderCountIsZero(conds []intCondition) bool {
	for _, c := range conds {
		if ((c.op == segment.OpEq || c.op == segment.OpLte) && c.value == 0) || (c.op == segment.OpLt && c.value == 1) {
			return true
		}
	}
	return false
}

func orderCountIsPositive(conds []intCondition) bool {
	for _, c := range conds {
		if (c.op == segment.OpGt && c.value == 0) || (c.op == segment.OpGte && c.value == 1) {
			return true
		}
	}
	return false
}

func describe(conds []intCondition) string {
	if len(conds) == 0 {
		return "koşul yok"
	}
	parts := make([]string, len(conds))
	for i, c := range conds {
		parts[i] = fmt.Sprintf("%s %d", c.op, c.value)
	}
	return strings.Join(parts, ", ")
}

// verifyEveryone: "all_customers" must be there exactly when the brief says
// everyone, and then it must stand alone.
func (in Intent) verifyEveryone(r segment.Rule, bad func(string, ...any)) {
	switch {
	case in.AudienceDefaulted && !has(r, "all_customers") && in.Occasion != nil:
		bad("Brief bir özel günden söz ediyor (%s) ama kitle belirtmiyor. Özel gün mesajın konusudur, kitleyi daraltmaz: kitle tüm müşterilerdir, yalnızca all_customers eq true koşulunu kullan.", in.Occasion.NameIn("tr"))
	case in.AudienceDefaulted && !has(r, "all_customers"):
		bad("Brief'teki ürünler (%q) teklifin kapsamıdır (offer.scope), kitleyi daraltmaz; brief kitle belirtmediği için kitle tüm müşterilerdir: yalnızca all_customers eq true koşulunu kullan.", in.Offer.Scope)
	case in.Everyone && !has(r, "all_customers"):
		bad("Brief açıkça tüm müşterilere/herkese diyor ama kuralda all_customers eq true koşulu yok. Kitle için yalnızca bu tek koşulu kullan (başka koşul ekleme).")
	case !in.Everyone && has(r, "all_customers"):
		bad("Brief \"herkese / tüm müşterilere\" demiyor ama kuralda all_customers var. Brief'te yazan kitle koşullarını kullan; kitle belirtilmemişse needs_clarification dön.")
	}
}

// verifyGender: a gender condition exactly when the brief explicitly narrows
// the audience by gender. A special day is never a reason for one.
func (in Intent) verifyGender(r segment.Rule, bad func(string, ...any)) {
	var got []string
	for _, c := range r.Conditions {
		if c.Field == "gender" {
			if v, ok := c.Value.(string); ok {
				got = append(got, v)
			}
		}
	}
	switch {
	case in.Gender != "" && (len(got) != 1 || got[0] != in.Gender):
		bad("Brief kitleyi cinsiyete göre sınırlıyor (%q) ama kuralda gender eq %s koşulu yok veya değeri yanlış; aynen ekle.", in.GenderPhrase, in.Gender)
	case in.Gender == "" && len(got) > 0:
		why := "Brief kitleyi cinsiyete göre açıkça sınırlamıyor"
		if in.Occasion != nil {
			why += fmt.Sprintf(" (%s bir özel gündür, mesajın konusudur; kitle değildir)", in.Occasion.NameIn("tr"))
		}
		bad("%s ama kuralda gender koşulu var; kaldır.", why)
	}
}

type numCondition struct {
	op    string
	value float64
}

func numConds(r segment.Rule, field string) []numCondition {
	var out []numCondition
	for _, c := range r.Conditions {
		if c.Field != field {
			continue
		}
		switch v := c.Value.(type) {
		case float64:
			out = append(out, numCondition{c.Op, v})
		case int64:
			out = append(out, numCondition{c.Op, float64(v)})
		}
	}
	return out
}

// verifySpend: a total_spent condition is right only when the brief describes
// a customer's TOTAL spending. An amount that qualifies a purchase ("1000 TL
// üzeri alışverişe") is a term of the offer (offer.min_spend), never an audience.
func (in Intent) verifySpend(r segment.Rule, bad func(string, ...any)) {
	got := numConds(r, "total_spent")
	if !in.HasSpend {
		if len(got) > 0 {
			hint := ""
			if in.Offer.MinSpend > 0 {
				hint = fmt.Sprintf(" Brief'teki %s TL alışveriş tutarı şartı teklife aittir: offer.min_spend = %s; kitleye (total_spent) yazılmaz.",
					offerNumber(in.Offer.MinSpend), offerNumber(in.Offer.MinSpend))
			}
			bad("Brief müşterinin toplam harcamasından söz etmiyor ama kuralda total_spent koşulu var; kaldır.%s", hint)
		}
		return
	}
	for _, g := range got {
		if g.op == in.SpendOp && g.value == in.SpendValue {
			return
		}
	}
	bad("Brief toplam harcama koşulu veriyor (total_spent %s %s) ama kuralda bu koşul yok veya değeri yanlış. Aynen ekle.", in.SpendOp, offerNumber(in.SpendValue))
}

func offerNumber(v float64) string { return offer.FormatNumber(v) }
