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

// The real model's flow (D87), with a scripted model standing in for Claude.

func grounded(t *testing.T, b string, steps ...mock.Step) (segment.Rule, *mock.Client) {
	t.Helper()
	m := mock.NewScripted(steps, nil)
	rule, err := brief.ResolveSegmentWith(context.Background(), m, b, llm.Hooks{}, brief.Grounded)
	if err != nil {
		t.Fatal(err)
	}
	return rule, m
}

// Briefs the rule reader would stop are given to the model, and its reading
// is used: code does not block, and does not second-guess the interpretation.
func TestGroundedTheModelReadsTheBriefItself(t *testing.T) {
	tests := []struct {
		name  string
		brief string
		rule  segment.Rule
	}{
		{"a calendar month the rule reader asks about", "Bu ay sepete ürün ekleyip almayanlara %10 indirim",
			okRule(pct(10), c("days_since_last_cart_add", "lte", int64(30)), c("purchased_since_last_cart_add", "eq", false))},
		{"wording the rule reader does not know", "Kuponunu kullanmamış müşterilere %10 indirim gönder",
			okRule(pct(10), c("all_customers", "eq", true))},
		{"a reading that differs from the rule reader's (no 'did not buy' condition)", "Son 30 günde sepete ürün ekleyip almayanlara %10 indirim",
			okRule(pct(10), c("days_since_last_cart_add", "lte", int64(30)))},
		{"a VIP audience read as big spenders, numbers from the brief", "5000 TL üzeri harcayan VIP müşterilere %10 indirim",
			okRule(pct(10), c("total_spent", "gt", 5000.0))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, m := grounded(t, tt.brief, rawRule(t, tt.rule))
			if rule.Status != segment.StatusOK || len(m.Calls()) != 1 {
				t.Fatalf("the model's rule must be used as it is: %+v (calls %d)", rule, len(m.Calls()))
			}
			if strings.Join(conds(rule), "|") != strings.Join(conds(tt.rule), "|") {
				t.Fatalf("conditions changed: %v", conds(rule))
			}
		})
	}
}

// The model's own clarification and refusal are its decision.
func TestGroundedTheModelMayAskOrRefuse(t *testing.T) {
	ask := segment.Rule{Status: segment.StatusNeedsClarification, Conditions: []segment.Condition{}, Question: brief.WhoQuestion, Offer: none}
	rule, _ := grounded(t, "%10 indirim gönder", rawRule(t, ask))
	if rule.Status != segment.StatusNeedsClarification || rule.Suggestion != segment.SuggestAllCustomers {
		t.Fatalf("'who?' keeps its one-click answer: %+v", rule)
	}
	no := segment.Rule{Status: segment.StatusUnsupported, Conditions: []segment.Condition{}, Explanation: "Yaş verisi yok.", Offer: none}
	rule, _ = grounded(t, "35 yaş üstü müşterilere %10 indirim", rawRule(t, no))
	if rule.Status != segment.StatusUnsupported {
		t.Fatalf("%+v", rule)
	}
}

// What the model could not have read in the brief is sent back, with the
// reason, and the repaired answer is used.
func TestGroundedInventionsAreRepaired(t *testing.T) {
	tests := []struct {
		name  string
		brief string
		wrong segment.Rule
		right segment.Rule
		say   string // folded word the feedback must contain
	}{
		{"a city that is not in the brief", "İstanbul'daki müşterilere %10 indirim",
			okRule(pct(10), c("city", "eq", "Ankara")), okRule(pct(10), c("city", "eq", "İstanbul")), "ankara"},
		{"a day count that is not in the brief", "Son 30 günde sepete ürün ekleyenlere %10 indirim",
			okRule(pct(10), c("days_since_last_cart_add", "lte", int64(7))), okRule(pct(10), c("days_since_last_cart_add", "lte", int64(30))), "7"},
		{"a percentage that is not in the brief", "Herkese %10 indirim",
			okRule(pct(15), c("all_customers", "eq", true)), okRule(pct(10), c("all_customers", "eq", true)), "15"},
		{"an amount that is not in the brief", "Toplam harcaması 5000 TL'den fazla olanlara %10 indirim",
			okRule(pct(10), c("total_spent", "gt", 3000.0)), okRule(pct(10), c("total_spent", "gt", 5000.0)), "3000"},
		{"an invented coupon code", "Herkese %10 indirim",
			okRule(withCode(pct(10), "HEDIYE10"), c("all_customers", "eq", true)), okRule(pct(10), c("all_customers", "eq", true)), "hediye10"},
		{"an invented year for a date without one", "Herkese %10 indirim, 31 Aralık'a kadar",
			okRule(withTerms(pct(10), 0, "2026-12-31"), c("all_customers", "eq", true)), okRule(pct(10), c("all_customers", "eq", true)), "31.12.2026"},
		{"free shipping the brief does not mention", "Herkese %10 indirim",
			okRule(combo(pct(10), freePart), c("all_customers", "eq", true)), okRule(pct(10), c("all_customers", "eq", true)), "kargo"},
		{"gender from a special day's name only", "Kadınlar Günü'ne özel %20 indirim",
			okRule(pct(20), c("gender", "eq", "female")), okRule(pct(20), c("all_customers", "eq", true)), "gender"},
		{"a tier step that is not in the brief", "Herkese her 1000 TL'ye 100 TL indirim",
			okRule(tier(100, 500, 0), c("all_customers", "eq", true)), okRule(tier(100, 1000, 0), c("all_customers", "eq", true)), "step_spend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, m := grounded(t, tt.brief, rawRule(t, tt.wrong), rawRule(t, tt.right))
			if rule.Status != segment.StatusOK || strings.Join(conds(rule), "|") != strings.Join(conds(tt.right), "|") || !rule.Offer.Equal(tt.right.Offer) {
				t.Fatalf("repaired rule: %+v", rule)
			}
			if fb := feedback(t, m); !strings.Contains(fb, trtext.Fold(tt.say)) {
				t.Errorf("the feedback should name %q: %s", tt.say, fb)
			}
		})
	}
}

// What the brief does say, in its own units, is accepted the first time.
func TestGroundedAcceptsTheBriefsOwnNumbersInOtherUnits(t *testing.T) {
	tests := []struct {
		brief string
		rule  segment.Rule
	}{
		{"Son 2 haftada sepete ürün ekleyenlere yüzde on beş indirim", okRule(pct(15), c("days_since_last_cart_add", "lte", int64(14)))},
		{"Son bir aydır sipariş vermemiş müşterilere %10 indirim", okRule(pct(10), c("days_since_last_order_or_never", "gt", int64(30)))},
		{"Son 48 saatte sepete ekleyenlere hatırlatma", okRule(none, c("days_since_last_cart_add", "lte", int64(2)))},
		{"Hiç sipariş vermemiş müşterilere 1.000 TL üzeri alışverişte 100 TL indirim, son kullanım 31.12.2026",
			okRule(withTerms(amountOf(100), 1000, "2026-12-31"), c("order_count", "eq", int64(0)))},
		{"henüz hiç sipariş vermemiş kullanıcılara ilk siparişine özel 1000 tl ye varan siparişte 300 tl indirim uygula",
			okRule(withTerms(amountOf(300), 1000, ""), c("order_count", "eq", int64(0)))},
		{"Her 1000 TL'lik alışverişe 100 TL indirim, toplamda en fazla 300 TL. İzmir ve Ankara'daki müşterilere.",
			okRule(tier(100, 1000, 300), c("city", "in", []string{"İzmir", "Ankara"}))},
		{"Herkese %20 indirim + ücretsiz kargo, kupon kodu BAHAR20", okRule(withCode(combo(pct(20), freePart), "BAHAR20"), c("all_customers", "eq", true))},
		{"Sepeti terk eden kadın müşterilere %10 indirim", okRule(pct(10), c("gender", "eq", "female"), c("purchased_since_last_cart_add", "eq", false))},
	}
	for _, tt := range tests {
		rule, m := grounded(t, tt.brief, rawRule(t, tt.rule))
		if rule.Status != segment.StatusOK || len(m.Calls()) != 1 {
			t.Errorf("%q: %+v (calls %d) %v", tt.brief, rule, len(m.Calls()), brief.Ground(tt.brief, tt.rule))
		}
	}
}

// A model that keeps inventing ends in a question, never in a rule that
// states something the brief did not.
func TestGroundedAPersistentInventionBecomesAQuestion(t *testing.T) {
	wrong := okRule(pct(10), c("city", "eq", "Bursa"))
	rule, m := grounded(t, "İstanbul'daki müşterilere %10 indirim", rawRule(t, wrong))
	if rule.Status != segment.StatusNeedsClarification || len(rule.Conditions) != 0 || len(m.Calls()) != brief.MaxAttempts {
		t.Fatalf("%+v (calls %d)", rule, len(m.Calls()))
	}
	if !strings.Contains(rule.Question, "Bursa") {
		t.Errorf("the question must say what was wrong: %q", rule.Question)
	}
}

// The schema and offer checks still apply to the real model.
func TestGroundedTheSchemaAndTheOfferAreStillChecked(t *testing.T) {
	badField := mock.Step{Kind: mock.Raw, Raw: `{"name":"Test kampanyası","status":"ok","match":"all","conditions":[{"field":"password","op":"eq","value":"x"}],"explanation":"x","question":"","offer":{"kind":"none","value":0,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}}`}
	badOffer := rawRule(t, okRule(offer.Offer{Kind: offer.KindAmount, Value: 100, MaxDiscount: 300}, c("all_customers", "eq", true)))
	for name, step := range map[string]mock.Step{"field outside the whitelist": badField, "an offer that makes no sense": badOffer} {
		rule, m := grounded(t, "Herkese 100 TL indirim", step, rawRule(t, okRule(amountOf(100), c("all_customers", "eq", true))))
		if rule.Status != segment.StatusOK || len(m.Calls()) != 2 {
			t.Errorf("%s: must be repaired: %+v (calls %d)", name, rule, len(m.Calls()))
		}
	}
}

// Everyone without the brief saying so gets the note; said so, no note.
func TestGroundedNoteWhenTheModelChoseEveryone(t *testing.T) {
	all := okRule(pct(20), c("all_customers", "eq", true))
	rule, _ := grounded(t, "Kadınlar Günü'ne özel %20 indirim", rawRule(t, all))
	if rule.Note != brief.AudienceDefaultedNote {
		t.Errorf("note = %q", rule.Note)
	}
	rule, _ = grounded(t, "Herkese %20 indirim", rawRule(t, all))
	if rule.Note != "" {
		t.Errorf("note = %q", rule.Note)
	}
}

// ---- the other direction: every amount and percentage of the brief is used (D89) ----

const varanBrief = "henüz hiç sipariş vermemiş kullanıcılara ilk siparişine özel 1000 tl ye varan siparişte 300 tl indirim uygula"

// The live case: the model wrote a plain 300 TL discount and dropped the
// 1000 TL. It is told the amount is unused and where it could go; it places it.
func TestGroundedAnUnusedAmountIsSentBack(t *testing.T) {
	dropped := okRule(amountOf(300), c("order_count", "eq", int64(0)))
	right := okRule(withTerms(amountOf(300), 1000, ""), c("order_count", "eq", int64(0)))
	rule, m := grounded(t, varanBrief, rawRule(t, dropped), rawRule(t, right))
	if rule.Status != segment.StatusOK || !rule.Offer.Equal(right.Offer) {
		t.Fatalf("%+v", rule)
	}
	fb := feedback(t, m)
	for _, w := range []string{"1000 tl", "min_spend", "max_discount", "total_spent"} {
		if !strings.Contains(fb, w) {
			t.Errorf("feedback should mention %q: %s", w, fb)
		}
	}
	// Placed as a cap instead (on a percentage, where a cap is valid): also used.
	// Which reading is meant is the model's call, not this check's.
	pctCap := okRule(capped(pct(20), 1000), c("all_customers", "eq", true))
	if rule, m := grounded(t, "Herkese %20 indirim, 1000 TL'ye varan", rawRule(t, pctCap)); rule.Status != segment.StatusOK || len(m.Calls()) != 1 {
		t.Errorf("an amount used as a cap counts as used: %+v", rule)
	}
}

// If the model never places it, the person is asked, with the choices.
func TestGroundedAnAmountThatStaysUnusedBecomesAChoiceQuestion(t *testing.T) {
	dropped := okRule(amountOf(300), c("order_count", "eq", int64(0)))
	rule, m := grounded(t, varanBrief, rawRule(t, dropped))
	if rule.Status != segment.StatusNeedsClarification || len(m.Calls()) != brief.MaxAttempts {
		t.Fatalf("%+v (calls %d)", rule, len(m.Calls()))
	}
	q := trtext.Fold(rule.Question)
	for _, w := range []string{"1000 tl ne icin", "minimum sepet", "ust siniri", "toplam harcamasi"} {
		if !strings.Contains(q, w) {
			t.Errorf("the question should offer %q: %q", w, rule.Question)
		}
	}
}

// An unused percentage is sent back too.
func TestGroundedAnUnusedPercentageIsSentBack(t *testing.T) {
	onlyShipping := okRule(freeShipping, c("all_customers", "eq", true))
	right := okRule(combo(pct(20), freePart), c("all_customers", "eq", true))
	rule, m := grounded(t, "Herkese %20 indirim ve ücretsiz kargo", rawRule(t, onlyShipping), rawRule(t, right))
	if rule.Status != segment.StatusOK || !rule.Offer.Equal(right.Offer) {
		t.Fatalf("%+v", rule)
	}
	if fb := feedback(t, m); !strings.Contains(fb, "%20") {
		t.Errorf("feedback: %s", fb)
	}
}

// Dates and special days are not amounts; day counts are not checked here.
func TestGroundedDatesAndSpecialDaysAreNotAmounts(t *testing.T) {
	for _, tt := range []struct {
		brief string
		rule  segment.Rule
	}{
		{"23 Nisan'a özel herkese %15 indirim, son kullanım 30.04.2027", okRule(withTerms(pct(15), 0, "2027-04-30"), c("all_customers", "eq", true))},
		{"8 Mart'a özel herkese 100 TL indirim, 31 Mart 2027'ye kadar geçerli", okRule(withTerms(amountOf(100), 0, "2027-03-31"), c("all_customers", "eq", true))},
		{"Son 30 günde sepete ekleyip almayanlara %10 indirim", okRule(pct(10), c("days_since_last_cart_add", "lte", int64(30)), c("purchased_since_last_cart_add", "eq", false))},
		{"Toplam harcaması 5000 TL'den fazla olanlara 1000 TL üzeri alışverişte 200 TL indirim",
			okRule(withTerms(amountOf(200), 1000, ""), c("total_spent", "gt", 5000.0))},
	} {
		rule, m := grounded(t, tt.brief, rawRule(t, tt.rule))
		if rule.Status != segment.StatusOK || len(m.Calls()) != 1 {
			t.Errorf("%q: %+v (calls %d) unused=%v", tt.brief, rule, len(m.Calls()), brief.UnusedValues(tt.brief, tt.rule))
		}
	}
}
