package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"smartcampaign/internal/clock"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// quietConfig has every chaos knob off; tests switch on exactly what they need
// and always with a rate of 0 or 1, never a fractional probability.
func quietConfig() Config {
	return Config{
		RetryAfterMin:      1,
		RetryAfterMax:      3,
		RateLimitPerSecond: 100,
		TimeoutDelay:       10 * time.Second,
		Seed:               1,
	}
}

type harness struct {
	srv  *Server
	h    http.Handler
	clk  *clock.Fake
	logs *bytes.Buffer // read only after all requests have finished
}

func newHarness(t *testing.T, mutate func(*Config)) *harness {
	t.Helper()
	cfg := quietConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	clk := clock.NewFake(t0)
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewJSONHandler(&lockedWriter{w: buf}, nil))
	srv := New(cfg, clk, log)
	return &harness{srv: srv, h: srv.Handler(), clk: clk, logs: buf}
}

type lockedWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (hn *harness) sms(key, to, body string) *httptest.ResponseRecorder {
	return hn.do("/v1/sms/send", key, fmt.Sprintf(`{"to":%q,"body":%q}`, to, body))
}

func (hn *harness) email(key, to, subject, body string) *httptest.ResponseRecorder {
	return hn.do("/v1/email/send", key, fmt.Sprintf(`{"to":%q,"subject":%q,"body":%q}`, to, subject, body))
}

func (hn *harness) do(path, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	hn.h.ServeHTTP(rec, req)
	return rec
}

const goodPhone = "+905551234567"

func messageIDOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		ID string `json:"provider_message_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("no provider_message_id in %q (%v)", rec.Body.String(), err)
	}
	return out.ID
}

func TestHealthz(t *testing.T) {
	hn := newHarness(t, nil)
	rec := httptest.NewRecorder()
	hn.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestOnlyDocumentedRoutesExist(t *testing.T) {
	hn := newHarness(t, nil)
	for _, p := range []string{"/admin/stats", "/admin/config", "/admin/reset", "/metrics", "/v1/push/send"} {
		rec := httptest.NewRecorder()
		hn.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d, want 404/405", p, rec.Code)
		}
	}
}

func TestValidation(t *testing.T) {
	hn := newHarness(t, nil)
	tests := []struct {
		name string
		rec  *httptest.ResponseRecorder
		want string
	}{
		{"missing key", hn.sms("", goodPhone, "hi"), ReasonMissingKey},
		{"sms bad phone", hn.sms("k1", "0555-INVALID", "hi"), ReasonInvalidReceiver},
		{"sms short phone", hn.sms("k2", "+90000", "hi"), ReasonInvalidReceiver},
		{"sms empty body", hn.sms("k3", goodPhone, ""), ReasonInvalidPayload},
		{"email bad address", hn.email("k4", "not-an-email", "s", "b"), ReasonInvalidReceiver},
		{"email no domain", hn.email("k5", "user@", "s", "b"), ReasonInvalidReceiver},
		{"email no local part", hn.email("k6", "@example.com", "s", "b"), ReasonInvalidReceiver},
		{"email empty subject", hn.email("k7", "a@example.com", "", "b"), ReasonInvalidPayload},
		{"broken json", hn.do("/v1/sms/send", "k8", "{not json"), ReasonInvalidPayload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400", tt.rec.Code)
			}
			if !strings.Contains(tt.rec.Body.String(), tt.want) {
				t.Fatalf("body %q does not mention %q", tt.rec.Body.String(), tt.want)
			}
		})
	}
	if got := hn.srv.Stats()[ChannelSMS].UniqueDeliveries + hn.srv.Stats()[ChannelEmail].UniqueDeliveries; got != 0 {
		t.Fatalf("invalid requests must never be delivered, got %d", got)
	}
}

func TestSuccessfulSends(t *testing.T) {
	hn := newHarness(t, nil)
	if rec := hn.sms("a", goodPhone, "hello"); rec.Code != 200 {
		t.Fatalf("sms: %d %s", rec.Code, rec.Body)
	}
	if rec := hn.email("b", "a@example.com", "subject", "body"); rec.Code != 200 {
		t.Fatalf("email: %d %s", rec.Code, rec.Body)
	}
	st := hn.srv.Stats()
	if st[ChannelSMS].UniqueDeliveries != 1 || st[ChannelEmail].UniqueDeliveries != 1 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestSameKeyIsAReplayNotASecondDelivery(t *testing.T) {
	hn := newHarness(t, nil)
	first := hn.sms("key-1", goodPhone, "hello")
	second := hn.sms("key-1", goodPhone, "hello")

	if first.Code != 200 || second.Code != 200 {
		t.Fatalf("codes %d, %d", first.Code, second.Code)
	}
	if first.Header().Get("Idempotent-Replay") != "" {
		t.Fatal("first request must not be marked as a replay")
	}
	if second.Header().Get("Idempotent-Replay") != "true" {
		t.Fatal("second request must be marked as a replay")
	}
	if messageIDOf(t, first) != messageIDOf(t, second) {
		t.Fatal("replay must return the original provider_message_id")
	}
	st := hn.srv.Stats()[ChannelSMS]
	if st.UniqueDeliveries != 1 || st.Replays != 1 {
		t.Fatalf("unique=%d replays=%d, want 1 and 1", st.UniqueDeliveries, st.Replays)
	}
	if !strings.Contains(hn.logs.String(), `"reason":"replay"`) {
		t.Fatalf("replay not logged: %s", hn.logs)
	}
}

func TestSameKeyDifferentPayloadIsRejected(t *testing.T) {
	hn := newHarness(t, nil)
	hn.sms("key-1", goodPhone, "hello")
	rec := hn.sms("key-1", goodPhone, "something else")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422", rec.Code)
	}
	if hn.srv.Stats()[ChannelSMS].UniqueDeliveries != 1 {
		t.Fatal("conflicting reuse must not create a delivery")
	}
}

func TestKeysAreScopedPerChannel(t *testing.T) {
	hn := newHarness(t, nil)
	hn.sms("same", goodPhone, "x")
	if rec := hn.email("same", "a@example.com", "s", "b"); rec.Code != 200 || rec.Header().Get("Idempotent-Replay") != "" {
		t.Fatalf("same key on another channel must be a fresh delivery: %d", rec.Code)
	}
}

func TestFailAfterAcceptRecordsMessageAndRetryIsAReplay(t *testing.T) {
	hn := newHarness(t, func(c *Config) { c.FailAfterAccept = 1 })

	first := hn.sms("key-1", goodPhone, "hello")
	if first.Code < 500 {
		t.Fatalf("first attempt status %d, want 5xx", first.Code)
	}
	st := hn.srv.Stats()[ChannelSMS]
	if st.UniqueDeliveries != 1 || st.FailedAfterAccept != 1 {
		t.Fatalf("message must be recorded despite the 5xx: %+v", st)
	}
	if !strings.Contains(hn.logs.String(), `"reason":"fail_after_accept"`) {
		t.Fatalf("fail_after_accept not logged: %s", hn.logs)
	}

	retry := hn.sms("key-1", goodPhone, "hello")
	if retry.Code != 200 || retry.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("retry with the same key must be answered as a replay, got %d", retry.Code)
	}
	if got := hn.srv.Stats()[ChannelSMS].UniqueDeliveries; got != 1 {
		t.Fatalf("unique deliveries = %d, want 1 (no double delivery)", got)
	}
}

func TestUniqueDeliveriesEqualUniqueKeysUnderRetries(t *testing.T) {
	// Every first attempt "fails after accept" and is retried twice: the
	// provider must still hold exactly one delivery per key.
	hn := newHarness(t, func(c *Config) {
		c.FailAfterAccept = 1
		c.RateLimitPerSecond = 100000
	})
	const keys = 200
	for i := 0; i < keys; i++ {
		key := "key-" + strconv.Itoa(i)
		to := fmt.Sprintf("+90555%07d", i)
		for attempt := 0; attempt < 3; attempt++ {
			hn.sms(key, to, "hello")
		}
	}
	st := hn.srv.Stats()[ChannelSMS]
	if st.UniqueDeliveries != keys {
		t.Fatalf("unique deliveries = %d, want %d", st.UniqueDeliveries, keys)
	}
	if st.Replays != keys*2 {
		t.Fatalf("replays = %d, want %d", st.Replays, keys*2)
	}
}

func TestConcurrentRequestsWithTheSameKeyDeliverOnce(t *testing.T) {
	hn := newHarness(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := hn.sms("shared-key", goodPhone, "hello"); rec.Code != 200 {
				t.Errorf("status %d", rec.Code)
			}
		}()
	}
	wg.Wait()
	st := hn.srv.Stats()[ChannelSMS]
	if st.UniqueDeliveries != 1 || st.Replays != 49 {
		t.Fatalf("unique=%d replays=%d, want 1 and 49", st.UniqueDeliveries, st.Replays)
	}
}

func TestInjected429(t *testing.T) {
	hn := newHarness(t, func(c *Config) { c.Rate429 = 1 })
	rec := hn.sms("k", goodPhone, "hi")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d", rec.Code)
	}
	ra, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil || ra < 1 || ra > 3 {
		t.Fatalf("Retry-After = %q, want 1..3", rec.Header().Get("Retry-After"))
	}
	st := hn.srv.Stats()[ChannelSMS]
	if st.Injected429 != 1 || st.UniqueDeliveries != 0 {
		t.Fatalf("stats %+v", st)
	}
	logs := hn.logs.String()
	if !strings.Contains(logs, `"status":429`) || !strings.Contains(logs, `"reason":"injected"`) {
		t.Fatalf("429 not logged with its reason: %s", logs)
	}
}

func TestInjected5xx(t *testing.T) {
	hn := newHarness(t, func(c *Config) { c.ErrorRate = 1 })
	rec := hn.sms("k", goodPhone, "hi")
	if rec.Code != 500 && rec.Code != 502 && rec.Code != 503 {
		t.Fatalf("status %d, want 500/502/503", rec.Code)
	}
	st := hn.srv.Stats()[ChannelSMS]
	if st.Injected5xx != 1 || st.UniqueDeliveries != 0 {
		t.Fatalf("stats %+v", st)
	}
	if !strings.Contains(hn.logs.String(), `"reason":"injected"`) {
		t.Fatalf("5xx not logged: %s", hn.logs)
	}
}

func TestTimeoutUsesFakeClockNotRealSleep(t *testing.T) {
	hn := newHarness(t, func(c *Config) { c.TimeoutRate = 1; c.TimeoutDelay = 10 * time.Second })
	started := time.Now()
	rec := hn.sms("k", goodPhone, "hi")
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status %d", rec.Code)
	}
	if hn.clk.Now().Sub(t0) < 10*time.Second {
		t.Fatal("fake clock should have advanced by the timeout delay")
	}
	if time.Since(started) > time.Second {
		t.Fatal("test slept for real")
	}
	if hn.srv.Stats()[ChannelSMS].UniqueDeliveries != 0 {
		t.Fatal("a timed-out request must not be delivered")
	}
}

func TestLatencyAdvancesFakeClock(t *testing.T) {
	hn := newHarness(t, func(c *Config) { c.Latency = 50 * time.Millisecond })
	hn.sms("k", goodPhone, "hi")
	if got := hn.clk.Now().Sub(t0); got != 50*time.Millisecond {
		t.Fatalf("clock advanced %v, want 50ms", got)
	}
}

func TestRateLimitRejectsThe101stRequestInAWindow(t *testing.T) {
	hn := newHarness(t, nil)
	for i := 0; i < 100; i++ {
		if rec := hn.sms("k"+strconv.Itoa(i), goodPhone, "hi"); rec.Code != 200 {
			t.Fatalf("request %d: status %d", i, rec.Code)
		}
	}
	rec := hn.sms("k100", goodPhone, "hi")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("101st request: status %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, want 1", rec.Header().Get("Retry-After"))
	}
	if !strings.Contains(hn.logs.String(), `"reason":"rate_limit"`) {
		t.Fatalf("rate limit 429 not logged with its reason: %s", hn.logs)
	}
	if got := hn.srv.Stats()[ChannelSMS].RateLimited; got != 1 {
		t.Fatalf("RateLimited = %d", got)
	}

	hn.clk.Advance(time.Second)
	if rec := hn.sms("k101", goodPhone, "hi"); rec.Code != 200 {
		t.Fatalf("after the window passed: status %d", rec.Code)
	}
}

func TestRateLimitIsPerChannel(t *testing.T) {
	hn := newHarness(t, nil)
	for i := 0; i < 100; i++ {
		hn.sms("s"+strconv.Itoa(i), goodPhone, "hi")
	}
	if rec := hn.email("e0", "a@example.com", "s", "b"); rec.Code != 200 {
		t.Fatalf("email must not be affected by SMS load: %d", rec.Code)
	}
}

func TestNoSlidingWindowEverExceedsTheLimit(t *testing.T) {
	// Offer 200 requests/second (one every 5ms) for 5 simulated seconds and
	// verify, independently of the provider's own counter, that no 1-second
	// window contains more than 100 accepted requests.
	hn := newHarness(t, nil)
	var accepted []time.Time
	rejected := 0
	for i := 0; i < 1000; i++ {
		hn.clk.Advance(5 * time.Millisecond)
		rec := hn.sms("k"+strconv.Itoa(i), goodPhone, "hi")
		switch rec.Code {
		case 200:
			accepted = append(accepted, hn.clk.Now())
		case 429:
			rejected++
		default:
			t.Fatalf("unexpected status %d", rec.Code)
		}
	}
	if rejected == 0 {
		t.Fatal("expected some 429s at twice the allowed rate")
	}
	lo := 0
	for hi := range accepted {
		for accepted[hi].Sub(accepted[lo]) >= time.Second {
			lo++
		}
		if n := hi - lo + 1; n > 100 {
			t.Fatalf("%d accepted requests inside one second ending at %v", n, accepted[hi])
		}
	}
	if got := hn.srv.Stats()[ChannelSMS].MaxPerWindow; got > 100 {
		t.Fatalf("provider counter reports %d in one window", got)
	}
}

func TestStatsCountEveryOutcome(t *testing.T) {
	hn := newHarness(t, nil)
	hn.sms("", goodPhone, "hi")  // bad
	hn.sms("a", goodPhone, "hi") // ok
	hn.sms("a", goodPhone, "hi") // replay
	hn.sms("b", "+90000", "hi")  // bad
	st := hn.srv.Stats()[ChannelSMS]
	if st.Received != 4 || st.BadRequests != 2 || st.UniqueDeliveries != 1 || st.Replays != 1 {
		t.Fatalf("stats %+v", st)
	}
}

func TestSameSeedGivesTheSameChaosSequence(t *testing.T) {
	// Fractional rates are fine here because the seed fixes the outcome: two
	// providers with the same seed must behave identically.
	run := func() []int {
		hn := newHarness(t, func(c *Config) {
			c.Seed = 7
			c.ErrorRate = 0.3
			c.Rate429 = 0.3
			c.RateLimitPerSecond = 100000
		})
		var codes []int
		for i := 0; i < 50; i++ {
			codes = append(codes, hn.sms("k"+strconv.Itoa(i), goodPhone, "hi").Code)
		}
		return codes
	}
	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("run diverged at request %d: %d vs %d", i, a[i], b[i])
		}
	}
}

func TestLoadConfig(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	c, err := LoadConfig(env(nil))
	if err != nil || c != DefaultConfig() {
		t.Fatalf("empty env must give defaults: %+v %v", c, err)
	}

	c, err = LoadConfig(env(map[string]string{
		"PROVIDER_ERROR_RATE": "0", "PROVIDER_429_RATE": "0", "PROVIDER_LATENCY_MS": "5",
		"PROVIDER_RATE_LIMIT_PER_SEC": "10", "PROVIDER_SEED": "9",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ErrorRate != 0 || c.Rate429 != 0 || c.Latency != 5*time.Millisecond || c.RateLimitPerSecond != 10 || c.Seed != 9 {
		t.Fatalf("config %+v", c)
	}

	for name, m := range map[string]map[string]string{
		"rate above 1":     {"PROVIDER_ERROR_RATE": "1.5"},
		"negative latency": {"PROVIDER_LATENCY_MS": "-1"},
		"not a number":     {"PROVIDER_429_RATE": "lots"},
		"zero limit":       {"PROVIDER_RATE_LIMIT_PER_SEC": "0"},
		"bad retry range":  {"PROVIDER_RETRY_AFTER_MIN_SEC": "5", "PROVIDER_RETRY_AFTER_MAX_SEC": "2"},
	} {
		if _, err := LoadConfig(env(m)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
