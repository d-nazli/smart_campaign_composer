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
	amountOf     = func(v float64) offer.Offer { return offer.Offer{Kind: offer.KindAmount, Value: v} }
	freePart     = offer.Part{Kind: offer.KindFreeShipping}
	// combo is a combined offer: the main part and the extras.
	combo = func(main offer.Offer, extras ...offer.Part) offer.Offer {
		main.Extras = extras
		return main
	}
	withExtras = func(o offer.Offer, extras ...offer.Part) offer.Offer { o.Extras = extras; return o }
	withCode   = func(o offer.Offer, code string) offer.Offer { o.Code = code; return o }
	// tier is "her step TL'ye value TL, toplamda en fazla max TL".
	tier = func(value, step, max float64) offer.Offer {
		return offer.Offer{Kind: offer.KindAmount, Value: value, StepSpend: step, MaxDiscount: max}
	}
	capped = func(o offer.Offer, max float64) offer.Offer { o.MaxDiscount = max; return o }
	// withTerms adds the terms of an offer: a minimum basket and a last day.
	withTerms = func(o offer.Offer, minSpend float64, expires string) offer.Offer {
		o.MinSpend, o.ExpiresAt = minSpend, expires
		return o
	}
)

const (
	cartWindow  = "days_since_last_cart_add lte "
	abandoned   = "purchased_since_last_cart_add eq false"
	noOrder     = "days_since_last_order_or_never gt "
	everyone    = "all_customers eq true"
	whoQuestion = "Bu kampanya kime gönderilsin? Tüm müşterilere mi?"
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
	// question is the exact text a needs_clarification answer must show, and
	// suggest the one-click answer offered with it (both optional).
	question string
	suggest  string
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
	{name: "offer: percent and free shipping go together", brief: "Sepeti terk edenlere %15 indirim ve ücretsiz kargo ver", status: segment.StatusOK, want: []string{abandoned}, offer: combo(pct(15), freePart)},
	{name: "offer: two different percentages", brief: "Sepeti terk edenlere %15 indirim, VIP olanlara %20 indirim ver", status: segment.StatusNeedsClarification, say: "teklif"},
	{name: "offer: impossible percentage", brief: "Sepeti terk edenlere %150 indirim ver", status: segment.StatusNeedsClarification, say: "150"},
	{name: "offer: 'kod' without a code", brief: "Sepeti terk edenlere indirim kodu gönder", status: segment.StatusNeedsClarification, say: "kod"},

	// ---- things we cannot do: must be said out loud ----------------------
	{name: "unsupported: age", brief: "35 yaş üstü sepeti terk eden müşterilere %10 indirim", status: segment.StatusUnsupported, say: "yaş"},
	{name: "unsupported: age, other wording", brief: "30 yaşındaki müşterilere kampanya", status: segment.StatusUnsupported, say: "yaş"},
	{name: "unsupported: product category", brief: "Ayakkabı kategorisinde sepete ürün ekleyenlere %10 indirim", status: segment.StatusUnsupported, say: "kategori"},
	{name: "unsupported: product, no word 'category'", brief: "Sepetine telefon ekleyip almayanlara %10 indirim", status: segment.StatusUnsupported, say: "telefon"},
	{name: "unsupported: birthday", brief: "Doğum günü bu hafta olanlara %10 indirim", status: segment.StatusUnsupported, say: "doğum"},

	// ---- not understood: ask, do not guess -------------------------------

	// ---- offer terms (minimum basket, last day) are NOT audience conditions ---
	// The brief that was reported: "1000 TL üzeri" is a term of the discount, and the
	// brief names no audience, so the only open question is who gets the message.
	{name: "mother's day: terms are read, the special day means everyone", brief: mothersDay,
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(amountOf(100), 1000, "2026-12-31")},
	{name: "mother's day, after 'send to everyone'", brief: mothersDay + "\n\nTüm müşterilere gönder.",
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(amountOf(100), 1000, "2026-12-31")},
	{name: "terms: minimum basket with a percentage", brief: "Tüm müşterilere 1000 TL üzeri alışverişe %10 indirim",
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(pct(10), 1000, "")},
	{name: "terms: 1.500 TL ve üzeri, slash date", brief: "Herkese 1.500 TL ve üzeri alışverişlerde 150 TL indirim, son kullanım 31/12/2026",
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(amountOf(150), 1500, "2026-12-31")},
	{name: "terms: en az ...'lik alışveriş", brief: "Herkese en az 750 TL'lik alışverişe ücretsiz kargo",
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(freeShipping, 750, "")},
	{name: "terms: 'sepete' counts as a shopping word", brief: "Herkese 500 TL üzeri sepete %10 indirim",
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(pct(10), 500, "")},
	{name: "terms: written-out date", brief: "Herkese %10 indirim, son kullanma tarihi 5 Ocak 2027",
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(pct(10), 0, "2027-01-05")},
	{name: "terms: 'kadar geçerli'", brief: "Herkese 31 Aralık 2026'ya kadar geçerli %10 indirim",
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(pct(10), 0, "2026-12-31")},
	{name: "terms with a cart audience", brief: "Son 30 günde sepeti terk edenlere 1000 TL üzeri alışverişe %10 indirim, son kullanım 31.12.2026",
		status: segment.StatusOK, want: []string{cartWindow + "30", abandoned}, offer: withTerms(pct(10), 1000, "2026-12-31")},
	{name: "terms: a minimum without an offer", brief: "Herkese 1000 TL üzeri alışverişlerde kampanya", status: segment.StatusNeedsClarification, say: "teklif"},
	{name: "terms: a date without an offer", brief: "Herkese son kullanım 31.12.2026", status: segment.StatusNeedsClarification, say: "teklif"},
	{name: "terms: impossible date", brief: "Herkese %10 indirim, son kullanım 31.02.2026", status: segment.StatusNeedsClarification, say: "geçerli bir tarih"},
	{name: "terms: day and month without a year", brief: "Herkese %10 indirim, 31 Aralık'a kadar", status: segment.StatusNeedsClarification, say: "31 Aralık 20"},
	{name: "terms: two different dates", brief: "Herkese %10 indirim, son kullanım 31.12.2026 ya da 15.01.2027", status: segment.StatusNeedsClarification, say: "birden fazla tarih"},
	{name: "terms: a date that is not said to be the last day", brief: "Herkese %10 indirim 31.12.2026", status: segment.StatusNeedsClarification, say: "son kullanım 31.12.2026"},
	{name: "terms: two different minimums", brief: "Herkese 500 TL üzeri alışverişe %10 indirim, 1000 TL üzeri alışverişe %15 indirim", status: segment.StatusNeedsClarification, say: "birden fazla"},

	// ---- everyone: only when the brief says so, never an empty rule ----------
	{name: "everyone: herkese %10 indirim", brief: "herkese %10 indirim", status: segment.StatusOK, want: []string{everyone}, offer: pct(10)},
	{name: "everyone: tüm müşterilere", brief: "Tüm müşterilere %10 indirim gönder", status: segment.StatusOK, want: []string{everyone}, offer: pct(10)},
	{name: "everyone: bütün kullanıcılara", brief: "Bütün kullanıcılara ücretsiz kargo", status: segment.StatusOK, want: []string{everyone}, offer: freeShipping},
	{name: "everyone: no offer", brief: "Herkese merhaba de", status: segment.StatusOK, want: []string{everyone}, offer: none},
	{name: "everyone is ignored when a real audience is named", brief: "İstanbul'daki herkese %10 indirim", status: segment.StatusOK, want: []string{"city eq İstanbul"}, offer: pct(10)},
	{name: "everyone is ignored next to a cart audience", brief: "Sepeti terk eden herkese %10 indirim", status: segment.StatusOK, want: []string{abandoned}, offer: pct(10)},
	{name: "nobody named: a discount only", brief: "%10 indirim gönder", status: segment.StatusNeedsClarification,
		question: whoQuestion, suggest: segment.SuggestAllCustomers, offer: pct(10)},
	{name: "nobody named: a greeting only", brief: "Hoş geldin mesajı gönder", status: segment.StatusNeedsClarification,
		question: whoQuestion, suggest: segment.SuggestAllCustomers, offer: none},

	// ---- combined offers: different kinds go together, the same kind twice is asked ----
	{name: "combined: order does not matter", brief: "Herkese ücretsiz kargo ve %20 indirim", status: segment.StatusOK, want: []string{everyone}, offer: combo(pct(20), freePart)},
	{name: "combined: written with +", brief: "Herkese %20 indirim + ücretsiz kargo", status: segment.StatusOK, want: []string{everyone}, offer: combo(pct(20), freePart)},
	{name: "combined: three parts", brief: "Tüm müşterilere %10 indirim, 50 TL hediye çeki ve ücretsiz kargo", status: segment.StatusOK, want: []string{everyone},
		offer: combo(pct(10), offer.Part{Kind: offer.KindAmount, Value: 50}, freePart)},
	{name: "combined: with a code and terms", brief: "Herkese 500 TL üzeri alışverişe %15 indirim ve ücretsiz kargo, kupon kodu YAZ15, son kullanım 31.12.2026",
		status: segment.StatusOK, want: []string{everyone}, offer: withCode(withTerms(combo(pct(15), freePart), 500, "2026-12-31"), "YAZ15")},
	{name: "combined: a choice is asked", brief: "Herkese %20 indirim ya da ücretsiz kargo", status: segment.StatusNeedsClarification, say: "ya da"},
	{name: "combined: two percentages are asked", brief: "Herkese %20 ya da %30 indirim", status: segment.StatusNeedsClarification, say: "aynı türden"},
	{name: "combined: two amounts are asked", brief: "Herkese 100 TL indirim ve 50 TL indirim", status: segment.StatusNeedsClarification, say: "aynı türden"},

	// ---- tiered discounts and caps ----------------------------------------------
	{name: "tier: the reported brief", brief: "Her 1000 TL'lik alışverişe 100 TL indirim, toplamda en fazla 300 TL. Tüm müşterilere.",
		status: segment.StatusOK, want: []string{everyone}, offer: tier(100, 1000, 300)},
	{name: "tier: 'TL'ye' without a shopping word, no cap", brief: "Herkese her 500 TL'ye 50 TL indirim", status: segment.StatusOK, want: []string{everyone}, offer: tier(50, 500, 0)},
	{name: "tier: 'alışverişte' and 'en çok'", brief: "Sepeti terk edenlere her 1.000 TL alışverişte 75 TL indirim, en çok 225 TL", status: segment.StatusOK,
		want: []string{abandoned}, offer: tier(75, 1000, 225)},
	{name: "tier with free shipping", brief: "Herkese her 1000 TL'lik alışverişe 100 TL indirim ve ücretsiz kargo, toplamda en fazla 300 TL", status: segment.StatusOK,
		want: []string{everyone}, offer: withExtras(tier(100, 1000, 300), freePart)},
	{name: "cap on a percentage", brief: "Herkese %20 indirim, en fazla 300 TL", status: segment.StatusOK, want: []string{everyone}, offer: capped(pct(20), 300)},
	{name: "cap on a percentage: '... TL'ye kadar'", brief: "Herkese 250 TL'ye kadar %20 indirim", status: segment.StatusOK, want: []string{everyone}, offer: capped(pct(20), 250)},
	// "N TL'ye varan sipariş" is a basket that reaches N TL: the minimum (D88).
	{name: "varan: the reported brief, 'tl ye' written apart", brief: "henüz hiç sipariş vermemiş kullanıcılara ilk siparişine özel 1000 tl ye varan siparişte 300 tl indirim uygula",
		status: segment.StatusOK, want: []string{"order_count eq 0"}, offer: withTerms(amountOf(300), 1000, "")},
	{name: "varan: 'TL'ye varan alışverişte' with a percentage", brief: "Herkese 500 TL'ye varan alışverişlerde %10 indirim",
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(pct(10), 500, "")},
	{name: "varan: 'ulaşan sepete'", brief: "Herkese 750 TL'ye ulaşan sepete ücretsiz kargo", status: segment.StatusOK, want: []string{everyone}, offer: withTerms(freeShipping, 750, "")},
	{name: "varan: '... varan indirim' stays a cap", brief: "Herkese 300 TL'ye varan %20 indirim", status: segment.StatusOK, want: []string{everyone}, offer: capped(pct(20), 300)},
	{name: "varan: 'kadar alışverişte' is an upper basket limit and is asked", brief: "Herkese 1000 TL'ye kadar alışverişte %10 indirim",
		status: segment.StatusNeedsClarification, say: "üst sınırı"},
	{name: "cap without anything to cap", brief: "Herkese en fazla 300 TL indirim", status: segment.StatusNeedsClarification, say: "üst sınır"},
	{name: "tier together with a percentage", brief: "Herkese %10 indirim ve her 1000 TL'ye 100 TL indirim", status: segment.StatusNeedsClarification, say: "kademeli"},
	{name: "tier: discount not smaller than the step", brief: "Herkese her 100 TL'ye 100 TL indirim", status: segment.StatusNeedsClarification, say: "step_spend"},

	// ---- special days: the theme, never the audience ----------------------------
	{name: "special day: Kadınlar Günü, nobody named -> everyone", brief: "Kadınlar Günü'ne özel %20 indirim", status: segment.StatusOK, want: []string{everyone}, offer: pct(20)},
	{name: "special day: 8 Mart", brief: "8 Mart'a özel %20 indirim kampanyası", status: segment.StatusOK, want: []string{everyone}, offer: pct(20)},
	{name: "special day: 8 Mart with a year is a last day, not a special day", brief: "Herkese 8 Mart 2027'ye kadar geçerli %20 indirim",
		status: segment.StatusOK, want: []string{everyone}, offer: withTerms(pct(20), 0, "2027-03-08")},
	{name: "special day + explicit gender", brief: "Kadınlar Günü'ne özel, kadın müşterilere %20 indirim", status: segment.StatusOK, want: []string{"gender eq female"}, offer: pct(20)},
	{name: "special day: Anneler Günü for women, explicitly", brief: "Anneler Günü için kadın müşterilere özel %15 indirim", status: segment.StatusOK, want: []string{"gender eq female"}, offer: pct(15)},
	{name: "special day: Anneler Günü alone is everyone", brief: "Anneler Günü'ne özel ücretsiz kargo", status: segment.StatusOK, want: []string{everyone}, offer: freeShipping},
	{name: "special day: Babalar Günü for men, explicitly", brief: "Babalar Günü'ne özel erkek müşterilerimize %10 indirim", status: segment.StatusOK, want: []string{"gender eq male"}, offer: pct(10)},
	{name: "special day: Babalar Günü with everyone said", brief: "Babalar Günü'ne özel ücretsiz kargo, tüm müşterilere", status: segment.StatusOK, want: []string{everyone}, offer: freeShipping},
	{name: "special day with a real audience: no gender, no default", brief: "Kadınlar Günü'nde sepeti terk eden İstanbul'daki müşterilere %10 indirim",
		status: segment.StatusOK, want: []string{"city eq İstanbul", abandoned}, offer: pct(10)},
	{name: "special day: Sevgililer Günü / 14 Şubat", brief: "14 Şubat Sevgililer Günü'ne özel %25 indirim", status: segment.StatusOK, want: []string{everyone}, offer: pct(25)},
	{name: "special day: Yılbaşı", brief: "Yılbaşına özel tüm ürünlerde %30 indirim", status: segment.StatusOK, want: []string{everyone}, offer: pct(30)},
	{name: "special day: Black Friday", brief: "Black Friday'e özel %40 indirim", status: segment.StatusOK, want: []string{everyone}, offer: pct(40)},
	{name: "special day: Ramazan Bayramı", brief: "Ramazan Bayramı'na özel 1000 TL üzeri alışverişe ücretsiz kargo", status: segment.StatusOK, want: []string{everyone}, offer: withTerms(freeShipping, 1000, "")},
	{name: "special day: Öğretmenler Günü", brief: "Öğretmenler Günü'ne özel 100 TL indirim", status: segment.StatusOK, want: []string{everyone}, offer: amountOf(100)},
	{name: "special day: 23 Nisan", brief: "23 Nisan Çocuk Bayramı'na özel %15 indirim", status: segment.StatusOK, want: []string{everyone}, offer: pct(15)},

	// ---- gender: only when the brief says so ------------------------------------
	{name: "gender: kadın müşterilere", brief: "Sepeti terk eden kadın müşterilere %10 indirim", status: segment.StatusOK, want: []string{"gender eq female", abandoned}, offer: pct(10)},
	{name: "gender: kadınlara", brief: "Ankara'daki kadınlara %10 indirim", status: segment.StatusOK, want: []string{"city eq Ankara", "gender eq female"}, offer: pct(10)},
	{name: "gender: cinsiyeti erkek olan", brief: "Cinsiyeti erkek olan müşterilere kampanya", status: segment.StatusOK, want: []string{"gender eq male"}, offer: none},
	{name: "gender: both genders is everyone", brief: "Kadın ve erkek müşterilerimize %10 indirim", status: segment.StatusOK, want: []string{everyone}, offer: pct(10)},
	{name: "gender: a gender word that is not the audience changes nothing", brief: "Kadın koleksiyonunda %20 indirim", status: segment.StatusNeedsClarification,
		question: whoQuestion, suggest: segment.SuggestAllCustomers, offer: pct(20)},

	// ---- an amount threshold: ambiguous, and the example uses YOUR number ----
	{name: "threshold: spending customers, example uses the brief's number", brief: "1500 TL üzeri alışveriş yapan müşterilere %10 indirim",
		status: segment.StatusNeedsClarification, say: "toplam harcaması 1500 TL'den fazla"},
	{name: "spend audience: toplam harcaması N TL üzeri", brief: "Toplam harcaması 2500 TL üzeri olan müşterilere %10 indirim",
		status: segment.StatusOK, want: []string{"total_spent gt 2500"}, offer: pct(10)},
	{name: "spend audience: 'den fazla harcayan", brief: "1.000 TL'den fazla harcayan müşterilere ücretsiz kargo",
		status: segment.StatusOK, want: []string{"total_spent gt 1000"}, offer: freeShipping},
	{name: "spend audience: ve üzeri", brief: "Toplam harcaması 5000 TL ve üzeri olanlara 200 TL indirim",
		status: segment.StatusOK, want: []string{"total_spent gte 5000"}, offer: amountOf(200)},
	{name: "spend audience: under a limit", brief: "Toplam harcaması 300 TL altında olan müşterilere %5 indirim",
		status: segment.StatusOK, want: []string{"total_spent lt 300"}, offer: pct(5)},
	{name: "spend audience with a city", brief: "Ankara'daki toplam harcaması 2500 TL üzeri müşterilere %10 indirim",
		status: segment.StatusOK, want: []string{"city eq Ankara", "total_spent gt 2500"}, offer: pct(10)},
	{name: "spend audience: words but no amount", brief: "Çok harcayan müşterilere %10 indirim", status: segment.StatusNeedsClarification, say: "tutar"},
	{name: "threshold: under a limit", brief: "750 TL altında alışveriş yapanlara %10 indirim", status: segment.StatusNeedsClarification, say: "750"},
	{name: "hours: the example converts the brief's number", brief: "Son 48 saatte sepete ürün ekleyip almayanlara %10 indirim",
		status: segment.StatusNeedsClarification, say: "son 2 gün"},
	{name: "vague window with a number in it", brief: "Son 45+ gün sepete ürün ekleyip almayanlara %10 indirim",
		status: segment.StatusNeedsClarification, say: "son 45 gün"},
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
				if tt.say != "" && !strings.Contains(shown, trtext.Fold(tt.say)) {
					t.Errorf("the message shown to the user must name %q, got question=%q explanation=%q", tt.say, rule.Question, rule.Explanation)
				}
				if tt.question != "" && rule.Question != tt.question {
					t.Errorf("question = %q, want exactly %q", rule.Question, tt.question)
				}
				if rule.Suggestion != tt.suggest {
					t.Errorf("suggestion = %q, want %q", rule.Suggestion, tt.suggest)
				}
				// When the only open question is "who?", the offer that WAS understood is
				// carried along, so the screen can show it next to the question.
				if tt.suggest != "" && !rule.Offer.Equal(tt.offer) {
					t.Errorf("offer on the clarification = %+v, want %+v", rule.Offer, tt.offer)
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

const mothersDay = "anneler gününe özel 1000 tl üzeri alışverişe 100tl indirim kullandır son kullanım 31.12.2026"

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

// ---- terms and "everyone": what a model must not get wrong ---------------------

func feedback(t *testing.T, m *mock.Client) string {
	t.Helper()
	calls := m.Calls()
	if len(calls) != 2 || len(calls[1].Previous) != 1 {
		t.Fatalf("expected exactly one repair round, got %d calls", len(calls))
	}
	return trtext.Fold(strings.Join(calls[1].Previous[0].Problems, " "))
}

// The slip seen with a real model: "1000 TL üzeri" became a total_spent audience
// condition. The threshold belongs to the offer.
func TestAThresholdIsNeverAcceptedAsAnAudienceCondition(t *testing.T) {
	const b = "Tüm müşterilere 1000 TL üzeri alışverişe 100 TL indirim, son kullanım 31.12.2026"
	right := withTerms(amountOf(100), 1000, "2026-12-31")
	everyoneCond := c("all_customers", "eq", true)
	tests := []struct {
		name  string
		wrong segment.Rule
		say   []string // words the repair feedback must contain
	}{
		{"threshold became total_spent", okRule(right, c("total_spent", "gt", 1000.0)), []string{"min_spend", "total_spent"}},
		{"threshold became total_spent and the audience is missing", okRule(amountOf(100), c("total_spent", "gt", 1000.0)), []string{"all_customers", "min_spend", "expires_at"}},
		{"terms left out of the offer", okRule(amountOf(100), everyoneCond), []string{"min_spend", "expires_at"}},
		{"wrong minimum", okRule(withTerms(amountOf(100), 500, "2026-12-31"), everyoneCond), []string{"min_spend"}},
		{"wrong last day", okRule(withTerms(amountOf(100), 1000, "2026-12-30"), everyoneCond), []string{"expires_at"}},
		{"everyone plus a threshold condition", okRule(right, everyoneCond, c("total_spent", "gt", 1000.0)), []string{"all_customers"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mock.NewScripted([]mock.Step{rawRule(t, tt.wrong), {Kind: mock.Smart}}, nil)
			rule, err := brief.ResolveSegment(context.Background(), m, b, llm.Hooks{})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(conds(rule), "|"); rule.Status != segment.StatusOK || got != everyone || !rule.Offer.Equal(right) {
				t.Fatalf("repaired rule: %s %v %+v", rule.Status, got, rule.Offer)
			}
			fb := feedback(t, m)
			for _, w := range tt.say {
				if !strings.Contains(fb, w) {
					t.Errorf("the repair feedback should mention %q: %s", w, fb)
				}
			}
		})
	}
}

func TestTheModelCannotInventOrForgetEveryone(t *testing.T) {
	invented := okRule(pct(10), c("all_customers", "eq", true))
	m := mock.NewScripted([]mock.Step{rawRule(t, invented), {Kind: mock.Smart}}, nil)
	rule, err := brief.ResolveSegment(context.Background(), m, "İstanbul'daki müşterilere %10 indirim", llm.Hooks{})
	if err != nil || strings.Join(conds(rule), "|") != "city eq İstanbul" {
		t.Fatalf("%v %v", conds(rule), err)
	}
	if fb := feedback(t, m); !strings.Contains(fb, "all_customers") {
		t.Errorf("feedback: %s", fb)
	}

	forgot := okRule(pct(10), c("city", "eq", "İstanbul"))
	m = mock.NewScripted([]mock.Step{rawRule(t, forgot), {Kind: mock.Smart}}, nil)
	rule, err = brief.ResolveSegment(context.Background(), m, "Herkese %10 indirim", llm.Hooks{})
	if err != nil || strings.Join(conds(rule), "|") != everyone {
		t.Fatalf("%v %v", conds(rule), err)
	}
	if fb := feedback(t, m); !strings.Contains(fb, "all_customers") {
		t.Errorf("feedback: %s", fb)
	}
}

// When no audience is named at all the model is not asked: it would guess.
func TestABriefThatNamesNobodyNeverReachesTheModel(t *testing.T) {
	for _, b := range []string{"%10 indirim gönder", "Hoş geldin mesajı gönder", "Kadın koleksiyonunda %20 indirim"} {
		m := mock.NewScripted([]mock.Step{rawRule(t, okRule(pct(10), c("all_customers", "eq", true)))}, nil)
		rule, err := brief.ResolveSegment(context.Background(), m, b, llm.Hooks{})
		if err != nil {
			t.Fatal(err)
		}
		if rule.Status != segment.StatusNeedsClarification || rule.Question != whoQuestion || rule.Suggestion != segment.SuggestAllCustomers {
			t.Errorf("%q: %+v", b, rule)
		}
		if n := len(m.Calls()); n != 0 {
			t.Errorf("%q: the model was called %d times", b, n)
		}
	}
}

// A spending audience the model CAN express keeps working: it is not "nobody".
func TestASpendingAudienceStillGoesToTheModel(t *testing.T) {
	ok := okRule(pct(10), c("total_spent", "gt", 5000.0))
	m := mock.NewScripted([]mock.Step{rawRule(t, ok)}, nil)
	rule, err := brief.ResolveSegment(context.Background(), m, "Toplam harcaması 5000 TL'den fazla olan müşterilere %10 indirim", llm.Hooks{})
	if err != nil || rule.Status != segment.StatusOK || strings.Join(conds(rule), "|") != "total_spent gt 5000" {
		t.Fatalf("%+v %v", rule, err)
	}
	if n := len(m.Calls()); n != 1 {
		t.Fatalf("calls = %d", n)
	}
}
