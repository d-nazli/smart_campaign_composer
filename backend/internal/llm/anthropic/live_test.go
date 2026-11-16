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
		parts = append(parts, fmt.Sprintf("%s %s %v", c.Field, c.Op, c.Value))
	}
	sort.Strings(parts)
	return strings.Join(parts, " | ")
}

// The whole pipeline (code reads the brief, the model answers, code verifies the
// answer) with the real model. Briefs that the code decides alone never reach the
// model; the others prove the prompt: terms go to the offer, never to the audience.
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
	}{
		{"the reported brief asks who, with the offer understood", mothersDay,
			segment.StatusNeedsClarification, "", offer.Offer{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"}, segment.SuggestAllCustomers},
		{"the reported brief after 'send to all'", mothersDay + "\n\nTüm müşterilere gönder.",
			segment.StatusOK, "all_customers eq true", offer.Offer{Kind: offer.KindAmount, Value: 100, MinSpend: 1000, ExpiresAt: "2026-12-31"}, ""},
		{"herkese %10 indirim", "herkese %10 indirim",
			segment.StatusOK, "all_customers eq true", offer.Offer{Kind: offer.KindPercent, Value: 10}, ""},
		{"tüm müşterilere, with a minimum and a last day", "Tüm müşterilere 1000 TL üzeri alışverişe %10 indirim, son kullanım 31.12.2026",
			segment.StatusOK, "all_customers eq true", offer.Offer{Kind: offer.KindPercent, Value: 10, MinSpend: 1000, ExpiresAt: "2026-12-31"}, ""},
		{"a minimum next to a real audience", "Son 30 günde sepeti terk edenlere 500 TL ve üzeri alışverişe ücretsiz kargo",
			segment.StatusOK, "days_since_last_cart_add lte 30 | purchased_since_last_cart_add eq false", offer.Offer{Kind: offer.KindFreeShipping, MinSpend: 500}, ""},
		{"nobody named", "%10 indirim gönder",
			segment.StatusNeedsClarification, "", offer.Offer{Kind: offer.KindPercent, Value: 10}, segment.SuggestAllCustomers},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := brief.ResolveSegment(ctx, c, tt.brief, llm.Hooks{OnRetry: func(n int, p []string) { t.Logf("retry %d: %v", n, p) }})
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(rule)
			t.Logf("rule: %s", b)
			if rule.Status != tt.status || rule.Suggestion != tt.suggestion {
				t.Fatalf("status %q suggestion %q, want %q / %q", rule.Status, rule.Suggestion, tt.status, tt.suggestion)
			}
			if tt.status == segment.StatusNeedsClarification {
				if rule.Question != brief.WhoQuestion {
					t.Errorf("question %q", rule.Question)
				}
			} else if got := liveConds(rule); got != tt.conds {
				t.Errorf("conditions %q, want %q", got, tt.conds)
			}
			if !rule.Offer.Equal(tt.want) {
				t.Errorf("offer %+v, want %+v", rule.Offer, tt.want)
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
