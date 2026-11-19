package brief_test

import (
	"context"
	"strings"
	"testing"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
	"smartcampaign/internal/trtext"
)

// The real model's flow (D113), with a scripted model standing in for Claude.
// Claude alone reads the brief; code checks only what is technical.

func modelDriven(t *testing.T, b string, steps ...mock.Step) (segment.Rule, *mock.Client) {
	t.Helper()
	m := mock.NewScripted(steps, nil)
	rule, err := brief.ResolveSegmentWith(context.Background(), m, b, llm.Hooks{}, brief.ModelDriven)
	if err != nil {
		t.Fatal(err)
	}
	return rule, m
}

func rawJSON(s string) mock.Step { return mock.Step{Kind: mock.Raw, Raw: s} }

// Turkish, English and complex briefs: the model's rule is used exactly as it
// came, after one call. Briefs the rule reader (mock mode) would stop are not
// stopped here.
func TestModelDrivenUsesTheModelsReadingAsItIs(t *testing.T) {
	tests := []struct {
		name  string
		brief string
		rule  segment.Rule
	}{
		{"Turkish: the case's example brief",
			"Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun.",
			okRule(pct(30), c("city", "eq", "İstanbul"), c("days_since_last_cart_add", "lte", int64(30)), c("purchased_since_last_cart_add", "eq", false))},
		{"English: cart abandoners outside a city",
			"Send a 15% discount to cart abandoners from the last 7 days who didn't purchase, except customers in Ankara.",
			okRule(pct(15), c("city", "neq", "Ankara"), c("days_since_last_cart_add", "lte", int64(7)), c("purchased_since_last_cart_add", "eq", false))},
		{"complex: tiered offer, two cities, spending audience",
			"İzmir ve Ankara'da toplam harcaması 5000 TL'yi geçen kadın müşterilere her 1000 TL'ye 100 TL indirim, toplamda en fazla 300 TL, son kullanım 31.12.2026",
			okRule(withTerms(tier(100, 1000, 300), 0, "2026-12-31"), c("city", "in", []string{"Ankara", "İzmir"}), c("gender", "eq", "female"), c("total_spent", "gt", 5000.0))},
		{"a calendar phrase the rule reader would ask about", "Bu ay sepete ürün ekleyip almayanlara %10 indirim",
			okRule(pct(10), c("days_since_last_cart_add", "lte", int64(30)), c("purchased_since_last_cart_add", "eq", false))},
		{"\"or\" between two requirements", "Son 60 günde sipariş vermemiş ya da İzmir'de yaşayan müşterilere ücretsiz kargo",
			segment.Rule{Status: segment.StatusOK, Match: segment.MatchAny, Explanation: "x", Offer: freeShipping,
				Conditions: []segment.Condition{c("days_since_last_order_or_never", "gt", int64(60)), c("city", "eq", "İzmir")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, m := modelDriven(t, tt.brief, rawRule(t, tt.rule))
			if rule.Status != segment.StatusOK || len(m.Calls()) != 1 {
				t.Fatalf("the model's rule must be used after one call: %+v (calls %d)", rule, len(m.Calls()))
			}
			if strings.Join(conds(rule), "|") != strings.Join(conds(tt.rule), "|") || rule.Match != tt.rule.Match || !rule.Offer.Equal(tt.rule.Offer) {
				t.Fatalf("the rule changed: %v %s %+v", conds(rule), rule.Match, rule.Offer)
			}
		})
	}
}

// Code never re-reads the brief to judge the model: a condition the brief
// names but the model left out, a city or a number the brief does not
// contain, an amount the model did not use. These were caught by the removed
// brief checks (Ground, UnusedValues, Gaps); now the model is the only
// interpreter and the marketer reviews the result before anything is saved.
func TestModelDrivenCodeDoesNotCompareTheRuleWithTheBrief(t *testing.T) {
	tests := []struct {
		name  string
		brief string
		rule  segment.Rule
	}{
		{"a dropped 'did not buy'", "Son 30 günde sepete ürün ekleyip almayanlara %10 indirim",
			okRule(pct(10), c("days_since_last_cart_add", "lte", int64(30)))},
		{"a city the brief does not name", "İstanbul'daki müşterilere %10 indirim", okRule(pct(10), c("city", "eq", "Ankara"))},
		{"a day count the brief does not give", "Son 30 günde sepete ürün ekleyenlere %10 indirim",
			okRule(pct(10), c("days_since_last_cart_add", "lte", int64(7)))},
		{"an amount of the brief left unused", "henüz hiç sipariş vermemiş kullanıcılara 1000 tl ye varan siparişte 300 tl indirim",
			okRule(amountOf(300), c("order_count", "eq", int64(0)))},
		{"gender for a special day", "Kadınlar Günü'ne özel %20 indirim", okRule(pct(20), c("gender", "eq", "female"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, m := modelDriven(t, tt.brief, rawRule(t, tt.rule))
			if rule.Status != segment.StatusOK || len(m.Calls()) != 1 || len(rule.Warnings) != 0 {
				t.Fatalf("no semantic check may send it back or warn: %+v (calls %d)", rule, len(m.Calls()))
			}
			if strings.Join(conds(rule), "|") != strings.Join(conds(tt.rule), "|") {
				t.Fatalf("conditions changed: %v", conds(rule))
			}
		})
	}
}

// What is technical is still checked and repaired, whatever the brief says:
// schema, whitelist, operator per field, value type and range, the business
// constraints of a rule and of its offer, and the campaign name.
func TestModelDrivenTechnicalProblemsAreRepaired(t *testing.T) {
	const offerNone = `"offer":{"kind":"none","value":0,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}`
	okWith := func(conds string) string {
		return `{"name":"Test kampanyası","status":"ok","match":"all","conditions":[` + conds + `],"explanation":"x","question":"",` + offerNone + `}`
	}
	right := rawRule(t, okRule(none, c("city", "eq", "İstanbul")))
	tests := []struct {
		name  string
		wrong mock.Step
		say   string // folded word the feedback must contain
	}{
		{"not JSON", rawJSON(`Here is the rule: {"status":"ok"`), "json"},
		{"a field outside the whitelist", rawJSON(okWith(`{"field":"password","op":"eq","value":"x"}`)), "password"},
		{"an operator the field does not take", rawJSON(okWith(`{"field":"city","op":"gt","value":"İstanbul"}`)), "gecersiz"},
		{"a value of the wrong type", rawJSON(okWith(`{"field":"days_since_last_cart_add","op":"lte","value":"otuz"}`)), "days_since_last_cart_add"},
		{"a value out of range", rawJSON(okWith(`{"field":"days_since_last_cart_add","op":"lte","value":-5}`)), "days_since_last_cart_add"},
		{"SQL in a value", rawJSON(okWith(`{"field":"city","op":"eq","value":"x' OR '1'='1"}`)), "city"},
		{"an unknown property", rawJSON(`{"name":"T","status":"ok","match":"all","conditions":[{"field":"city","op":"eq","value":"İstanbul"}],"explanation":"x","question":"","sql":"DROP TABLE users",` + offerNone + `}`), "sql"},
		{"everyone next to another condition", rawJSON(okWith(`{"field":"all_customers","op":"eq","value":true},{"field":"city","op":"eq","value":"İstanbul"}`)), "all_customers"},
		{"ok without conditions", rawJSON(okWith(``)), "en az bir kosul"},
		{"a question with an ok rule", rawJSON(`{"name":"T","status":"ok","match":"all","conditions":[{"field":"city","op":"eq","value":"İstanbul"}],"explanation":"x","question":"Emin misiniz?",` + offerNone + `}`), "question"},
		{"an offer that makes no sense", rawRule(t, okRule(offer.Offer{Kind: offer.KindPercent, Value: 150}, c("city", "eq", "İstanbul"))), "150"},
		{"no campaign name", rawJSON(`{"name":"","status":"ok","match":"all","conditions":[{"field":"city","op":"eq","value":"İstanbul"}],"explanation":"x","question":"",` + offerNone + `}`), "name bos olamaz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, m := modelDriven(t, "İstanbul'daki müşterilere hatırlatma", tt.wrong, right)
			if rule.Status != segment.StatusOK || strings.Join(conds(rule), "|") != "city eq İstanbul" {
				t.Fatalf("repaired rule: %+v", rule)
			}
			if fb := feedback(t, m); !strings.Contains(fb, trtext.Fold(tt.say)) {
				t.Errorf("the feedback should name %q: %s", tt.say, fb)
			}
		})
	}
}

// Output that stays invalid is an error with every attempt, never a rule and
// never a question made up by code.
func TestModelDrivenOutputThatStaysInvalidIsAnError(t *testing.T) {
	m := mock.NewScripted([]mock.Step{rawJSON(`{"status":"ok","conditions":[{"field":"age","op":"gt","value":30}]}`)}, nil)
	rule, err := brief.ResolveSegmentWith(context.Background(), m, "30 yaş üstü müşterilere %10 indirim", llm.Hooks{}, brief.ModelDriven)
	ex, ok := llm.IsExhausted(err)
	if !ok || len(ex.Attempts) != brief.MaxAttempts || len(m.Calls()) != brief.MaxAttempts {
		t.Fatalf("want an exhausted error after %d attempts: %v (calls %d)", brief.MaxAttempts, err, len(m.Calls()))
	}
	if rule.Status != "" || len(rule.Conditions) != 0 {
		t.Fatalf("no rule may come out: %+v", rule)
	}
}

// When the model cannot tell what the brief means it asks (or says what it
// cannot serve); its answer reaches the person unchanged.
func TestModelDrivenTheModelAsksOrRefuses(t *testing.T) {
	who := segment.Rule{Status: segment.StatusNeedsClarification, Conditions: []segment.Condition{}, Question: brief.WhoQuestion, Offer: none}
	rule, m := modelDriven(t, "%10 indirim gönder", rawRule(t, who))
	if rule.Status != segment.StatusNeedsClarification || rule.Suggestion != segment.SuggestAllCustomers || len(m.Calls()) != 1 {
		t.Fatalf("'who?' keeps its one-click answer: %+v", rule)
	}

	const vague = "\"VIP\" müşterileri neye göre seçeyim: toplam harcama mı, sipariş sayısı mı?"
	ask := segment.Rule{Status: segment.StatusNeedsClarification, Conditions: []segment.Condition{}, Question: vague, Note: "x", Offer: none}
	rule, _ = modelDriven(t, "VIP müşterilere %10 indirim", rawRule(t, ask))
	if rule.Status != segment.StatusNeedsClarification || rule.Question != vague || rule.Suggestion != "" || rule.Note != "" {
		t.Fatalf("the model's own question, nothing added: %+v", rule)
	}

	en := segment.Rule{Status: segment.StatusNeedsClarification, Conditions: []segment.Condition{}, Question: "Which time window do you mean by \"recently\"?", Offer: none}
	rule, _ = modelDriven(t, "Send 10% off to people who recently added to cart", rawRule(t, en))
	if rule.Status != segment.StatusNeedsClarification || rule.Question != en.Question {
		t.Fatalf("an English question is passed on: %+v", rule)
	}

	no := segment.Rule{Status: segment.StatusUnsupported, Conditions: []segment.Condition{}, Explanation: "Yaş verisi yok.", Offer: none}
	rule, _ = modelDriven(t, "35 yaş üstü müşterilere %10 indirim", rawRule(t, no))
	if rule.Status != segment.StatusUnsupported || rule.Explanation != "Yaş verisi yok." {
		t.Fatalf("%+v", rule)
	}
}

// The note on an ok audience is the model's; code adds none and keeps no
// warnings.
func TestModelDrivenTheNoteIsTheModels(t *testing.T) {
	all := okRule(pct(20), c("all_customers", "eq", true))
	all.Note = brief.AudienceDefaultedNote
	all.Warnings = []string{"written by a model, not accepted"}
	rule, _ := modelDriven(t, "Kadınlar Günü'ne özel %20 indirim", rawRule(t, all))
	if rule.Note != brief.AudienceDefaultedNote || len(rule.Warnings) != 0 {
		t.Fatalf("note %q warnings %q", rule.Note, rule.Warnings)
	}
	plain := okRule(pct(20), c("all_customers", "eq", true))
	rule, _ = modelDriven(t, "Kadınlar Günü'ne özel %20 indirim", rawRule(t, plain))
	if rule.Note != "" {
		t.Fatalf("code must not add a note: %q", rule.Note)
	}
}
