package brief_test

import (
	"strings"
	"testing"

	"smartcampaign/internal/segment"
)

// Grounded mode (real Claude): the model reads the brief, but a part the rule
// reader is certain about (a city, did / did not buy, the number of days and
// its direction) may not silently fall out of the rule or turn around. The
// model hears about it once; if it still leaves it out the rule is used, with
// a warning on the audience step (LLM-1).

const cartBrief = "Son 30 günde sepete ürün ekleyip almayanlara %10 indirim"

var (
	window    = c("days_since_last_cart_add", "lte", int64(30))
	notBought = c("purchased_since_last_cart_add", "eq", false)
)

func feedbackOf(t *testing.T, calls int, problems [][]string) string {
	t.Helper()
	if len(problems) == 0 {
		t.Fatalf("no feedback was sent (calls %d)", calls)
	}
	return strings.Join(problems[len(problems)-1], " ")
}

func TestGroundedADroppedConditionIsSentBackOnce(t *testing.T) {
	rule, m := grounded(t, cartBrief, rawRule(t, okRule(pct(10), window)), rawRule(t, okRule(pct(10), window, notBought)))
	calls := m.Calls()
	if len(calls) != 2 {
		t.Fatalf("the model must hear about the missing condition once: %d calls", len(calls))
	}
	fb := feedbackOf(t, len(calls), [][]string{calls[1].Previous[0].Problems})
	if !strings.Contains(fb, "purchased_since_last_cart_add") || !strings.Contains(fb, "almayan") {
		t.Fatalf("the feedback must name the condition and the brief's words: %s", fb)
	}
	if strings.Join(conds(rule), "|") != strings.Join(conds(okRule(pct(10), window, notBought)), "|") || len(rule.Warnings) != 0 {
		t.Fatalf("the corrected rule is used without a warning: %v %v", conds(rule), rule.Warnings)
	}
}

func TestGroundedAConditionStillMissingIsAWarningNotABlock(t *testing.T) {
	rule, m := grounded(t, cartBrief, rawRule(t, okRule(pct(10), window))) // the model keeps leaving it out
	if n := len(m.Calls()); n != 2 {
		t.Fatalf("feedback is sent once, then the rule is used: %d calls", n)
	}
	if rule.Status != segment.StatusOK || strings.Join(conds(rule), "|") != strings.Join(conds(okRule(pct(10), window)), "|") {
		t.Fatalf("the model's rule must be used, not blocked: %+v", rule)
	}
	if len(rule.Warnings) != 1 || !strings.Contains(rule.Warnings[0], "almayan") ||
		!strings.Contains(rule.Warnings[0], "kuralda yok. Kitle beklediğinden geniş olabilir.") {
		t.Fatalf("want one warning naming the missing part: %q", rule.Warnings)
	}
}

func TestGroundedAReversedConditionIsCaught(t *testing.T) {
	tests := []struct {
		name, brief string
		rule        segment.Rule
		phrase      string
	}{
		{"days turned around (gt instead of lte)", cartBrief,
			okRule(pct(10), c("days_since_last_cart_add", "gt", int64(30)), notBought), "30"},
		{"did not buy read as bought (eq true)", cartBrief,
			okRule(pct(10), window, c("purchased_since_last_cart_add", "eq", true)), "almayan"},
		{"an excluded city included", "İstanbul hariç son 30 günde sepete ürün ekleyip almayanlara %10 indirim",
			okRule(pct(10), c("city", "eq", "İstanbul"), window, notBought), "İstanbul"},
		{"no order in N days read as an order within N days", "Son 60 gündür alışveriş yapmayan müşterilere %10 indirim",
			okRule(pct(10), c("days_since_last_order", "lte", int64(60))), "60"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, m := grounded(t, tt.brief, rawRule(t, tt.rule))
			if n := len(m.Calls()); n != 2 {
				t.Fatalf("a reversed condition must go back to the model once: %d calls", n)
			}
			if rule.Status != segment.StatusOK || len(rule.Warnings) != 1 ||
				!strings.Contains(rule.Warnings[0], tt.phrase) || !strings.Contains(rule.Warnings[0], "ters") {
				t.Fatalf("want one non-blocking warning about the reversed part: %+v %q", rule.Status, rule.Warnings)
			}
		})
	}
}

func TestGroundedAMissingCityIsCaught(t *testing.T) {
	b := "Son 30 günde İstanbul'da sepete ürün ekleyip almayanlara %10 indirim"
	rule, m := grounded(t, b, rawRule(t, okRule(pct(10), window, notBought)))
	if n := len(m.Calls()); n != 2 {
		t.Fatalf("calls %d", n)
	}
	if len(rule.Warnings) != 1 || !strings.Contains(rule.Warnings[0], "İstanbul") || !strings.Contains(rule.Warnings[0], "kuralda yok") {
		t.Fatalf("warnings %q", rule.Warnings)
	}
}

// Conditions the model adds, or reads differently, are its interpretation: only
// what is missing or reversed is checked, never what is extra.
func TestGroundedACompleteRuleWithMoreConditionsIsNotQuestioned(t *testing.T) {
	more := okRule(pct(10), window, notBought, c("order_count", "gte", int64(0)))
	rule, m := grounded(t, cartBrief, rawRule(t, more))
	if n := len(m.Calls()); n != 1 || len(rule.Warnings) != 0 {
		t.Fatalf("a complete rule must be used at once: calls %d warnings %q", n, rule.Warnings)
	}
}
