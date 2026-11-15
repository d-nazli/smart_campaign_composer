//go:build live

// Smoke test against the REAL Claude API. Not part of `make test`; run it with
// `make test-live` (needs ANTHROPIC_API_KEY in .env). It costs a few cents.
package anthropic

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"smartcampaign/internal/config"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
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
