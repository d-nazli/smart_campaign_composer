package brief_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
	"smartcampaign/internal/trtext"
)

// ---- helpers ---------------------------------------------------------------

// cond renders a condition the way the table writes it: "field op value".
func cond(c segment.Condition) string {
	switch v := c.Value.(type) {
	case []string:
		s := append([]string(nil), v...)
		sort.Strings(s)
		return fmt.Sprintf("%s %s [%s]", c.Field, c.Op, strings.Join(s, ","))
	default:
		return fmt.Sprintf("%s %s %v", c.Field, c.Op, v)
	}
}

func conds(r segment.Rule) []string {
	out := make([]string, 0, len(r.Conditions))
	for _, c := range r.Conditions {
		out = append(out, cond(c))
	}
	sort.Strings(out)
	return out
}

var (
	none         = offer.Offer{Kind: offer.KindNone}
	pct          = func(v float64) offer.Offer { return offer.Offer{Kind: offer.KindPercent, Value: v} }
	freeShipping = offer.Offer{Kind: offer.KindFreeShipping}
)

const (
	cartWindow = "days_since_last_cart_add lte "
	abandoned  = "purchased_since_last_cart_add eq false"
	noOrder    = "days_since_last_order_or_never gt "
)

// ---- the table -------------------------------------------------------------

// Each row is one way a marketer might write a brief. want is the exact set of
// conditions; offer is the exact incentive; status is the only acceptable
// outcome. For needs_clarification / unsupported rows, say is a word the
// message shown on screen must contain, so the person knows WHAT was not
// understood (nothing may be dropped silently).
type row struct {
	name   string
	brief  string
	status string
	want   []string
	offer  offer.Offer
	say    string
}

var table = []row{
	// ---- the brief that was reported -------------------------------------
	{name: "reported brief (Adana, 30 days, no order, yüzde 15)",
		brief:  "son 30 günde adanada sepete ekleme yapmış ancak sipariş oluşturmamış kişilere yüzde 15 indirim kuponunu tanımla",
		status: segment.StatusOK, want: []string{"city eq Adana", cartWindow + "30", abandoned}, offer: pct(15)},
	{name: "CASE.md example brief",
		brief:  "Son 30 günde sepete ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun.",
		status: segment.StatusOK, want: []string{"city eq İstanbul", cartWindow + "30", abandoned}, offer: pct(30)},

	// ---- not buying: every way to say it ---------------------------------
	{name: "not buying: sipariş vermemiş", brief: "Son 30 günde İstanbul'da sepete ürün ekleyip sipariş vermemiş kullanıcılara %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq İstanbul", cartWindow + "30", abandoned}, offer: pct(10)},
	{name: "not buying: sipariş oluşturmamış", brief: "Son 30 günde İstanbul'da sepete ürün ekleyip sipariş oluşturmamış kullanıcılara %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq İstanbul", cartWindow + "30", abandoned}, offer: pct(10)},
	{name: "not buying: almamış", brief: "Son 30 günde İstanbul'da sepete ürün ekleyip almamış kullanıcılara %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq İstanbul", cartWindow + "30", abandoned}, offer: pct(10)},
	{name: "not buying: satın almamış", brief: "Son 30 günde İstanbul'da sepete ürün ekleyip satın almamış kullanıcılara %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq İstanbul", cartWindow + "30", abandoned}, offer: pct(10)},
	{name: "not buying: alışveriş yapmayan", brief: "Son 30 günde İstanbul'da sepete ürün ekleyip alışveriş yapmayan kullanıcılara %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq İstanbul", cartWindow + "30", abandoned}, offer: pct(10)},
	{name: "not buying: sepeti terk eden", brief: "Son 30 günde İstanbul'da sepeti terk eden kullanıcılara %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq İstanbul", cartWindow + "30", abandoned}, offer: pct(10)},
	{name: "not buying: ödemeyi tamamlamamış", brief: "Son 30 günde İstanbul'da ödemeyi tamamlamamış kullanıcılara %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq İstanbul", cartWindow + "30", abandoned}, offer: pct(10)},
	{name: "never bought, no cart: alışveriş yapmayan", brief: "Ankara'daki alışveriş yapmayan müşterilere %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq Ankara", "order_count eq 0"}, offer: pct(10)},
	{name: "never bought, no cart: hiç sipariş vermemiş", brief: "Ankara'daki hiç sipariş vermemiş müşterilere %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq Ankara", "order_count eq 0"}, offer: pct(10)},
	{name: "bought in a window", brief: "Son 14 günde sipariş veren müşterilere teşekkür et",
		status: segment.StatusOK, want: []string{"days_since_last_order lte 14"}, offer: none},
	{name: "cart add, then bought", brief: "Son 30 günde sepete ürün ekleyip satın alan müşterilere teşekkür et",
		status: segment.StatusOK, want: []string{cartWindow + "30", "purchased_since_last_cart_add eq true"}, offer: none},
	{name: "cart add only", brief: "Son 7 günde sepete ürün ekleyenlere hatırlatma gönder",
		status: segment.StatusOK, want: []string{cartWindow + "7"}, offer: none},
	{name: "welcome offer without an amount", brief: "Hiç alışveriş yapmamış müşterilere hoş geldin indirimi",
		status: segment.StatusOK, want: []string{"order_count eq 0"}, offer: none},

	// ---- time ------------------------------------------------------------
	{name: "time: son 30 gün", brief: "Son 30 günde sepete ürün ekleyip almayanlara %10 indirim", status: segment.StatusOK, want: []string{cartWindow + "30", abandoned}, offer: pct(10)},
	{name: "time: son 2 hafta", brief: "Son 2 haftada sepete ürün ekleyip almayanlara %10 indirim", status: segment.StatusOK, want: []string{cartWindow + "14", abandoned}, offer: pct(10)},
	{name: "time: son iki hafta", brief: "Son iki haftada sepete ürün ekleyip almayanlara %10 indirim", status: segment.StatusOK, want: []string{cartWindow + "14", abandoned}, offer: pct(10)},
	{name: "time: son bir ay", brief: "Son bir ayda sepete ürün ekleyip almayanlara %10 indirim", status: segment.StatusOK, want: []string{cartWindow + "30", abandoned}, offer: pct(10)},
	{name: "time: son 3 ay", brief: "Son 3 ayda sepete ürün ekleyip almayanlara %10 indirim", status: segment.StatusOK, want: []string{cartWindow + "90", abandoned}, offer: pct(10)},
	{name: "time: bu ay (calendar, not a rolling window)", brief: "Bu ay sepete ürün ekleyip almayanlara %10 indirim", status: segment.StatusNeedsClarification, say: "bu ay"},
	{name: "time: geçen hafta (calendar, not a rolling window)", brief: "Geçen hafta sepete ürün ekleyip almayanlara %10 indirim", status: segment.StatusNeedsClarification, say: "geçen hafta"},
	{name: "time: window with nothing to apply it to", brief: "Son 30 günde müşterilere %10 indirim gönder", status: segment.StatusNeedsClarification, say: "30"},
	// "No order in the last N days" is the most common campaign. People who never
	// ordered count too, so it is one NULL-inclusive field (D34 amendment, D50).
	{name: "no order in window: sipariş vermemiş", brief: "Son 30 günde sipariş vermemiş müşterilere %10 indirim gönder",
		status: segment.StatusOK, want: []string{noOrder + "30"}, offer: pct(10)},
	{name: "no order in window: sipariş oluşturmamış", brief: "Son 30 günde sipariş oluşturmamış müşterilere %10 indirim gönder",
		status: segment.StatusOK, want: []string{noOrder + "30"}, offer: pct(10)},
	{name: "no order in window: alışveriş yapmayan, 2 hafta", brief: "Son 2 haftada alışveriş yapmayan müşterilere %10 indirim gönder",
		status: segment.StatusOK, want: []string{noOrder + "14"}, offer: pct(10)},
	{name: "no order in window: son bir ay, yüzde 15", brief: "Son bir aydır satın alma yapmamış kişilere yüzde 15 indirim tanımla",
		status: segment.StatusOK, want: []string{noOrder + "30"}, offer: pct(15)},
	{name: "no order in window: with a city", brief: "Son 30 günde Adana'daki sipariş vermemiş müşterilere %10 indirim gönder",
		status: segment.StatusOK, want: []string{"city eq Adana", noOrder + "30"}, offer: pct(10)},
	{name: "no order in window: excluding a city", brief: "İstanbul hariç, son 60 günde sipariş vermemiş müşterilere ücretsiz kargo",
		status: segment.StatusOK, want: []string{"city neq İstanbul", noOrder + "60"}, offer: freeShipping},
	{name: "no order in window: still means the cart rule when a cart is named", brief: "Son 30 günde sepete ürün ekleyip sipariş vermemiş müşterilere %10 indirim gönder",
		status: segment.StatusOK, want: []string{cartWindow + "30", abandoned}, offer: pct(10)},

	// ---- cities ----------------------------------------------------------
	{name: "city: adanada", brief: "Sepeti terk eden adanada yaşayanlara %10 indirim", status: segment.StatusOK, want: []string{"city eq Adana", abandoned}, offer: pct(10)},
	{name: "city: Adana'daki", brief: "Sepeti terk eden Adana'daki müşterilere %10 indirim", status: segment.StatusOK, want: []string{"city eq Adana", abandoned}, offer: pct(10)},
	{name: "city: ADANA", brief: "Sepeti terk eden ADANA müşterilerine %10 indirim", status: segment.StatusOK, want: []string{"city eq Adana", abandoned}, offer: pct(10)},
	{name: "city: İzmir ve Ankara'daki", brief: "Sepeti terk eden İzmir ve Ankara'daki müşterilere %10 indirim", status: segment.StatusOK, want: []string{"city in [Ankara,İzmir]", abandoned}, offer: pct(10)},
	{name: "city: İstanbul hariç", brief: "İstanbul hariç sepeti terk eden müşterilere %10 indirim", status: segment.StatusOK, want: []string{"city neq İstanbul", abandoned}, offer: pct(10)},
	{name: "city: İzmir ve Ankara hariç", brief: "İzmir ve Ankara hariç sepeti terk eden müşterilere %10 indirim", status: segment.StatusOK, want: []string{"city not_in [Ankara,İzmir]", abandoned}, offer: pct(10)},

	// ---- offer: every way to write it ------------------------------------
	{name: "offer: %15", brief: "Sepeti terk edenlere %15 indirim kuponu tanımla", status: segment.StatusOK, want: []string{abandoned}, offer: pct(15)},
	{name: "offer: yüzde 15", brief: "Sepeti terk edenlere yüzde 15 indirim kuponu tanımla", status: segment.StatusOK, want: []string{abandoned}, offer: pct(15)},
	{name: "offer: 15% indirim", brief: "Sepeti terk edenlere 15% indirim kuponu tanımla", status: segment.StatusOK, want: []string{abandoned}, offer: pct(15)},
	{name: "offer: yüzde on beş", brief: "Sepeti terk edenlere yüzde on beş indirim kuponu tanımla", status: segment.StatusOK, want: []string{abandoned}, offer: pct(15)},
	{name: "offer: 100 TL indirim", brief: "Sepeti terk edenlere 100 TL indirim kuponu tanımla", status: segment.StatusOK, want: []string{abandoned}, offer: offer.Offer{Kind: offer.KindAmount, Value: 100}},
	{name: "offer: 1.000 TL", brief: "Sepeti terk edenlere 1.000 TL indirim kuponu tanımla", status: segment.StatusOK, want: []string{abandoned}, offer: offer.Offer{Kind: offer.KindAmount, Value: 1000}},
	{name: "offer: ücretsiz kargo", brief: "Sepeti terk edenlere ücretsiz kargo fırsatı sun", status: segment.StatusOK, want: []string{abandoned}, offer: freeShipping},
	{name: "offer: coupon code only", brief: "Sepeti terk edenlere HOSGELDIN10 kupon kodunu gönder", status: segment.StatusOK, want: []string{abandoned}, offer: offer.Offer{Kind: offer.KindCouponCode, Code: "HOSGELDIN10"}},
	{name: "offer: percent together with a code", brief: "Sepeti terk edenlere %15 indirim, kupon kodu: YAZ25", status: segment.StatusOK, want: []string{abandoned}, offer: offer.Offer{Kind: offer.KindPercent, Value: 15, Code: "YAZ25"}},
	{name: "offer: no offer at all", brief: "Sepeti terk edenlere hatırlatma gönder", status: segment.StatusOK, want: []string{abandoned}, offer: none},
	{name: "offer: two different offers", brief: "Sepeti terk edenlere %15 indirim ve ücretsiz kargo ver", status: segment.StatusNeedsClarification, say: "teklif"},
	{name: "offer: two different percentages", brief: "Sepeti terk edenlere %15 indirim, VIP olanlara %20 indirim ver", status: segment.StatusNeedsClarification, say: "teklif"},
	{name: "offer: impossible percentage", brief: "Sepeti terk edenlere %150 indirim ver", status: segment.StatusNeedsClarification, say: "150"},
	{name: "offer: 'kod' without a code", brief: "Sepeti terk edenlere indirim kodu gönder", status: segment.StatusNeedsClarification, say: "kod"},

	// ---- things we cannot do: must be said out loud ----------------------
	{name: "unsupported: age", brief: "35 yaş üstü sepeti terk eden müşterilere %10 indirim", status: segment.StatusUnsupported, say: "yaş"},
	{name: "unsupported: age, other wording", brief: "30 yaşındaki müşterilere kampanya", status: segment.StatusUnsupported, say: "yaş"},
	{name: "unsupported: gender", brief: "Sepeti terk eden kadın müşterilere %10 indirim", status: segment.StatusUnsupported, say: "kadın"},
	{name: "unsupported: gender, other wording", brief: "Cinsiyeti erkek olan müşterilere kampanya", status: segment.StatusUnsupported, say: "cinsiyet"},
	{name: "unsupported: product category", brief: "Ayakkabı kategorisinde sepete ürün ekleyenlere %10 indirim", status: segment.StatusUnsupported, say: "kategori"},
	{name: "unsupported: product, no word 'category'", brief: "Sepetine telefon ekleyip almayanlara %10 indirim", status: segment.StatusUnsupported, say: "telefon"},
	{name: "unsupported: birthday", brief: "Doğum günü bu hafta olanlara %10 indirim", status: segment.StatusUnsupported, say: "doğum"},

	// ---- not understood: ask, do not guess -------------------------------
	{name: "clarify: nothing to select on", brief: "Herkese merhaba de", status: segment.StatusNeedsClarification, say: "kimlere"},
	{name: "clarify: a negation we have no field for", brief: "Kuponunu kullanmamış müşterilere %10 indirim gönder", status: segment.StatusNeedsClarification, say: "kullanmamış"},
	{name: "clarify: contradictory purchase wording", brief: "Sipariş veren ama siparişi tamamlamayan müşterilere %10 indirim", status: segment.StatusNeedsClarification, say: "sipariş"},
}

func TestBriefTable(t *testing.T) {
	for _, tt := range table {
		t.Run(tt.name, func(t *testing.T) {
			m := mock.New(0)
			rule, err := brief.ResolveSegment(context.Background(), m, tt.brief, llm.Hooks{})
			if err != nil {
				t.Fatalf("ResolveSegment: %v", err)
			}
			if rule.Status != tt.status {
				t.Fatalf("status = %q, want %q\nquestion=%q\nexplanation=%q\nconditions=%v", rule.Status, tt.status, rule.Question, rule.Explanation, conds(rule))
			}
			if tt.status != segment.StatusOK {
				if len(rule.Conditions) != 0 {
					t.Errorf("a %s answer must not carry conditions: %v", tt.status, conds(rule))
				}
				shown := trtext.Fold(rule.Question + " " + rule.Explanation)
				if !strings.Contains(shown, trtext.Fold(tt.say)) {
					t.Errorf("the message shown to the user must name %q, got question=%q explanation=%q", tt.say, rule.Question, rule.Explanation)
				}
				return
			}
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if got := conds(rule); strings.Join(got, " | ") != strings.Join(want, " | ") {
				t.Errorf("conditions:\n got  %v\n want %v", got, want)
			}
			if !rule.Offer.Equal(tt.offer) {
				t.Errorf("offer = %+v, want %+v", rule.Offer, tt.offer)
			}
		})
	}
}

// Whatever the mock answers must also pass the real validator and the
// backend's own coverage check, so the table cannot pass by accident.
func TestEveryTableRowIsStableAcrossRuns(t *testing.T) {
	for _, tt := range table {
		a, errA := brief.ResolveSegment(context.Background(), mock.New(0), tt.brief, llm.Hooks{})
		b, errB := brief.ResolveSegment(context.Background(), mock.New(0), tt.brief, llm.Hooks{})
		if errA != nil || errB != nil {
			t.Fatalf("%s: %v %v", tt.name, errA, errB)
		}
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if string(ja) != string(jb) {
			t.Errorf("%s: not deterministic:\n%s\n%s", tt.name, ja, jb)
		}
	}
}

// ---- the backend catches a model that drops something ----------------------

const reportedBrief = "son 30 günde adanada sepete ekleme yapmış ancak sipariş oluşturmamış kişilere yüzde 15 indirim kuponunu tanımla"

func rawRule(t *testing.T, r segment.Rule) mock.Step {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return mock.Step{Kind: mock.Raw, Raw: string(b)}
}

func c(field, op string, v any) segment.Condition {
	return segment.Condition{Field: field, Op: op, Value: v}
}

func okRule(offer offer.Offer, cs ...segment.Condition) segment.Rule {
	return segment.Rule{Status: segment.StatusOK, Match: segment.MatchAll, Conditions: cs, Explanation: "x", Offer: offer}
}

func TestBackendCatchesWhatTheModelDrops(t *testing.T) {
	city := c("city", "eq", "Adana")
	window := c("days_since_last_cart_add", "lte", int64(30))
	notBought := c("purchased_since_last_cart_add", "eq", false)

	tests := []struct {
		name    string
		wrong   segment.Rule
		problem string // a word the repair feedback must contain
	}{
		{"forgot the 'did not order' condition", okRule(pct(15), city, window), "satın"},
		{"forgot the city", okRule(pct(15), window, notBought), "şehir"},
		{"forgot the time window", okRule(pct(15), city, notBought), "30"},
		{"wrong time window", okRule(pct(15), city, c("days_since_last_cart_add", "lte", int64(7)), notBought), "30"},
		{"forgot the offer", okRule(none, city, window, notBought), "teklif"},
		{"wrong offer", okRule(pct(20), city, window, notBought), "teklif"},
		{"invented a city", okRule(pct(15), city, c("city", "eq", "Bursa"), window, notBought), "şehir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mock.NewScripted([]mock.Step{rawRule(t, tt.wrong), {Kind: mock.Smart}}, nil)
			rule, err := brief.ResolveSegment(context.Background(), m, reportedBrief, llm.Hooks{})
			if err != nil {
				t.Fatal(err)
			}
			if rule.Status != segment.StatusOK || strings.Join(conds(rule), "|") != "city eq Adana|days_since_last_cart_add lte 30|purchased_since_last_cart_add eq false" || !rule.Offer.Equal(pct(15)) {
				t.Fatalf("the repaired rule is still wrong: %+v", rule)
			}
			calls := m.Calls()
			if len(calls) != 2 || len(calls[1].Previous) != 1 {
				t.Fatalf("expected one repair round, got %d calls", len(calls))
			}
			if feedback := strings.Join(calls[1].Previous[0].Problems, " "); !strings.Contains(trtext.Fold(feedback), trtext.Fold(tt.problem)) {
				t.Errorf("repair feedback should mention %q, got: %s", tt.problem, feedback)
			}
		})
	}
}

// "No order in the last 30 days" has a trap: days_since_last_order gt 30 looks
// right but silently drops everyone who never ordered. The backend must reject
// it and every other wrong shape, then accept the NULL-inclusive field.
func TestNoOrderInWindowRejectsTheLookalikes(t *testing.T) {
	const b = "Son 30 günde sipariş vermemiş müşterilere %10 indirim gönder"
	right := c("days_since_last_order_or_never", "gt", int64(30))
	tests := []struct {
		name  string
		wrong segment.Rule
	}{
		{"misses people who never ordered", okRule(pct(10), c("days_since_last_order", "gt", int64(30)))},
		{"only people who never ordered", okRule(pct(10), c("order_count", "eq", int64(0)))},
		{"forgot the condition", okRule(pct(10), c("city", "eq", "Adana"))},
		{"wrong window", okRule(pct(10), c("days_since_last_order_or_never", "gt", int64(7)))},
		{"invented a cart condition", okRule(pct(10), right, c("purchased_since_last_cart_add", "eq", false))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mock.NewScripted([]mock.Step{rawRule(t, tt.wrong), {Kind: mock.Smart}}, nil)
			rule, err := brief.ResolveSegment(context.Background(), m, b, llm.Hooks{})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(conds(rule), "|"); rule.Status != segment.StatusOK || got != "days_since_last_order_or_never gt 30" {
				t.Fatalf("repaired rule = %s %v", rule.Status, got)
			}
			if n := len(m.Calls()); n != 2 {
				t.Fatalf("model calls = %d, want 2 (one repair round)", n)
			}
		})
	}
	// A rule that is already right passes first time.
	m := mock.NewScripted([]mock.Step{rawRule(t, okRule(pct(10), right))}, nil)
	if _, err := brief.ResolveSegment(context.Background(), m, b, llm.Hooks{}); err != nil || len(m.Calls()) != 1 {
		t.Fatalf("the right rule should pass in one call (err=%v, calls=%d)", err, len(m.Calls()))
	}
}

// A model that never fixes it must end in an explicit question, not in a
// quietly narrower or wider audience.
func TestUnfixableCoverageProblemBecomesAClarificationQuestion(t *testing.T) {
	wrong := okRule(pct(15), c("city", "eq", "Adana"), c("days_since_last_cart_add", "lte", int64(30)))
	m := mock.NewScripted([]mock.Step{rawRule(t, wrong)}, nil) // repeats forever
	rule, err := brief.ResolveSegment(context.Background(), m, reportedBrief, llm.Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Status != segment.StatusNeedsClarification || len(rule.Conditions) != 0 {
		t.Fatalf("want needs_clarification without conditions, got %+v", rule)
	}
	if !strings.Contains(trtext.Fold(rule.Question), "siparis") {
		t.Errorf("the question must name what could not be applied: %q", rule.Question)
	}
	if n := len(m.Calls()); n != 3 {
		t.Errorf("model calls = %d, want 3", n)
	}
}

// What the backend already knows it cannot express is decided in code. The
// model is not asked, so it cannot "helpfully" ignore the part it cannot do.
func TestUnsupportedAndAmbiguousBriefsNeverReachTheModel(t *testing.T) {
	for _, b := range []string{
		"35 yaş üstü sepeti terk eden müşterilere %10 indirim",
		"Bu ay sepete ürün ekleyip almayanlara %10 indirim",
	} {
		m := mock.NewScripted([]mock.Step{rawRule(t, okRule(none, c("city", "eq", "Adana")))}, nil)
		rule, err := brief.ResolveSegment(context.Background(), m, b, llm.Hooks{})
		if err != nil {
			t.Fatal(err)
		}
		if rule.Status == segment.StatusOK {
			t.Errorf("%q: the model's rule must not be accepted", b)
		}
		if n := len(m.Calls()); n != 0 {
			t.Errorf("%q: model was called %d times", b, n)
		}
	}
}
