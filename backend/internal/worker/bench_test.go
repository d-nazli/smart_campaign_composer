//go:build bench

// Throughput benchmark of the worker against the mock provider (real clock,
// both channels at once). Not part of `make test`: run it with `make bench`.
// It uses throwaway test databases only, so it leaves nothing behind.
//
// Environment (optional):
//
//	BENCH_RATES     worker paces to try, comma separated (default: the worker default, 97)
//	BENCH_MESSAGES  messages per channel (default 800)
package worker_test

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"smartcampaign/internal/clock"
	"smartcampaign/internal/provider"
	"smartcampaign/internal/queue"
	"smartcampaign/internal/testdb"
	"smartcampaign/internal/worker"
)

type benchResult struct {
	calls, accepted float64 // per second, over 4 s of steady state
	peak            int     // most calls the provider saw inside one second
	real429         int     // refusals by the provider's real limit
}

// measureChannels sends perChannel messages on both channels through the real
// worker and mock provider and measures each channel.
func measureChannels(t *testing.T, cfg provider.Config, rate, perChannel int) map[string]benchResult {
	t.Helper()
	pool := testdb.NewSeeded(t)
	id := sqlCampaign(t, pool, perChannel, perChannel)
	if c := counts(t, pool, id); c["sms/pending"] < perChannel || c["email/pending"] < perChannel {
		t.Skipf("seed has too few reachable recipients: %v", c)
	}
	tp := newProvider(t, cfg, clock.Real{})
	wcfg := worker.DefaultConfig()
	wcfg.RatePerSecond = rate
	w := newWorker(t, queue.NewPG(pool), tp.URL, clock.Real{}, wcfg)

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- w.Run(runCtx) }()
	deadline := time.Now().Add(90 * time.Second)
	for scalar(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id=$1 AND status IN ('pending','sending')`, id) > 0 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("not finished in 90s: %v", counts(t, pool, id))
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	<-done

	byChannel := map[string][]time.Time{}
	for _, r := range tp.requests() {
		byChannel[r.Channel] = append(byChannel[r.Channel], r.At)
	}
	out := map[string]benchResult{}
	for _, ch := range []string{"sms", "email"} {
		times := byChannel[ch]
		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
		// Steady state: 4 s that start 1 s after the first call (the end of the
		// campaign is a trickle of retries and must not count).
		from, to := times[0].Add(time.Second), times[0].Add(5*time.Second)
		n := 0
		for _, at := range times {
			if !at.Before(from) && at.Before(to) {
				n++
			}
		}
		sent := scalar(t, pool, `SELECT count(*) FROM deliveries WHERE campaign_id=$1 AND channel=$2 AND status='sent' AND sent_at >= $3 AND sent_at < $4`, id, ch, from, to)
		out[ch] = benchResult{calls: float64(n) / 4, accepted: float64(sent) / 4, peak: maxInAnyWindow(times), real429: tp.stats(ch).RateLimited}
	}
	return out
}

func envInts(key string, def []int) []int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	var out []int
	for _, s := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 {
			panic(fmt.Sprintf("%s: %q is not a positive integer", key, s))
		}
		out = append(out, n)
	}
	return out
}

// TestBenchThroughput prints, per pace and provider, the calls and accepted
// messages per second of each channel, and fails if a one-second window ever
// held more than 100 calls of a channel or the provider's real limit refused one.
func TestBenchThroughput(t *testing.T) {
	rates := envInts("BENCH_RATES", []int{worker.DefaultConfig().RatePerSecond})
	perChannel := envInts("BENCH_MESSAGES", []int{800})[0]

	quiet := quiet()
	quiet.Latency, quiet.LatencyJitter = 50*time.Millisecond, 100*time.Millisecond
	noisy := provider.DefaultConfig() // the demo's provider: 5% 5xx, 3% 429, 1% hang, 2% accepted-then-error
	noisy.TimeoutDelay = 6 * time.Second
	noisy.Seed = 7
	providers := []struct {
		name string
		cfg  provider.Config
	}{{"hatasız", quiet}, {"varsayılan kaos", noisy}}

	var lines []string
	for _, rate := range rates {
		for _, p := range providers {
			// One subtest each, so its database is dropped before the next one.
			t.Run(fmt.Sprintf("rate=%d/%s", rate, p.name), func(t *testing.T) {
				res := measureChannels(t, p.cfg, rate, perChannel)
				for _, ch := range []string{"sms", "email"} {
					r := res[ch]
					lines = append(lines, fmt.Sprintf("%-5d %-16s %-6s %8.1f %8.1f %6d %8d", rate, p.name, ch, r.calls, r.accepted, r.peak, r.real429))
					if r.peak > 100 || r.real429 > 0 {
						t.Errorf("%s: %d calls inside one second, %d real 429s: the 100/s rule was broken", ch, r.peak, r.real429)
					}
				}
			})
		}
	}
	fmt.Printf("\npace  provider         kanal  çağrı/sn  kabul/sn  pik/1sn  gerçek429\n")
	for _, l := range lines {
		fmt.Println(l)
	}
	fmt.Println()
}
