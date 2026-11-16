package metrics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/metrics"
	"smartcampaign/internal/testdb"
)

var ctx = context.Background()

// ---- reading the exposition format ----------------------------------------------

var (
	sampleRe = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[a-zA-Z_][a-zA-Z0-9_]*="[^"\\]*"(,[a-zA-Z_][a-zA-Z0-9_]*="[^"\\]*")*\})? (-?[0-9][0-9.]*(e[+-]?[0-9]+)?|\+Inf|NaN)$`)
	typeRe   = regexp.MustCompile(`^# TYPE ([a-zA-Z_:][a-zA-Z0-9_:]*) (counter|gauge|summary|histogram|untyped)$`)
)

// parse checks that text is valid Prometheus text format (every sample has a
// HELP and a TYPE before it, no series twice) and returns series -> value.
func parse(t *testing.T, text string) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	help, typed := map[string]bool{}, map[string]string{}
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("exposition must end with a newline")
	}
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "# HELP "):
			help[strings.Fields(line)[2]] = true
		case strings.HasPrefix(line, "# TYPE "):
			m := typeRe.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("bad TYPE line: %q", line)
			}
			typed[m[1]] = m[2]
		case line == "":
			t.Fatal("blank line in exposition")
		default:
			m := sampleRe.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("not a valid sample line: %q", line)
			}
			family := strings.TrimSuffix(strings.TrimSuffix(m[1], "_sum"), "_count")
			if typed[m[1]] == "" && typed[family] != "summary" {
				t.Fatalf("sample %q has no TYPE before it", line)
			}
			if !help[m[1]] && !help[family] {
				t.Fatalf("sample %q has no HELP before it", line)
			}
			key := m[1] + m[2]
			if _, dup := out[key]; dup {
				t.Fatalf("series %s appears twice", key)
			}
			v, err := strconv.ParseFloat(m[4], 64)
			if err != nil {
				t.Fatalf("value of %q: %v", line, err)
			}
			out[key] = v
		}
	}
	return out
}

func render(t *testing.T, pool *pgxpool.Pool, now time.Time) map[string]float64 {
	t.Helper()
	text, err := metrics.NewAt(pool, func() time.Time { return now }).Render(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return parse(t, text)
}

func expect(t *testing.T, got map[string]float64, want map[string]float64) {
	t.Helper()
	for series, w := range want {
		g, ok := got[series]
		if !ok {
			t.Errorf("missing series %s", series)
			continue
		}
		if g != w {
			t.Errorf("%s = %v, want %v", series, g, w)
		}
	}
}

// ---- fixtures ---------------------------------------------------------------------

type fixture struct {
	pool     *pgxpool.Pool
	campaign int64
	now      time.Time
	next     int // users already used by a delivery row (the unique key needs distinct users)
}

func setup(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.NewSeeded(t)
	f := &fixture{pool: pool, now: time.Now().UTC().Truncate(time.Second)}
	if err := pool.QueryRow(ctx, `INSERT INTO campaigns (brief, status, segment_status) VALUES ('b', 'sending', 'ok') RETURNING id`).Scan(&f.campaign); err != nil {
		t.Fatal(err)
	}
	return f
}

// deliveries inserts n rows that all look the same.
func (f *fixture) deliveries(t *testing.T, channel, status string, n, retries, limited, calls int, micros int64, sentAgo time.Duration) {
	t.Helper()
	var sentAt *time.Time
	if status == "sent" {
		at := f.now.Add(-sentAgo)
		sentAt = &at
	}
	for i := 0; i < n; i++ {
		f.next++
		_, err := f.pool.Exec(ctx, `
			INSERT INTO deliveries (campaign_id, user_id, channel, status, retries, rate_limited, provider_calls, provider_micros, sent_at)
			VALUES ($1, (SELECT id FROM users ORDER BY id OFFSET $2 LIMIT 1), $3, $4, $5, $6, $7, $8, $9)`,
			f.campaign, f.next, channel, status, retries, limited, calls, micros, sentAt)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// llm inserts one attempt row.
func (f *fixture) llm(t *testing.T, kind string, attempt int, valid, refused, fellBack bool, latencyMs, in, out int) {
	t.Helper()
	_, err := f.pool.Exec(ctx, `
		INSERT INTO llm_generations (campaign_id, kind, attempt, mode, model, valid, refused, fell_back, latency_ms, input_tokens, output_tokens)
		VALUES ($1, $2, $3, 'mock', 'm', $4, $5, $6, $7, $8, $9)`, f.campaign, kind, attempt, valid, refused, fellBack, latencyMs, in, out)
	if err != nil {
		t.Fatal(err)
	}
}

// ---- tests ------------------------------------------------------------------------

func TestDeliveryMetricsAreCountedPerChannel(t *testing.T) {
	f := setup(t)
	s10 := 2 * time.Second // inside the 10 s send-rate window
	old := time.Hour
	f.deliveries(t, "sms", "sent", 3, 1, 1, 2, 300_000, s10)
	f.deliveries(t, "sms", "sent", 2, 0, 0, 1, 100_000, old)
	f.deliveries(t, "sms", "failed", 2, 2, 0, 3, 600_000, 0)
	f.deliveries(t, "sms", "skipped", 1, 0, 0, 0, 0, 0)
	f.deliveries(t, "sms", "pending", 3, 1, 1, 1, 50_000, 0)
	f.deliveries(t, "sms", "sending", 1, 0, 0, 0, 0, 0)
	f.deliveries(t, "email", "sent", 4, 0, 0, 1, 80_000, old)
	f.deliveries(t, "email", "failed", 1, 0, 0, 1, 20_000, 0)
	f.deliveries(t, "email", "pending", 2, 0, 0, 0, 0, 0)

	got := render(t, f.pool, f.now)
	expect(t, got, map[string]float64{
		`smartcampaign_deliveries_total{channel="sms",status="sent"}`:    5,
		`smartcampaign_deliveries_total{channel="sms",status="failed"}`:  2,
		`smartcampaign_deliveries_total{channel="sms",status="skipped"}`: 1,
		`smartcampaign_delivery_queue_length{channel="sms"}`:             4, // pending 3 + sending 1
		`smartcampaign_delivery_retries_total{channel="sms"}`:            10,
		`smartcampaign_delivery_rate_limited_total{channel="sms"}`:       6,
		`smartcampaign_send_rate_per_second{channel="sms"}`:              0.3, // 3 sent in the last 10 s
		`smartcampaign_provider_response_seconds_count{channel="sms"}`:   17,
		`smartcampaign_provider_response_seconds_sum{channel="sms"}`:     2.45,

		`smartcampaign_deliveries_total{channel="email",status="sent"}`:    4,
		`smartcampaign_deliveries_total{channel="email",status="failed"}`:  1,
		`smartcampaign_deliveries_total{channel="email",status="skipped"}`: 0, // present even when zero
		`smartcampaign_delivery_queue_length{channel="email"}`:             2,
		`smartcampaign_delivery_retries_total{channel="email"}`:            0,
		`smartcampaign_delivery_rate_limited_total{channel="email"}`:       0,
		`smartcampaign_send_rate_per_second{channel="email"}`:              0,
		`smartcampaign_provider_response_seconds_count{channel="email"}`:   5,
		`smartcampaign_provider_response_seconds_sum{channel="email"}`:     0.34,
	})
}

// A fresh system has nothing to report, but every series must exist (at zero) so
// that dashboards and rate() work from the first scrape.
func TestAnEmptySystemStillExposesEverySeriesAtZero(t *testing.T) {
	pool, _ := testdb.New(t)
	got := render(t, pool, time.Now())
	for _, ch := range []string{"sms", "email"} {
		for _, st := range []string{"sent", "failed", "skipped"} {
			if v, ok := got[`smartcampaign_deliveries_total{channel="`+ch+`",status="`+st+`"}`]; !ok || v != 0 {
				t.Errorf("deliveries %s/%s: %v %v", ch, st, v, ok)
			}
		}
		for _, name := range []string{"delivery_queue_length", "delivery_retries_total", "delivery_rate_limited_total", "send_rate_per_second"} {
			if v, ok := got[`smartcampaign_`+name+`{channel="`+ch+`"}`]; !ok || v != 0 {
				t.Errorf("%s %s: %v %v", name, ch, v, ok)
			}
		}
	}
	for _, kind := range []string{"segment", "messages"} {
		for _, name := range []string{"llm_generations_total", "llm_first_attempt_valid_total", "llm_repair_attempts_total", "llm_fallbacks_total", "llm_refusals_total"} {
			if v, ok := got[`smartcampaign_`+name+`{kind="`+kind+`"}`]; !ok || v != 0 {
				t.Errorf("%s %s: %v %v", name, kind, v, ok)
			}
		}
	}
	for _, st := range []string{"ok", "needs_clarification", "unsupported"} {
		if v, ok := got[`smartcampaign_segment_outcomes_total{status="`+st+`"}`]; !ok || v != 0 {
			t.Errorf("segment outcome %s: %v %v", st, v, ok)
		}
	}
}

func TestLLMMetricsCountRunsRepairsFallbacksAndRefusals(t *testing.T) {
	f := setup(t)
	// segment run 1: valid first time
	f.llm(t, "segment", 1, true, false, false, 1000, 100, 50)
	// segment run 2: invalid, repaired on the second attempt
	f.llm(t, "segment", 1, false, false, false, 200, 10, 5)
	f.llm(t, "segment", 2, true, false, false, 300, 20, 10)
	// segment run 3: the model refused
	f.llm(t, "segment", 1, false, true, false, 50, 0, 0)
	// messages run 1: valid first time
	f.llm(t, "messages", 1, true, false, false, 500, 300, 200)
	// messages run 2: three invalid attempts, then the built-in template
	f.llm(t, "messages", 1, false, false, false, 400, 100, 100)
	f.llm(t, "messages", 2, false, false, false, 400, 100, 100)
	f.llm(t, "messages", 3, false, false, true, 400, 100, 100)
	// messages run 3: repaired
	f.llm(t, "messages", 1, false, false, false, 400, 100, 100)
	f.llm(t, "messages", 2, true, false, false, 400, 100, 100)

	got := render(t, f.pool, f.now)
	expect(t, got, map[string]float64{
		`smartcampaign_llm_generations_total{kind="segment"}`:               3,
		`smartcampaign_llm_first_attempt_valid_total{kind="segment"}`:       1,
		`smartcampaign_llm_repair_attempts_total{kind="segment"}`:           1,
		`smartcampaign_llm_fallbacks_total{kind="segment"}`:                 0,
		`smartcampaign_llm_refusals_total{kind="segment"}`:                  1,
		`smartcampaign_llm_attempt_seconds_count{kind="segment"}`:           4,
		`smartcampaign_llm_attempt_seconds_sum{kind="segment"}`:             1.55,
		`smartcampaign_llm_tokens_total{kind="segment",direction="input"}`:  130,
		`smartcampaign_llm_tokens_total{kind="segment",direction="output"}`: 65,

		`smartcampaign_llm_generations_total{kind="messages"}`:               3,
		`smartcampaign_llm_first_attempt_valid_total{kind="messages"}`:       1,
		`smartcampaign_llm_repair_attempts_total{kind="messages"}`:           3,
		`smartcampaign_llm_fallbacks_total{kind="messages"}`:                 1,
		`smartcampaign_llm_refusals_total{kind="messages"}`:                  0,
		`smartcampaign_llm_attempt_seconds_count{kind="messages"}`:           6,
		`smartcampaign_llm_attempt_seconds_sum{kind="messages"}`:             2.5,
		`smartcampaign_llm_tokens_total{kind="messages",direction="input"}`:  800,
		`smartcampaign_llm_tokens_total{kind="messages",direction="output"}`: 700,
	})
}

// needs_clarification and unsupported are decided in code as well as by the
// model, so they are counted from the campaigns, not from the LLM audit rows.
func TestSegmentOutcomesAreCountedFromCampaigns(t *testing.T) {
	f := setup(t) // one campaign with segment_status ok
	for _, st := range []string{"ok", "needs_clarification", "unsupported", "unsupported", "unsupported"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO campaigns (brief, segment_status) VALUES ('b', $1)`, st); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO campaigns (brief) VALUES ('draft without a segment yet')`); err != nil {
		t.Fatal(err)
	}
	expect(t, render(t, f.pool, f.now), map[string]float64{
		`smartcampaign_segment_outcomes_total{status="ok"}`:                  2,
		`smartcampaign_segment_outcomes_total{status="needs_clarification"}`: 1,
		`smartcampaign_segment_outcomes_total{status="unsupported"}`:         3,
	})
}

func TestHandlerServesTheTextFormat(t *testing.T) {
	f := setup(t)
	f.deliveries(t, "sms", "sent", 1, 0, 0, 1, 1000, time.Second)
	rec := httptest.NewRecorder()
	metrics.New(f.pool).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("content type %q", ct)
	}
	got := parse(t, rec.Body.String())
	if got[`smartcampaign_deliveries_total{channel="sms",status="sent"}`] != 1 {
		t.Fatalf("%v", got)
	}
}

// A broken database is a failed scrape (so the alert fires), never a page of
// zeros that looks like "nothing is happening".
func TestHandlerFailsLoudlyWhenTheDatabaseIsGone(t *testing.T) {
	f := setup(t)
	h := metrics.New(f.pool).Handler()
	f.pool.Close()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "smartcampaign_deliveries_total") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
