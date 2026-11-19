//go:build live

// Smoke test against the REAL Claude API. Not part of `make test`; run it with
// `make test-live` (needs ANTHROPIC_API_KEY in .env). It costs a few cents.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/config"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/segment"
)

const liveBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

func liveClient(t *testing.T) *Client {
	t.Helper()
	key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if key == "" {
		t.Fatal("ANTHROPIC_API_KEY is empty: put a key in .env before running `make test-live`")
	}
	model := os.Getenv("CLAUDE_MODEL")
	if model == "" {
		model = config.DefaultModel
	}
	return New(key, model)
}

func TestLiveSegmentForExampleBrief(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	rule, err := llm.Repair(ctx, 3, func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
		return c.Segment(ctx, llm.SegmentRequest{Brief: liveBrief, Previous: prev})
	}, func(raw string) (segment.Rule, []string) {
		r, err := segment.Parse(raw)
		if pe, ok := err.(*segment.ParseError); ok {
			return r, pe.Problems
		} else if err != nil {
			return r, []string{err.Error()}
		}
		return r, nil
	}, llm.Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(rule)
	t.Logf("rule: %s", b)
	if rule.Status != segment.StatusOK {
		t.Fatalf("status = %s", rule.Status)
	}
	hasCity, hasCart := false, false
	for _, cnd := range rule.Conditions {
		hasCity = hasCity || (cnd.Field == "city" && cnd.Value == "İstanbul")
		hasCart = hasCart || cnd.Field == "days_since_last_cart_add"
	}
	if !hasCity || !hasCart {
		t.Fatalf("rule misses the city or the cart window: %s", b)
	}
}

func TestLiveMessagesForExampleBrief(t *testing.T) {
	c := liveClient(t)
	pol, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	streamed := 0
	got, err := llm.Repair(ctx, 3, func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
		return c.Messages(ctx, llm.MessageRequest{Brief: liveBrief, Explanation: "İstanbul'da sepeti terk edenler", Policy: pol, Previous: prev},
			func(string) { streamed++ })
	}, func(raw string) (content.Content, []string) {
		var ct content.Content
		if err := json.Unmarshal([]byte(raw), &ct); err != nil {
			return ct, []string{"geçersiz JSON: " + err.Error()}
		}
		norm, rep := pol.Validate(ct)
		if !rep.OK {
			return ct, rep.Messages()
		}
		return norm, nil
	}, llm.Hooks{OnRetry: func(n int, p []string) { t.Logf("retry %d: %v", n, p) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sms (%d): %s\nsubject: %s\nbody: %s", pol.EffectiveLength(got.SMSBody), got.SMSBody, got.EmailSubject, got.EmailBody)
	if streamed < 2 {
		t.Errorf("expected a streamed response, got %d text fragments", streamed)
	}
}

// ---- terms of the offer and "everyone", against the real model -----------------

const mothersDay = "anneler gününe özel 1000 tl üzeri alışverişe 100tl indirim kullandır son kullanım 31.12.2026"

// liveCondString writes the conditions in a stable, comparable form.
func liveConds(r segment.Rule) string {
	var parts []string
	for _, c := range r.Conditions {
		v := c.Value
		if list, ok := v.([]string); ok {
			list = append([]string(nil), list...)
			sort.Strings(list)
			v = list
		}
		parts = append(parts, fmt.Sprintf("%s %s %v", c.Field, c.Op, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, " | ")
}

// The production pipeline of Claude mode (D113): Claude reads the brief, code
// checks the answer technically. These cases prove the prompt: terms go to the
// offer, never to the audience; a special day alone means everyone, with the
// model's note; nobody named means the fixed "who?" question.
func TestLiveBriefsWithTermsAndEveryone(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tests := []struct {
		name, brief string
		status      string
		conds       string // exact, sorted
		want        offer.Offer
		suggestion  string
		note        bool // the model must explain that everyone was chosen
	}{
		{"a special day without an audience means everyone", mothersDay,
			segment.StatusOK, "all_customers eq true", offer.Offer{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"}, "", true},
		{"herkese %10 indirim", "herkese %10 indirim",
			segment.StatusOK, "all_customers eq true", offer.Offer{Kind: offer.KindPercent, Value: 10}, "", false},
		{"tüm müşterilere, with a minimum and a last day", "Tüm müşterilere 1000 TL üzeri alışverişe %10 indirim, son kullanım 31.12.2026",
			segment.StatusOK, "all_customers eq true", offer.Offer{Kind: offer.KindPercent, Value: 10, MinSpend: 1000, ExpiresAt: "2026-12-31"}, "", false},
		{"a minimum next to a real audience", "Son 30 günde sepeti terk edenlere 500 TL ve üzeri alışverişe ücretsiz kargo",
			segment.StatusOK, "days_since_last_cart_add lte 30 | purchased_since_last_cart_add eq false", offer.Offer{Kind: offer.KindFreeShipping, MinSpend: 500}, "", false},
		{"nobody named", "%10 indirim gönder",
			segment.StatusNeedsClarification, "", offer.Offer{Kind: offer.KindNone}, segment.SuggestAllCustomers, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := liveResolve(ctx, t, c, tt.brief)
			if rule.Status != tt.status || rule.Suggestion != tt.suggestion {
				t.Fatalf("status %q suggestion %q, want %q / %q", rule.Status, rule.Suggestion, tt.status, tt.suggestion)
			}
			if tt.status == segment.StatusNeedsClarification {
				if rule.Question != brief.WhoQuestion {
					t.Errorf("question %q", rule.Question)
				}
				return
			}
			if got := liveConds(rule); got != tt.conds {
				t.Errorf("conditions %q, want %q", got, tt.conds)
			}
			if !rule.Offer.Equal(tt.want) {
				t.Errorf("offer %+v, want %+v", rule.Offer, tt.want)
			}
			if tt.note && rule.Note == "" {
				t.Errorf("the model should say why everyone was chosen")
			}
		})
	}
}

func liveResolve(ctx context.Context, t *testing.T, c *Client, text string) segment.Rule {
	t.Helper()
	rule, err := brief.ResolveSegmentWith(ctx, c, text, llm.Hooks{OnRetry: func(n int, p []string) { t.Logf("retry %d: %v", n, p) }}, brief.ModelDriven)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(rule)
	t.Logf("rule: %s", b)
	return rule
}

// Turkish, English and complex briefs that say everything needed: Claude
// alone turns them into exactly these conditions and this offer.
func TestLiveModelDrivenClearBriefs(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	tests := []struct {
		name, brief, conds string
		want               offer.Offer
		alt                string // an equivalent way to write conds ("gte 1" for "gt 0")
	}{
		{"Turkish: the case's example", liveBrief,
			"city eq İstanbul | days_since_last_cart_add lte 30 | purchased_since_last_cart_add eq false", offer.Offer{Kind: offer.KindPercent, Value: 30}, ""},
		{"English: abandoners in a city", "Send a 15% discount to customers in Izmir who added something to their cart in the last 7 days but didn't buy it.",
			"city eq İzmir | days_since_last_cart_add lte 7 | purchased_since_last_cart_add eq false", offer.Offer{Kind: offer.KindPercent, Value: 15}, ""},
		{"English: lapsed buyers, a city excluded", "Win back everyone who hasn't ordered in the last 90 days, except customers in Ankara. Offer free shipping.",
			"city neq Ankara | days_since_last_order_or_never gt 90", offer.Offer{Kind: offer.KindFreeShipping}, ""},
		{"complex: tier, cities, gender, spending", "İzmir ve Ankara'da yaşayan, toplam harcaması 5000 TL'den fazla olan kadın müşterilere her 1000 TL'lik alışverişe 100 TL indirim, toplamda en fazla 300 TL. Son kullanım 31.12.2026.",
			"city in [Ankara İzmir] | gender eq female | total_spent gt 5000", offer.Offer{Kind: offer.KindAmount, Value: 100, StepSpend: 1000, MaxDiscount: 300, ExpiresAt: "2026-12-31"}, ""},
		{"complex: two weeks, never ordered excluded", "Son 2 haftada sepete ürün ekleyip satın almayan ve daha önce en az bir sipariş vermiş müşterilere %20 indirim + ücretsiz kargo",
			"days_since_last_cart_add lte 14 | order_count gt 0 | purchased_since_last_cart_add eq false",
			offer.Offer{Kind: offer.KindPercent, Value: 20, Extras: []offer.Part{{Kind: offer.KindFreeShipping}}},
			"days_since_last_cart_add lte 14 | order_count gte 1 | purchased_since_last_cart_add eq false"},
		{"age: an explicit range in a city", "18-25 yaş arası İstanbul'daki kullanıcılar",
			"age gte 18 | age lte 25 | city eq İstanbul", offer.Offer{Kind: offer.KindNone}, ""},
		{"scope: products the offer is valid on are not an audience", "Kış ürünlerinde geçerli 200 TL indirim",
			"all_customers eq true", offer.Offer{Kind: offer.KindAmount, Value: 200, Scope: "kış ürünleri"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := liveResolve(ctx, t, c, tt.brief)
			if rule.Status != segment.StatusOK {
				t.Fatalf("status %q: %s %s", rule.Status, rule.Question, rule.Explanation)
			}
			if got := liveConds(rule); got != tt.conds && (tt.alt == "" || got != tt.alt) {
				t.Errorf("conditions %q, want %q", got, tt.conds)
			}
			if !rule.Offer.Equal(tt.want) {
				t.Errorf("offer %+v, want %+v", rule.Offer, tt.want)
			}
			if strings.TrimSpace(rule.Explanation) == "" {
				t.Error("the explanation is the summary the marketer reads")
			}
		})
	}
}

// Briefs Claude cannot turn into a rule without guessing: it asks (or says
// what it cannot serve) instead of inventing a condition. No code reads these
// briefs; the answer is Claude's alone.
func TestLiveModelDrivenAmbiguousBriefsAreAskedAbout(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	tests := []struct {
		name, brief string
		statuses    []string
	}{
		{"a label without a threshold", "VIP müşterilere %10 indirim gönder", []string{segment.StatusNeedsClarification}},
		{"English: a vague time window", "Send 10% off to people who recently added items to their cart", []string{segment.StatusNeedsClarification}},
		{"an amount whose purpose is unclear", "İstanbul'daki müşterilere 2000 TL 250 TL indirim", []string{segment.StatusNeedsClarification}},
		{"two different discounts", "Sepeti terk edenlere %15 ya da %20 indirim", []string{segment.StatusNeedsClarification}},
		{"data the database does not have", "35 yaş üstü ve son 30 günde telefon kategorisine bakan müşterilere %10 indirim",
			[]string{segment.StatusUnsupported, segment.StatusNeedsClarification}},
		{"age: a vague age group is asked about", "gençlere özel indirim", []string{segment.StatusNeedsClarification}},
		{"scope: people chosen by products they bought", "Kış ürünü almış kişilere %10 indirim", []string{segment.StatusUnsupported}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := liveResolve(ctx, t, c, tt.brief)
			ok := false
			for _, s := range tt.statuses {
				ok = ok || rule.Status == s
			}
			if !ok || len(rule.Conditions) != 0 {
				t.Fatalf("want one of %v without conditions, got %q %v", tt.statuses, rule.Status, liveConds(rule))
			}
			if rule.Status == segment.StatusNeedsClarification && strings.TrimSpace(rule.Question) == "" {
				t.Error("a clarification needs a question")
			}
			if strings.HasPrefix(tt.name, "age:") && !strings.Contains(strings.ToLower(rule.Question), "yaş") {
				t.Errorf("the question must ask about the age range: %q", rule.Question)
			}
		})
	}
}

// The real model writes messages that state the minimum and the last day, in the
// SMS and in the email, within the limits.
func TestLiveMessagesStateTheTermsOfTheOffer(t *testing.T) {
	c := liveClient(t)
	pol, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	o := offer.Offer{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	got, err := llm.Repair(ctx, 3, func(ctx context.Context, prev []llm.Attempt, _ int) (llm.Output, error) {
		return c.Messages(ctx, llm.MessageRequest{Brief: mothersDay, Explanation: "Tüm müşteriler", Offer: o, Policy: pol, Previous: prev}, nil)
	}, func(raw string) (content.Content, []string) {
		var ct content.Content
		if err := json.Unmarshal([]byte(raw), &ct); err != nil {
			return ct, []string{"geçersiz JSON: " + err.Error()}
		}
		norm, rep := pol.Validate(ct)
		if !rep.OK {
			return ct, rep.Messages()
		}
		var problems []string
		for _, q := range pol.Quality(norm, o) {
			problems = append(problems, q.Message)
		}
		return norm, problems
	}, llm.Hooks{OnRetry: func(n int, p []string) { t.Logf("retry %d: %v", n, p) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sms: %s\nsubject: %s\nbody: %s", got.SMSBody, got.EmailSubject, got.EmailBody)
	if !o.Mentions(got.SMSBody) || !o.Mentions(got.EmailSubject+"\n"+got.EmailBody) {
		t.Fatalf("the messages must state the offer with its minimum and last day: %+v", got)
	}
}
