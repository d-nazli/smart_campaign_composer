package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/clock"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/offer"
	"smartcampaign/internal/provider"
	"smartcampaign/internal/queue"
	"smartcampaign/internal/testdb"
	"smartcampaign/internal/worker"
)

var (
	ctx = context.Background()
	log = slog.New(slog.NewTextHandler(io.Discard, nil))
)

// ---- the provider under test -------------------------------------------------

type request struct {
	Channel, To, Subject, Body, Key string
	At                              time.Time
}

// testProvider is the real mock provider behind an httptest server that also
// records every request the worker makes.
type testProvider struct {
	URL  string
	prov *provider.Server
	mu   sync.Mutex
	reqs []request
}

func (p *testProvider) requests() []request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]request(nil), p.reqs...)
}

func (p *testProvider) stats(channel string) provider.ChannelStats { return p.prov.Stats()[channel] }

// quiet is the provider without any chaos and without latency.
func quiet() provider.Config {
	c := provider.DefaultConfig()
	c.Latency, c.LatencyJitter = 0, 0
	c.ErrorRate, c.Rate429, c.TimeoutRate, c.FailAfterAccept = 0, 0, 0, 0
	c.RetryAfterMin, c.RetryAfterMax = 1, 1
	return c
}

func newProvider(t *testing.T, cfg provider.Config, clk clock.Clock) *testProvider {
	t.Helper()
	tp := &testProvider{prov: provider.New(cfg, clk, log)}
	h := tp.prov.Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var b struct{ To, Subject, Body string }
		_ = json.Unmarshal(raw, &b)
		ch := "sms"
		if strings.Contains(r.URL.Path, "email") {
			ch = "email"
		}
		tp.mu.Lock()
		tp.reqs = append(tp.reqs, request{ch, b.To, b.Subject, b.Body, r.Header.Get("Idempotency-Key"), clk.Now()})
		tp.mu.Unlock()
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	tp.URL = srv.URL
	return tp
}

// ---- fixtures -----------------------------------------------------------------

func fakeClock() *clock.Fake { return clock.NewFake(time.Now().Add(time.Minute).Truncate(time.Second)) }

func testConfig() worker.Config {
	cfg := worker.DefaultConfig()
	cfg.Concurrency = 1 // deterministic with the fake clock
	cfg.Policy.Rand = func() float64 { return 0.5 }
	return cfg
}

func newWorker(t *testing.T, q queue.Queue, url string, clk clock.Clock, cfg worker.Config) *worker.Worker {
	t.Helper()
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	return worker.New(q, worker.NewHTTPSender(url, 5*time.Second), policy, clk, log, cfg)
}

// sqlCampaign inserts an approved campaign with small hand-made content and
// deliveries for recipients whose phone / email the provider accepts.
func sqlCampaign(t *testing.T, pool *pgxpool.Pool, nSMS, nEmail int) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO campaigns (brief, status) VALUES ('b', 'approved') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// One text per locale, as a real approval guarantees.
	_, err := pool.Exec(ctx, `INSERT INTO campaign_contents (campaign_id, locale, sms_body, email_subject, email_body, source, source_version)
		SELECT $1, code,
		       CASE WHEN code LIKE 'en-%' THEN '{{first_name}}, 15% off is waiting for you!'
		            WHEN code LIKE 'de-%' THEN '{{first_name}}, 15 % Rabatt wartet auf Sie!'
		            WHEN is_source THEN '{{first_name}}, %15 indirim seni bekliyor!'
		            ELSE '[' || code || '] {{first_name}}, %15 indirim seni bekliyor!' END,
		       CASE WHEN code LIKE 'en-%' THEN '15% off' WHEN code LIKE 'de-%' THEN '15 % Rabatt' ELSE '%15 indirim' END,
		       CASE WHEN code LIKE 'en-%' THEN 'Hi {{first_name}}, enjoy 15% off.'
		            WHEN code LIKE 'de-%' THEN 'Hallo {{first_name}}, 15 % Rabatt für Sie.'
		            WHEN is_source THEN 'Merhaba {{first_name}}, %15 indirim var.'
		            ELSE '[' || code || '] Merhaba {{first_name}}, %15 indirim var.' END,
		       CASE WHEN is_source THEN 'llm' ELSE 'translated' END,
		       CASE WHEN is_source THEN NULL ELSE 1 END
		FROM locales`, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deliveries (campaign_id, user_id, channel)
		SELECT $1, id, 'sms' FROM users WHERE sms_consent AND phone ~ '^\+90[0-9]{10}$' ORDER BY id LIMIT $2`, id, nSMS); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deliveries (campaign_id, user_id, channel)
		SELECT $1, id, 'email' FROM users WHERE email_consent AND email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$' ORDER BY id LIMIT $2`, id, nEmail); err != nil {
		t.Fatal(err)
	}
	return id
}

// approvedCampaign goes through the real flow: brief, messages, approval.
func approvedCampaign(t *testing.T, pool *pgxpool.Pool) (int64, *campaign.ApproveResult) {
	t.Helper()
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := campaign.NewService(pool, mock.New(0), policy, log)
	v, err := svc.Create(ctx, "Son 7 günde sepete ürün ekleyip almayanlara %15 indirim hatırlatması gönder")
	if err != nil || v.Segment.Status != "ok" {
		t.Fatalf("create: %v %+v", err, v)
	}
	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Approve(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	return v.ID, res
}

func counts(t *testing.T, pool *pgxpool.Pool, id int64) map[string]int {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT channel || '/' || status, count(*) FROM deliveries WHERE campaign_id=$1 GROUP BY 1`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			t.Fatal(err)
		}
		out[k] = n
	}
	return out
}

func unfinished(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM deliveries WHERE status IN ('pending','sending')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// drain runs the worker until nothing is left; when only backed-off rows remain
// it moves the fake clock forward instead of sleeping.
func drain(t *testing.T, w *worker.Worker, pool *pgxpool.Pool, clk *clock.Fake) {
	t.Helper()
	for i := 0; i < 20000; i++ {
		n, err := w.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			if unfinished(t, pool) == 0 {
				return
			}
			clk.Advance(5 * time.Second)
		}
	}
	t.Fatal("the worker did not finish the queue")
}

func scalar(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ---- the whole flow --------------------------------------------------------------

// Brief -> approval -> worker -> provider: everyone reachable gets exactly one
// message, nothing breaks a limit, bad addresses fail without retries.
func TestApprovedCampaignIsDeliveredToEveryReachableRecipientOnce(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id, approved := approvedCampaign(t, pool)
	if approved.Queued.SMS < 100 || approved.Queued.Email < 100 {
		t.Fatalf("audience too small to prove anything: %+v", approved.Queued)
	}
	clk := fakeClock()
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	drain(t, w, pool, clk)

	c := counts(t, pool, id)
	if c["sms/sent"]+c["sms/failed"] != approved.Queued.SMS || c["email/sent"]+c["email/failed"] != approved.Queued.Email {
		t.Fatalf("not every delivery reached a final state: %v vs %+v", c, approved.Queued)
	}
	if c["sms/failed"] == 0 || c["email/failed"] == 0 {
		t.Fatalf("the seed contains malformed contacts, some deliveries must fail: %v", c)
	}
	// What failed, failed for good on the first try: a bad address is not retried.
	if n := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id=$1 AND status='failed'
		AND (retries <> 0 OR attempts <> 0 OR last_error NOT LIKE '%invalid_recipient%')`, id); n != 0 {
		t.Fatalf("%d failed deliveries were retried or failed for another reason", n)
	}
	// The provider holds exactly one message per sent delivery, no duplicates.
	for _, ch := range []string{"sms", "email"} {
		if got := tp.stats(ch).UniqueDeliveries; got != c[ch+"/sent"] {
			t.Errorf("%s: provider holds %d messages, %d deliveries are sent", ch, got, c[ch+"/sent"])
		}
	}
	// Personalised, within limits, no template left over, for every recipient.
	for _, r := range tp.requests() {
		if strings.Contains(r.Body, "{{") || strings.Contains(r.Subject, "{{") {
			t.Fatalf("unrendered template: %+v", r)
		}
		if r.Channel == "sms" && utf8.RuneCountInString(r.Body) > 160 {
			t.Fatalf("sms of %d characters to %s", utf8.RuneCountInString(r.Body), r.To)
		}
		if r.Channel == "email" && (utf8.RuneCountInString(r.Subject) > 80 || strings.ContainsAny(r.Subject, "\r\n")) {
			t.Fatalf("bad subject: %q", r.Subject)
		}
		if !(offer.Offer{Kind: offer.KindPercent, Value: 15}).Mentions(r.Body) {
			t.Fatalf("the offer is missing from a sent message: %q", r.Body)
		}
	}
	if got := scalar(t, pool, `SELECT count(*) FROM campaigns WHERE id=$1 AND status='completed'`, id); got != 1 {
		t.Fatal("the campaign was not marked completed")
	}
}

// Chaos: 5xx, 429s, timeouts and "accepted but answered with an error". Every
// delivery ends sent or failed, and the idempotency key keeps the provider at
// one message per recipient even though the worker sent some of them twice.
func TestChaoticProviderEndsInFinalStatesWithoutDuplicates(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id := sqlCampaign(t, pool, 150, 0)
	clk := fakeClock()
	cfg := quiet()
	cfg.ErrorRate, cfg.Rate429, cfg.TimeoutRate, cfg.FailAfterAccept, cfg.Seed = 0.25, 0.10, 0.05, 0.20, 7
	tp := newProvider(t, cfg, clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	drain(t, w, pool, clk)

	c := counts(t, pool, id)
	sent, failed := c["sms/sent"], c["sms/failed"]
	if sent+failed != 150 || c["sms/pending"] != 0 || c["sms/sending"] != 0 {
		t.Fatalf("not final: %v", c)
	}
	if failed > 15 {
		t.Fatalf("%d of 150 failed; five attempts should rescue almost all: %v", failed, c)
	}
	st := tp.stats("sms")
	if st.UniqueDeliveries < sent || st.UniqueDeliveries > sent+failed {
		t.Fatalf("provider holds %d messages for %d sent / %d failed", st.UniqueDeliveries, sent, failed)
	}
	if st.Replays == 0 || st.FailedAfterAccept == 0 {
		t.Fatalf("the chaos did not exercise idempotency: %+v", st)
	}
	// One provider key per delivery, never two different recipients on a key.
	keys := map[string]string{}
	for _, r := range tp.requests() {
		if to, seen := keys[r.Key]; seen && to != r.To {
			t.Fatalf("key %s used for %s and %s", r.Key, to, r.To)
		}
		keys[r.Key] = r.To
	}
	if scalar(t, pool, `SELECT coalesce(sum(retries),0) FROM deliveries WHERE campaign_id=$1`, id) == 0 {
		t.Fatal("no retries were recorded")
	}
}

// The worker dies after the provider accepted a message but before "sent" was
// written. The row is reclaimed after its lease and sent again with the same
// key: the provider answers from memory, the person gets ONE message.
func TestCrashBeforeMarkSentIsRecoveredWithoutASecondMessage(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id := sqlCampaign(t, pool, 1, 0)
	clk := fakeClock()
	tp := newProvider(t, quiet(), clk)
	q := &dropFirstMarkSent{Queue: queue.NewPG(pool)}
	w := newWorker(t, q, tp.URL, clk, testConfig())

	if n, err := w.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("first run: n=%d err=%v", n, err)
	}
	if c := counts(t, pool, id); c["sms/sending"] != 1 {
		t.Fatalf("the lost result should leave the row in flight: %v", c)
	}
	if n, _ := w.RunOnce(ctx); n != 0 {
		t.Fatal("a live lease must not be taken over")
	}
	clk.Advance(31 * time.Second)
	if n, err := w.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("recovery run: n=%d err=%v", n, err)
	}

	if c := counts(t, pool, id); c["sms/sent"] != 1 {
		t.Fatalf("not recovered: %v", c)
	}
	st := tp.stats("sms")
	if st.UniqueDeliveries != 1 || st.Replays != 1 || st.Received != 2 {
		t.Fatalf("provider: %+v; want one message, received twice, one replay", st)
	}
	reqs := tp.requests()
	if len(reqs) != 2 || reqs[0].Key != reqs[1].Key || reqs[0].Body != reqs[1].Body {
		t.Fatalf("the repeat must be identical: %+v", reqs)
	}
}

type dropFirstMarkSent struct {
	queue.Queue
	dropped atomic.Bool
}

func (d *dropFirstMarkSent) MarkSent(ctx context.Context, id int64, pid string, now time.Time) error {
	if d.dropped.CompareAndSwap(false, true) {
		return nil // "the process died here"
	}
	return d.Queue.MarkSent(ctx, id, pid, now)
}

// A 429 is obeyed: the refused delivery waits for its Retry-After and costs no
// attempt. A single 429 does not pause the channel: the next delivery goes out
// at once. Only a burst (10 or more 429s within one second) pauses the whole
// channel (Pacer.RateLimited, tested in pacer_test.go).
func TestALone429DelaysOnlyItsOwnMessage(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	sqlCampaign(t, pool, 2, 0)
	clk := fakeClock()

	var mu sync.Mutex
	var at []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		at = append(at, clk.Now())
		first := len(at) == 1
		mu.Unlock()
		if first {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":"rate_limit"}`))
			return
		}
		_, _ = w.Write([]byte(`{"provider_message_id":"pm_x","status":"accepted"}`))
	}))
	defer srv.Close()

	q := queue.NewPG(pool)
	w := newWorker(t, q, srv.URL, clk, testConfig())
	// First delivery: refused with 429, Retry-After 7s.
	if n, err := w.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	mu.Lock()
	t1 := at[0]
	mu.Unlock()
	// Second delivery (a different recipient): one stray 429 does not stall the
	// channel, so it goes out at once (the refused one waits for its own Retry-After).
	if n, err := w.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	mu.Lock()
	t2 := at[1]
	mu.Unlock()
	if t2.Sub(t1) >= 7*time.Second {
		t.Fatalf("one stray 429 stalled the whole channel for %v", t2.Sub(t1))
	}

	var attempts, limited int
	var next time.Time
	if err := pool.QueryRow(ctx, `SELECT attempts, rate_limited, next_attempt_at FROM deliveries WHERE status='pending'`).Scan(&attempts, &limited, &next); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || limited != 1 || !next.Equal(t1.Add(7*time.Second)) {
		t.Fatalf("attempts=%d rate_limited=%d next=%v (want %v)", attempts, limited, next, t1.Add(7*time.Second))
	}
	if got, _ := q.Claim(ctx, "sms", 10, time.Second, t1.Add(6*time.Second)); len(got) != 0 {
		t.Fatal("the refused delivery was claimable before Retry-After was over")
	}
	// Give the claim above back (its lease is 1s) and let the worker finish.
	clk.Advance(2 * time.Second)
	drain(t, w, pool, clk)
	if scalar(t, pool, `SELECT count(*) FROM deliveries WHERE status='sent' AND attempts=0`) != 2 {
		t.Fatal("both deliveries should end sent without having used an attempt")
	}
}

func TestConsentWithdrawnAfterApprovalIsRespected(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id, approved := approvedCampaign(t, pool)

	rows, err := pool.Query(ctx, `UPDATE users SET sms_consent=false
		WHERE id IN (SELECT user_id FROM deliveries WHERE campaign_id=$1 AND channel='sms' ORDER BY id LIMIT 10)
		RETURNING phone`, id)
	if err != nil {
		t.Fatal(err)
	}
	withdrawn := map[string]bool{}
	for rows.Next() {
		var p *string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		if p != nil {
			withdrawn[*p] = true
		}
	}
	rows.Close()

	clk := fakeClock()
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	drain(t, w, pool, clk)

	c := counts(t, pool, id)
	if c["sms/skipped"] != 10 {
		t.Fatalf("skipped %d, want 10: %v (queued %+v)", c["sms/skipped"], c, approved.Queued)
	}
	for _, r := range tp.requests() {
		if r.Channel == "sms" && withdrawn[r.To] {
			t.Fatalf("sent an SMS to %s after consent was withdrawn", r.To)
		}
	}
	if n := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE status='skipped' AND last_error NOT LIKE '%consent%'`); n != 0 {
		t.Fatal("a skipped delivery must say why")
	}
}

// The channel limit is respected for real: with the provider enforcing 100/s,
// the worker sends 300 messages and is never told to slow down.
func TestRealPaceNeverTriggersTheProvidersRateLimit(t *testing.T) {
	pool := testdb.NewSeeded(t)
	id := sqlCampaign(t, pool, 300, 0)
	cfg := quiet() // real limit: 100 per second, real clock
	tp := newProvider(t, cfg, clock.Real{})

	wcfg := worker.DefaultConfig() // 64 senders, 97 per second
	w := newWorker(t, queue.NewPG(pool), tp.URL, clock.Real{}, wcfg)

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- w.Run(runCtx) }()

	deadline := time.Now().Add(30 * time.Second)
	for scalar(t, pool, `SELECT count(*) FROM campaigns WHERE id=$1 AND status='completed'`, id) == 0 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("not finished in 30s: %v", counts(t, pool, id))
		}
		time.Sleep(100 * time.Millisecond)
	}
	elapsed := time.Since(start)
	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after its context ended")
	}

	if c := counts(t, pool, id); c["sms/sent"] != 300 {
		t.Fatalf("%v", c)
	}
	st := tp.stats("sms")
	if st.RateLimited != 0 || st.MaxPerWindow > 100 {
		t.Fatalf("the provider had to push back: %+v", st)
	}
	var times []time.Time
	for _, r := range tp.requests() {
		times = append(times, r.At)
	}
	if got := maxInAnyWindow(times); got > 100 {
		t.Fatalf("%d requests inside one second", got)
	}
	// The busiest second the sending screen shows ("En yüksek (1 sn)") is read
	// from the calls the worker recorded; it must agree that 100 was never passed.
	peaks, err := campaign.PeakPerSecond(ctx, pool, id)
	if err != nil {
		t.Fatal(err)
	}
	if peaks["sms"] == 0 || peaks["sms"] > 100 {
		t.Fatalf("recorded busiest second: %v", peaks)
	}
	// 300 messages at 97/s cannot take less than ~3.1s.
	if elapsed < 3*time.Second {
		t.Fatalf("300 messages in %v: faster than the configured 97/s", elapsed)
	}
}

// Every provider call is timed, retries included, so the response-time metric
// reflects what the provider really did.
func TestEveryProviderCallIsTimed(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id := sqlCampaign(t, pool, 20, 0)
	clk := fakeClock()
	cfg := quiet()
	cfg.Latency = 40 * time.Millisecond // the fake clock makes this exact
	cfg.ErrorRate, cfg.Seed = 0.3, 3
	tp := newProvider(t, cfg, clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	drain(t, w, pool, clk)

	var calls int
	var micros int64
	if err := pool.QueryRow(ctx, `SELECT sum(provider_calls), sum(provider_micros) FROM deliveries WHERE campaign_id=$1`, id).Scan(&calls, &micros); err != nil {
		t.Fatal(err)
	}
	if want := tp.stats("sms").Received; calls != want {
		t.Fatalf("recorded %d calls, the provider received %d", calls, want)
	}
	if calls <= 20 {
		t.Fatalf("%d calls for 20 deliveries: the chaos should have forced retries", calls)
	}
	if micros != int64(calls)*40_000 {
		t.Fatalf("total %dµs for %d calls of 40ms each", micros, calls)
	}
}

// ---- locales ---------------------------------------------------------------------

// wording is how the discount reads in that locale's language only (the mock
// translates every locale from its sentence bank).
func wording(code string) string {
	lang, _, _ := strings.Cut(code, "-")
	return map[string]string{"tr": "indirim", "en": "off", "de": "Rabatt", "fr": "réduction", "es": "descuento",
		"it": "sconto", "nl": "korting", "pt": "desconto", "ar": "خصم"}[lang]
}

// Everyone gets the text in THEIR locale (an exact match), and each person
// gets it once.
func TestEveryRecipientGetsTheirOwnLocale(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id, approved := approvedCampaign(t, pool)
	clk := fakeClock()
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	drain(t, w, pool, clk)

	// Which locale does each address belong to?
	localeOf := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT coalesce(phone, ''), coalesce(email, ''), locale FROM users`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var phone, email, code string
		if err := rows.Scan(&phone, &email, &code); err != nil {
			t.Fatal(err)
		}
		for key, addr := range map[string]string{"sms:" + phone: phone, "email:" + email: email} {
			if addr == "" {
				continue
			}
			if _, dup := localeOf[key]; dup {
				localeOf[key] = "?" // the malformed addresses of the seed are shared by several people
			} else {
				localeOf[key] = code
			}
		}
	}
	rows.Close()

	perLocale := map[string]int{}
	for _, r := range tp.requests() {
		code, ok := localeOf[r.Channel+":"+r.To]
		if !ok || code == "?" {
			continue // a malformed address nobody (or several people) own
		}
		perLocale[code]++
		text := r.Subject + " " + r.Body
		if w := wording(code); w == "" || !strings.Contains(text, w) || (code != "tr-TR" && strings.Contains(text, "indirim")) {
			t.Fatalf("%s got %q", code, text)
		}
	}
	for _, code := range []string{"tr-TR", "en-GB", "en-US", "de-DE", "fr-FR", "ar-SA"} {
		if perLocale[code] == 0 {
			t.Fatalf("nobody got a %s message: %v", code, perLocale)
		}
	}
	if c := counts(t, pool, id); c["sms/sent"] == 0 || c["email/sent"] == 0 {
		t.Fatalf("%v (queued %+v)", c, approved.Queued)
	}
	// The locale each message was written in is kept on the delivery.
	if n := scalar(t, pool, `SELECT count(*) FROM deliveries d JOIN users u ON u.id = d.user_id
		WHERE d.campaign_id=$1 AND d.status='sent' AND d.locale IS DISTINCT FROM u.locale`, id); n != 0 {
		t.Fatalf("%d sent deliveries do not record their locale", n)
	}
}

// A recipient whose locale has no stored text gets nothing: never another
// locale's text (Insider's localization drops them the same way).
func TestARecipientWhoseLocaleHasNoTextGetsNothing(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	id := sqlCampaign(t, pool, 0, 0)
	if _, err := pool.Exec(ctx, `DELETE FROM campaign_contents WHERE campaign_id=$1 AND locale='de-DE'`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deliveries (campaign_id, user_id, channel)
		SELECT $1, id, 'sms' FROM users WHERE locale='de-DE' AND sms_consent AND phone ~ '^\+90[0-9]{10}$' ORDER BY id LIMIT 1`, id); err != nil {
		t.Fatal(err)
	}
	clk := fakeClock()
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	if n, err := w.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if got := len(tp.requests()); got != 0 {
		t.Fatalf("%d messages were sent without a de-DE text", got)
	}
	if n := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id=$1 AND status='skipped' AND last_error LIKE 'no content for locale%de-DE'`, id); n != 1 {
		t.Fatalf("the delivery should be skipped with the reason: %v", counts(t, pool, id))
	}
}

// The api validated the template with a SMALLER first_name allowance (18) than
// the worker renders with (20): a 20-letter name would push the SMS to 162 and
// the subject to 82 characters. The worker measures again and does not send
// it; the delivery fails for good with the reason. A short name still goes out.
func TestAMessageOverTheLimitAfterPersonalizationIsNeverSent(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO campaigns (brief, status) VALUES ('b', 'approved') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	narrow, err := content.NewPolicy(map[string]int{"first_name": 18})
	if err != nil {
		t.Fatal(err)
	}
	tmpl := content.Content{
		SMSBody:      "{{first_name}}" + strings.Repeat("a", 142),
		EmailSubject: "{{first_name}}" + strings.Repeat("b", 62),
		EmailBody:    "Merhaba {{first_name}}",
	}
	if _, rep := narrow.Validate(tmpl); !rep.OK || rep.SMSLength != 160 || rep.SubjectLength != 80 {
		t.Fatalf("the template must be exactly at the limits for an 18-character name: %+v", rep)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campaign_contents (campaign_id, locale, sms_body, email_subject, email_body, source)
		VALUES ($1, 'tr-TR', $2, $3, $4, 'llm')`, id, tmpl.SMSBody, tmpl.EmailSubject, tmpl.EmailBody); err != nil {
		t.Fatal(err)
	}
	var longID, shortID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE sms_consent AND email_consent AND phone ~ '^\+90[0-9]{10}$'
		AND email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$' ORDER BY id LIMIT 1`).Scan(&longID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE sms_consent AND email_consent AND phone ~ '^\+90[0-9]{10}$'
		AND email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$' AND id > $1 ORDER BY id LIMIT 1`, longID).Scan(&shortID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET first_name = CASE WHEN id=$1 THEN 'Abdurrahmanşerifxxxx' ELSE 'Ali' END, locale = 'tr-TR'
		WHERE id IN ($1, $2)`, longID, shortID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deliveries (campaign_id, user_id, channel)
		SELECT $1, u, ch FROM unnest(ARRAY[$2::bigint, $3::bigint]) u, unnest(ARRAY['sms', 'email']) ch`, id, longID, shortID); err != nil {
		t.Fatal(err)
	}

	clk := fakeClock()
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig()) // default allowance: 20
	drain(t, w, pool, clk)

	for _, r := range tp.requests() {
		if utf8.RuneCountInString(r.Body) > 160 && r.Channel == "sms" || utf8.RuneCountInString(r.Subject) > 80 {
			t.Fatalf("an over-long message reached the provider: %+v", r)
		}
	}
	if got := len(tp.requests()); got != 2 {
		t.Fatalf("provider got %d requests, want only the short name's sms and email", got)
	}
	if n := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id=$1 AND user_id=$2
		AND status='failed' AND attempts=0 AND last_error LIKE 'message too long%'`, id, longID); n != 2 {
		t.Fatalf("%d of the long name's deliveries failed with the reason, want 2", n)
	}
	if n := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id=$1 AND user_id=$2 AND status='sent'`, id, shortID); n != 2 {
		t.Fatalf("the short name's messages were not sent (%d)", n)
	}
}

// ---- A/B variants ----------------------------------------------------------------

// With Variant B every person gets the text of their own variant (decided when
// queued) in their own locale, and each delivery once.
func TestEveryRecipientGetsTheirVariantInTheirLocale(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := campaign.NewService(pool, mock.New(0), policy, log)
	v, err := svc.Create(ctx, "Son 7 günde sepete ürün ekleyip almayanlara %15 indirim hatırlatması gönder")
	if err != nil || v.Segment.Status != "ok" {
		t.Fatalf("create: %v %+v", err, v)
	}
	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.GenerateVariant(ctx, v.ID, campaign.VariantB, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	clk := fakeClock()
	tp := newProvider(t, quiet(), clk)
	w := newWorker(t, queue.NewPG(pool), tp.URL, clk, testConfig())
	drain(t, w, pool, clk)

	// The text of every (variant, locale), by delivery id.
	type key struct{ variant, locale string }
	texts := map[key]string{}
	rows, err := pool.Query(ctx, `SELECT variant, locale, sms_body FROM campaign_contents WHERE campaign_id=$1`, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k key
		var sms string
		if err := rows.Scan(&k.variant, &k.locale, &sms); err != nil {
			t.Fatal(err)
		}
		texts[k] = sms
	}
	rows.Close()
	contains := func(body, tmpl string) bool {
		for _, piece := range strings.Split(tmpl, "{{first_name}}") {
			if p := strings.TrimSpace(piece); p != "" && !strings.Contains(body, p) {
				return false
			}
		}
		return true
	}
	seen := map[string]int{}
	perVariant := map[string]int{}
	for _, r := range tp.requests() {
		if r.Channel != "sms" {
			continue
		}
		seen[r.Key]++
		var variant, code string
		if err := pool.QueryRow(ctx, `SELECT variant, locale FROM deliveries WHERE id=$1::bigint`, r.Key).Scan(&variant, &code); err != nil {
			t.Fatal(err)
		}
		own, other := texts[key{variant, code}], texts[key{map[string]string{"A": "B", "B": "A"}[variant], code}]
		if !contains(r.Body, own) || (own != other && contains(r.Body, other)) {
			t.Fatalf("delivery %s (%s, %s) got %q, its text is %q", r.Key, variant, code, r.Body, own)
		}
		perVariant[variant]++
	}
	for k, n := range seen {
		if n != 1 {
			t.Fatalf("delivery %s sent %d times", k, n)
		}
	}
	if perVariant["A"] == 0 || perVariant["B"] == 0 {
		t.Fatalf("sent per variant %v", perVariant)
	}
}

// ---- failures per channel --------------------------------------------------------

// A message fails for a reason of its own channel (a malformed phone number
// is not a malformed e-mail address): over a wide audience the people whose SMS
// failed and the people whose e-mail failed are not the same set.
func TestTheTwoChannelsFailForDifferentPeople(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := campaign.NewService(pool, mock.New(0), policy, log)
	v, err := svc.Create(ctx, "Tüm müşterilere %10 indirim gönder")
	if err != nil || v.Segment.Status != "ok" {
		t.Fatalf("create: %v %+v", err, v)
	}
	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	clk := fakeClock()
	w := newWorker(t, queue.NewPG(pool), newProvider(t, quiet(), clk).URL, clk, testConfig())
	drain(t, w, pool, clk)

	failed := func(channel string) map[int64]bool {
		rows, err := pool.Query(ctx, `SELECT user_id FROM deliveries WHERE campaign_id=$1 AND channel=$2 AND status='failed'`, v.ID, channel)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[int64]bool{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			out[id] = true
		}
		return out
	}
	sms, email := failed("sms"), failed("email")
	if len(sms) == 0 || len(email) == 0 {
		t.Fatalf("expected failures on both channels: sms %d, email %d", len(sms), len(email))
	}
	both := 0
	for id := range sms {
		if email[id] {
			both++
		}
	}
	if both == len(sms) || both == len(email) {
		t.Fatalf("the same people failed on both channels: sms %d, email %d, both %d", len(sms), len(email), both)
	}
	if both > len(sms)/2 {
		t.Fatalf("most people who failed on SMS also failed on e-mail: sms %d, both %d", len(sms), both)
	}
}
