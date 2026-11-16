// Package metrics renders Prometheus metrics from the database.
//
// Everything is read from the tables the system already writes (deliveries,
// llm_generations, campaigns) at scrape time. That keeps the api and the worker
// independent processes, makes the numbers survive restarts, and means there is
// one source of truth. The text format is written by hand: it is small, and a
// test checks it against the exposition rules.
package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// sendRateWindow is how far back "sends per second" looks.
const sendRateWindow = 10 * time.Second

var (
	channels = []string{"sms", "email"}
	statuses = []string{"sent", "failed", "skipped"}
	kinds    = []string{"segment", "messages"}
	outcomes = []string{"ok", "needs_clarification", "unsupported"}
)

// Querier is satisfied by *pgxpool.Pool.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Collector reads the numbers on every scrape.
type Collector struct {
	db  Querier
	now func() time.Time
	log *slog.Logger
}

func New(db Querier) *Collector { return &Collector{db: db, now: time.Now, log: slog.Default()} }

// NewAt is New with an injectable clock (tests).
func NewAt(db Querier, now func() time.Time) *Collector {
	return &Collector{db: db, now: now, log: slog.Default()}
}

// WithLogger sets where scrape failures are logged.
func (c *Collector) WithLogger(l *slog.Logger) *Collector { c.log = l; return c }

type deliveryTotals struct {
	count, retries, limited, calls, micros, recent int64
}

// Render collects the current numbers in Prometheus text format.
func (c *Collector) Render(ctx context.Context) (string, error) {
	now := c.now()

	// ---- deliveries ----
	byStatus := map[[2]string]*deliveryTotals{}
	rows, err := c.db.Query(ctx, `
		SELECT channel, status, count(*), coalesce(sum(retries), 0)::bigint, coalesce(sum(rate_limited), 0)::bigint,
		       coalesce(sum(provider_calls), 0)::bigint, coalesce(sum(provider_micros), 0)::bigint,
		       count(*) FILTER (WHERE status = 'sent' AND sent_at > $1 AND sent_at <= $2)
		FROM deliveries GROUP BY channel, status`, now.Add(-sendRateWindow), now)
	if err != nil {
		return "", fmt.Errorf("deliveries: %w", err)
	}
	for rows.Next() {
		var ch, st string
		var d deliveryTotals
		if err := rows.Scan(&ch, &st, &d.count, &d.retries, &d.limited, &d.calls, &d.micros, &d.recent); err != nil {
			rows.Close()
			return "", fmt.Errorf("deliveries: %w", err)
		}
		byStatus[[2]string{ch, st}] = &d
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("deliveries: %w", err)
	}

	// ---- LLM attempts ----
	type llmTotals struct{ runs, firstValid, repairs, fallbacks, refusals, attempts, ms, in, out int64 }
	byKind := map[string]*llmTotals{}
	rows, err = c.db.Query(ctx, `
		SELECT kind,
		       count(*) FILTER (WHERE attempt = 1),
		       count(*) FILTER (WHERE attempt = 1 AND valid),
		       count(*) FILTER (WHERE attempt > 1),
		       count(*) FILTER (WHERE fell_back),
		       count(*) FILTER (WHERE refused),
		       count(*), coalesce(sum(latency_ms), 0)::bigint,
		       coalesce(sum(input_tokens), 0)::bigint, coalesce(sum(output_tokens), 0)::bigint
		FROM llm_generations GROUP BY kind`)
	if err != nil {
		return "", fmt.Errorf("llm_generations: %w", err)
	}
	for rows.Next() {
		var k string
		var l llmTotals
		if err := rows.Scan(&k, &l.runs, &l.firstValid, &l.repairs, &l.fallbacks, &l.refusals, &l.attempts, &l.ms, &l.in, &l.out); err != nil {
			rows.Close()
			return "", fmt.Errorf("llm_generations: %w", err)
		}
		byKind[k] = &l
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("llm_generations: %w", err)
	}

	// ---- segment outcomes ----
	segments := map[string]int64{}
	rows, err = c.db.Query(ctx, `SELECT segment_status, count(*) FROM campaigns WHERE segment_status IS NOT NULL GROUP BY 1`)
	if err != nil {
		return "", fmt.Errorf("campaigns: %w", err)
	}
	for rows.Next() {
		var st string
		var n int64
		if err := rows.Scan(&st, &n); err != nil {
			rows.Close()
			return "", fmt.Errorf("campaigns: %w", err)
		}
		segments[st] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("campaigns: %w", err)
	}

	// ---- write ----
	var e exposition
	sum := func(ch string, f func(*deliveryTotals) int64, only ...string) int64 {
		var total int64
		for _, st := range []string{"pending", "sending", "sent", "failed", "skipped"} {
			if len(only) > 0 && !contains(only, st) {
				continue
			}
			if d := byStatus[[2]string{ch, st}]; d != nil {
				total += f(d)
			}
		}
		return total
	}

	e.family("smartcampaign_deliveries_total", "counter", "Deliveries that reached a final state, by channel and status.")
	for _, ch := range channels {
		for _, st := range statuses {
			var n int64
			if d := byStatus[[2]string{ch, st}]; d != nil {
				n = d.count
			}
			e.sample("smartcampaign_deliveries_total", n, "channel", ch, "status", st)
		}
	}
	e.family("smartcampaign_delivery_queue_length", "gauge", "Deliveries waiting to be sent or being sent right now (pending, retrying, in flight).")
	for _, ch := range channels {
		e.sample("smartcampaign_delivery_queue_length", sum(ch, func(d *deliveryTotals) int64 { return d.count }, "pending", "sending"), "channel", ch)
	}
	e.family("smartcampaign_delivery_retries_total", "counter", "Times a delivery went back to the queue after a failed or refused attempt (429s included).")
	for _, ch := range channels {
		e.sample("smartcampaign_delivery_retries_total", sum(ch, func(d *deliveryTotals) int64 { return d.retries }), "channel", ch)
	}
	e.family("smartcampaign_delivery_rate_limited_total", "counter", "Retries caused by an HTTP 429 from the provider (a subset of retries).")
	for _, ch := range channels {
		e.sample("smartcampaign_delivery_rate_limited_total", sum(ch, func(d *deliveryTotals) int64 { return d.limited }), "channel", ch)
	}
	e.family("smartcampaign_send_rate_per_second", "gauge", fmt.Sprintf("Messages accepted by the provider per second, averaged over the last %d seconds.", int(sendRateWindow.Seconds())))
	for _, ch := range channels {
		recent := sum(ch, func(d *deliveryTotals) int64 { return d.recent }, "sent")
		e.sampleF("smartcampaign_send_rate_per_second", float64(recent)/sendRateWindow.Seconds(), "channel", ch)
	}
	e.family("smartcampaign_provider_response_seconds", "summary", "Time the provider took to answer a send call (every call, retries included).")
	for _, ch := range channels {
		e.sampleF("smartcampaign_provider_response_seconds_sum", float64(sum(ch, func(d *deliveryTotals) int64 { return d.micros }))/1e6, "channel", ch)
		e.sample("smartcampaign_provider_response_seconds_count", sum(ch, func(d *deliveryTotals) int64 { return d.calls }), "channel", ch)
	}

	get := func(k string) llmTotals {
		if l := byKind[k]; l != nil {
			return *l
		}
		return llmTotals{}
	}
	e.family("smartcampaign_llm_generations_total", "counter", "LLM generation runs (a run is one first attempt plus its repairs), by kind.")
	for _, k := range kinds {
		e.sample("smartcampaign_llm_generations_total", get(k).runs, "kind", k)
	}
	e.family("smartcampaign_llm_first_attempt_valid_total", "counter", "Runs whose first attempt was already valid.")
	for _, k := range kinds {
		e.sample("smartcampaign_llm_first_attempt_valid_total", get(k).firstValid, "kind", k)
	}
	e.family("smartcampaign_llm_repair_attempts_total", "counter", "Extra attempts made by the repair loop after an invalid or failed one.")
	for _, k := range kinds {
		e.sample("smartcampaign_llm_repair_attempts_total", get(k).repairs, "kind", k)
	}
	e.family("smartcampaign_llm_fallbacks_total", "counter", "Message runs that ended with the built-in template instead of model output.")
	for _, k := range kinds {
		e.sample("smartcampaign_llm_fallbacks_total", get(k).fallbacks, "kind", k)
	}
	e.family("smartcampaign_llm_refusals_total", "counter", "Attempts where the model declined to answer.")
	for _, k := range kinds {
		e.sample("smartcampaign_llm_refusals_total", get(k).refusals, "kind", k)
	}
	e.family("smartcampaign_llm_attempt_seconds", "summary", "Duration of LLM attempts (average = sum / count).")
	for _, k := range kinds {
		e.sampleF("smartcampaign_llm_attempt_seconds_sum", float64(get(k).ms)/1e3, "kind", k)
		e.sample("smartcampaign_llm_attempt_seconds_count", get(k).attempts, "kind", k)
	}
	e.family("smartcampaign_llm_tokens_total", "counter", "Tokens spent on LLM attempts.")
	for _, k := range kinds {
		e.sample("smartcampaign_llm_tokens_total", get(k).in, "kind", k, "direction", "input")
		e.sample("smartcampaign_llm_tokens_total", get(k).out, "kind", k, "direction", "output")
	}
	e.family("smartcampaign_segment_outcomes_total", "counter", "Campaigns by what became of their brief: ok, needs_clarification or unsupported.")
	for _, st := range outcomes {
		e.sample("smartcampaign_segment_outcomes_total", segments[st], "status", st)
	}
	return e.String(), nil
}

// Handler serves the metrics. If the database cannot be read the scrape FAILS
// (503); it never answers with zeros that would look like "all quiet".
func (c *Collector) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		text, err := c.Render(ctx)
		if err != nil {
			if r.Context().Err() == nil {
				c.log.Error("metrics scrape failed", "error", err)
			}
			http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(text))
	})
}

// ---- text format ----------------------------------------------------------------

type exposition struct{ b strings.Builder }

func (e *exposition) String() string { return e.b.String() }

func (e *exposition) family(name, typ, help string) {
	fmt.Fprintf(&e.b, "# HELP %s %s\n# TYPE %s %s\n", name, escapeHelp(help), name, typ)
}

func (e *exposition) sample(name string, v int64, labels ...string) {
	e.write(name, strconv.FormatInt(v, 10), labels)
}

func (e *exposition) sampleF(name string, v float64, labels ...string) {
	e.write(name, strconv.FormatFloat(v, 'g', -1, 64), labels)
}

func (e *exposition) write(name, value string, labels []string) {
	e.b.WriteString(name)
	for i := 0; i+1 < len(labels); i += 2 {
		sep := ","
		if i == 0 {
			sep = "{"
		}
		fmt.Fprintf(&e.b, `%s%s="%s"`, sep, labels[i], escapeLabel(labels[i+1]))
	}
	if len(labels) > 0 {
		e.b.WriteString("}")
	}
	e.b.WriteString(" " + value + "\n")
}

func escapeHelp(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`).Replace(s)
}

func escapeLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
