package worker_test

import (
	"context"
	"testing"
	"time"

	"smartcampaign/internal/clock"
	"smartcampaign/internal/content"
	"smartcampaign/internal/queue"
	"smartcampaign/internal/testdb"
	"smartcampaign/internal/worker"
)

// heldSender holds every send until released, like a provider that is slow to
// answer; a send whose context ends first comes back as a transport error, the
// way the HTTP client reports it.
type heldSender struct {
	started chan struct{}
	release chan struct{}
}

func (s *heldSender) Send(ctx context.Context, _ string, _ worker.Message, key string) worker.Result {
	s.started <- struct{}{}
	select {
	case <-s.release:
		return worker.Result{Outcome: worker.Outcome{Status: 202}, MessageID: "pm_" + key}
	case <-ctx.Done():
		return worker.Result{Outcome: worker.Outcome{Err: ctx.Err(), Detail: "interrupted"}}
	}
}

func shutdownWorker(t *testing.T, grace time.Duration) (*worker.Worker, *heldSender, func() int) {
	t.Helper()
	pool := testdb.NewSeeded(t)
	id := sqlCampaign(t, pool, 3, 0)
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := worker.DefaultConfig()
	cfg.Concurrency, cfg.BatchSize = 1, 1
	cfg.ShutdownGrace = grace
	s := &heldSender{started: make(chan struct{}, 10), release: make(chan struct{})}
	w := worker.New(queue.NewPG(pool), s, policy, clock.Real{}, log, cfg)
	sent := func() int { return counts(t, pool, id)["sms/sent"] }
	return w, s, sent
}

// Stopping the worker (SIGTERM) while a message is with the provider: no new
// row is claimed, the answer that arrives is recorded, then Run returns.
func TestShutdownRecordsTheSendsUnderWay(t *testing.T) {
	t.Parallel()
	w, s, sent := shutdownWorker(t, 10*time.Second)
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- w.Run(runCtx) }()

	<-s.started // one message is with the provider
	stop()      // SIGTERM
	select {
	case err := <-done:
		t.Fatalf("Run returned (%v) while a send was under way", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(s.release) // the provider answers: accepted
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the send finished")
	}
	if got := sent(); got != 1 {
		t.Fatalf("the accepted message must be recorded as sent (and nothing else sent): %d sent", got)
	}
	if n := len(s.started); n != 0 {
		t.Fatalf("%d new sends started after the stop", n)
	}
}

// A provider that never answers does not hold the shutdown longer than the grace.
func TestShutdownWaitsAtMostTheGrace(t *testing.T) {
	t.Parallel()
	w, s, sent := shutdownWorker(t, 300*time.Millisecond)
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- w.Run(runCtx) }()
	<-s.started
	began := time.Now()
	stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run waited past the grace")
	}
	if waited := time.Since(began); waited < 250*time.Millisecond {
		t.Fatalf("Run returned after %v, before the grace was over", waited)
	}
	if got := sent(); got != 0 {
		t.Fatalf("an unanswered send was recorded as sent: %d", got)
	}
}
