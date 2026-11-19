package worker_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"smartcampaign/internal/clock"
	"smartcampaign/internal/content"
	"smartcampaign/internal/queue"
	"smartcampaign/internal/testdb"
	"smartcampaign/internal/worker"
)

// countingSender accepts every message and counts them.
type countingSender struct{ n atomic.Int64 }

func (s *countingSender) Send(_ context.Context, _ string, _ worker.Message, key string) worker.Result {
	s.n.Add(1)
	return worker.Result{Outcome: worker.Outcome{Status: 202}, MessageID: "pm_" + key}
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The rate limit lives in each worker's own pacer, so two workers would send at
// twice the rate. Only the worker holding the send lock sends; the other waits
// and takes over when the first one stops.
func TestOnlyOneWorkerSendsAtATime(t *testing.T) {
	t.Parallel()
	pool := testdb.NewSeeded(t)
	sqlCampaign(t, pool, 30, 30)
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := worker.DefaultConfig()
	cfg.RatePerSecond = 1000
	cfg.Poll = 20 * time.Millisecond

	senders := [2]*countingSender{{}, {}}
	cancels := [2]context.CancelFunc{}
	done := [2]chan error{make(chan error, 1), make(chan error, 1)}
	for i := range senders {
		w := worker.New(queue.NewPG(pool), senders[i], policy, clock.Real{}, log, cfg)
		runCtx, cancel := context.WithCancel(ctx)
		cancels[i] = cancel
		go func() { done[i] <- w.RunExclusive(runCtx, pool, 50*time.Millisecond) }()
	}
	defer func() {
		for i := range cancels {
			cancels[i]()
		}
	}()

	waitFor(t, "the first campaign to be sent", 20*time.Second, func() bool { return unfinished(t, pool) == 0 })
	a, b := senders[0].n.Load(), senders[1].n.Load()
	if a+b != 60 || (a != 0 && b != 0) {
		t.Fatalf("both workers sent: %d and %d (want all 60 from one)", a, b)
	}
	winner, waiting := 0, 1
	if b > 0 {
		winner, waiting = 1, 0
	}

	// The sender stops; the waiting worker takes the lock and sends what comes next.
	cancels[winner]()
	select {
	case <-done[winner]:
	case <-time.After(10 * time.Second):
		t.Fatal("the sending worker did not stop")
	}
	sqlCampaign(t, pool, 5, 0)
	waitFor(t, "the waiting worker to take over", 20*time.Second, func() bool { return unfinished(t, pool) == 0 })
	if got := senders[waiting].n.Load(); got != 5 {
		t.Fatalf("the waiting worker sent %d (want 5)", got)
	}
}
