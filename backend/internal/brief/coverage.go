package brief

import (
	"fmt"

	"smartcampaign/internal/segment"
)

// Gap is a part of the brief the rule reader is certain about (a city, did or
// did not buy, the number of days and its direction) that a real model's rule
// leaves out or turns around (LLM-1, D111).
//
// It is Grounded mode's coverage check. Verify (Strict mode) also refuses
// conditions the brief does not ask for, a different offer and "any"; in
// Grounded mode those are the model's interpretation (D87), and the rule
// reader's lexicon is narrower than the model's reading, so only what is
// MISSING or REVERSED is checked here, never what is extra.
type Gap struct {
	Phrase   string // the brief's words, shown to the person
	Reversed bool   // the rule has the condition the other way round
	Feedback string // what the model is told (once)
}

// Warning is the non-blocking sentence the audience step shows for a gap that
// the model left in its rule.
func (g Gap) Warning() string {
	if g.Reversed {
		return fmt.Sprintf("Brief'te \"%s\" var, kuralda ters yönde. Kitle beklediğinden farklı olabilir.", g.Phrase)
	}
	return fmt.Sprintf("Brief'te \"%s\" var, kuralda yok. Kitle beklediğinden geniş olabilir.", g.Phrase)
}

// Gaps compares an "ok" rule with what Analyze read in the brief.
func Gaps(in Intent, r segment.Rule) []Gap {
	if r.Status != segment.StatusOK {
		return nil
	}
	var out []Gap
	missing := func(phrase, format string, a ...any) {
		out = append(out, Gap{Phrase: phrase, Feedback: fmt.Sprintf("Brief'te %q var ama kuralda karşılığı yok: ", phrase) + fmt.Sprintf(format, a...)})
	}
	reversed := func(phrase, format string, a ...any) {
		out = append(out, Gap{Phrase: phrase, Reversed: true, Feedback: fmt.Sprintf("Brief'te %q var ama kuralda ters yönde: ", phrase) + fmt.Sprintf(format, a...)})
	}

	// Cities: an included city must be included, an excluded one excluded.
	inc, exc := ruleCities(r)
	for _, city := range in.Included {
		switch {
		case exc[city]:
			reversed(city, "%s dışarıda bırakılmış; brief bu şehri istiyor, city eq/in %s yaz.", city, city)
		case !inc[city]:
			missing(city, "city eq/in %s koşulunu ekle.", city)
		}
	}
	for _, city := range in.Excluded {
		switch {
		case inc[city]:
			reversed(city, "%s kitleye alınmış; brief bu şehri hariç tutuyor, city neq/not_in %s yaz.", city, city)
		case !exc[city]:
			missing(city, "%s hariç tutulmalı: city neq/not_in %s koşulunu ekle.", city, city)
		}
	}

	notBought := in.NegationPhrase
	if notBought == "" {
		notBought = "satın almamış"
	}
	const bought = "satın almış"

	// Did / did not buy after adding to the cart.
	if in.Cart && (in.NotPurchased || in.Purchased) {
		want, phrase := !in.NotPurchased, notBought
		if in.Purchased && !in.NotPurchased {
			phrase = bought
		}
		got, found := boolValue(r, "purchased_since_last_cart_add")
		switch {
		case !found:
			missing(phrase, "purchased_since_last_cart_add eq %t koşulunu ekle.", want)
		case got != want:
			reversed(phrase, "purchased_since_last_cart_add %t olmalı.", want)
		}
	}

	// Without a cart and without a time window: never ordered / has ordered.
	if !in.Cart && !in.HasWindow {
		orders := intCond(r, "order_count")
		switch {
		case in.NotPurchased && orderCountIsPositive(orders):
			reversed(notBought, "order_count eq 0 olmalı (hiç sipariş vermemiş).")
		case in.NotPurchased && !orderCountIsZero(orders):
			missing(notBought, "order_count eq 0 koşulunu ekle.")
		case in.Purchased && !in.NotPurchased && orderCountIsZero(orders):
			reversed(bought, "order_count gt 0 olmalı.")
		case in.Purchased && !in.NotPurchased && !orderCountIsPositive(orders):
			missing(bought, "order_count gt 0 koşulunu ekle.")
		}
	}

	// The number of days and its direction.
	if in.HasWindow {
		phrase, n := in.WindowPhrase, int64(in.WindowDays)
		switch {
		case in.Cart:
			cart := intCond(r, "days_since_last_cart_add")
			switch {
			case within(cart, n):
			case beyond(cart, n):
				reversed(phrase, "son %d gün içinde demek: days_since_last_cart_add lte %d olmalı.", n, n)
			default:
				missing(phrase, "days_since_last_cart_add lte %d koşulunu ekle.", n)
			}
		case in.NotPurchased:
			// "No order in the last N days": the last order is older than N days.
			order := append(intCond(r, "days_since_last_order_or_never"), intCond(r, "days_since_last_order")...)
			switch {
			case beyond(order, n):
			case within(order, n):
				reversed(phrase, "son %d günde sipariş VERMEMİŞ demek: days_since_last_order_or_never gt %d olmalı.", n, n)
			default:
				missing(phrase, "days_since_last_order_or_never gt %d koşulunu ekle.", n)
			}
		case in.Purchased:
			order := intCond(r, "days_since_last_order")
			switch {
			case within(order, n):
			case beyond(order, n):
				reversed(phrase, "son %d günde sipariş vermiş demek: days_since_last_order lte %d olmalı.", n, n)
			default:
				missing(phrase, "days_since_last_order lte %d koşulunu ekle.", n)
			}
		}
	}
	return out
}

// ruleCities are the rule's included and excluded cities.
func ruleCities(r segment.Rule) (inc, exc map[string]bool) {
	inc, exc = map[string]bool{}, map[string]bool{}
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
	return inc, exc
}

// boolValue reads a boolean condition, "neq false" counting as "eq true".
func boolValue(r segment.Rule, field string) (value, found bool) {
	for _, c := range r.Conditions {
		if v, ok := c.Value.(bool); ok && c.Field == field {
			switch c.Op {
			case segment.OpEq:
				return v, true
			case segment.OpNeq:
				return !v, true
			}
		}
	}
	return false, false
}

// within: at most n days (lte n, lt n+1).
func within(conds []intCondition, n int64) bool {
	for _, c := range conds {
		if (c.op == segment.OpLte && c.value == n) || (c.op == segment.OpLt && c.value == n+1) {
			return true
		}
	}
	return false
}

// beyond: more than n days (gt n, gte n+1), or the same number the other way
// round (gte n), which is what a turned-around "within n days" looks like.
func beyond(conds []intCondition, n int64) bool {
	for _, c := range conds {
		if (c.op == segment.OpGt && c.value == n) || (c.op == segment.OpGte && (c.value == n+1 || c.value == n)) {
			return true
		}
	}
	return false
}
