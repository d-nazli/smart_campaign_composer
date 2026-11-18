package worker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"smartcampaign/internal/clock"
	"smartcampaign/internal/worker"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// maxInAnyWindow is the largest number of events inside any half-open window
// [t, t+1s): the number the provider's own limiter would see.
func maxInAnyWindow(times []time.Time) int {
	best, lo := 0, 0
	for hi := range times {
		for times[hi].Sub(times[lo]) >= time.Second {
			lo++
		}
		if n := hi - lo + 1; n > best {
			best = n
		}
	}
	return best
}

// waits takes n slots one after another and returns the (fake) time of each.
func waits(t *testing.T, p *worker.Pacer, clk *clock.Fake, n int) []time.Time {
	t.Helper()
	out := make([]time.Time, n)
	for i := range out {
		if err := p.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
		out[i] = clk.Now()
	}
	return out
}

// The point of the pacer: whatever the rate, no 1-second window ever holds
// more sends than the rate. Strict, not "on average".
func TestNoOneSecondWindowEverExceedsTheRate(t *testing.T) {
	for _, rate := range []int{1, 7, 90, 100, 333} {
		clk := clock.NewFake(t0)
		p := worker.NewPacer(clk, rate)
		times := waits(t, p, clk, 3*rate+20)
		if got := maxInAnyWindow(times); got > rate {
			t.Errorf("rate %d: %d sends inside one second", rate, got)
		}
		// ...and it is not needlessly slow: it uses (nearly) the whole allowance.
		if got := maxInAnyWindow(times); got < rate-1 {
			t.Errorf("rate %d: only %d sends in the busiest second; the pacer is too slow", rate, got)
		}
	}
}

func TestTheFirstSlotIsImmediate(t *testing.T) {
	clk := clock.NewFake(t0)
	p := worker.NewPacer(clk, 90)
	if got := waits(t, p, clk, 1)[0]; !got.Equal(t0) {
		t.Fatalf("first wait moved the clock to %v", got)
	}
}

// Idle time must not be saved up as a burst allowance.
func TestIdleTimeDoesNotBuildUpABurst(t *testing.T) {
	clk := clock.NewFake(t0)
	p := worker.NewPacer(clk, 90)
	waits(t, p, clk, 10)
	clk.Advance(time.Minute)
	times := waits(t, p, clk, 400)
	if got := maxInAnyWindow(times); got > 90 {
		t.Fatalf("%d sends in one second after an idle minute", got)
	}
}

// A 429 pauses the whole channel until Retry-After is over.
func TestPauseUntilHoldsBackEverySlotBeforeTheDeadline(t *testing.T) {
	clk := clock.NewFake(t0)
	p := worker.NewPacer(clk, 90)
	waits(t, p, clk, 5)
	resume := clk.Now().Add(7 * time.Second)
	p.PauseUntil(resume)

	times := waits(t, p, clk, 200)
	if times[0].Before(resume) {
		t.Fatalf("sent at %v, before the pause ended at %v", times[0], resume)
	}
	if got := maxInAnyWindow(times); got > 90 {
		t.Fatalf("burst after the pause: %d in one second", got)
	}
}

func TestAnEarlierPauseNeverShortensALaterOne(t *testing.T) {
	clk := clock.NewFake(t0)
	p := worker.NewPacer(clk, 90)
	p.PauseUntil(t0.Add(10 * time.Second))
	p.PauseUntil(t0.Add(2 * time.Second))
	if got := waits(t, p, clk, 1)[0]; got.Before(t0.Add(10 * time.Second)) {
		t.Fatalf("resumed too early: %v", got)
	}
}

func TestCancelledWaitReturnsTheContextError(t *testing.T) {
	clk := clock.NewFake(t0)
	p := worker.NewPacer(clk, 1)
	waits(t, p, clk, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Wait(ctx); err == nil {
		t.Fatal("a cancelled wait must fail so the worker stops sending")
	}
}

// Many senders share one pacer; the total pace is still the configured one.
func TestConcurrentSendersShareOnePace(t *testing.T) {
	const rate, senders, each = 200, 8, 25 // 200 sends at 200/s: about one second
	p := worker.NewPacer(clock.Real{}, rate)
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < each; j++ {
				if err := p.Wait(context.Background()); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	// 200 slots at 5ms apart need at least 199*5ms = 995ms.
	if elapsed := time.Since(start); elapsed < 950*time.Millisecond {
		t.Fatalf("200 sends took only %v; the pacer let them through too fast", elapsed)
	}
}

func TestAFewStray429sDoNotPauseTheChannel(t *testing.T) {
	clk := clock.NewFake(t0)
	p := worker.NewPacer(clk, 90)
	for i := 0; i < 9; i++ { // one short of a burst
		if p.RateLimited(clk.Now().Add(3 * time.Second)) {
			t.Fatalf("paused after %d 429s", i+1)
		}
	}
	if got := waits(t, p, clk, 1)[0]; !got.Equal(t0) {
		t.Fatalf("the next slot was held back to %v", got)
	}
}

func TestABurstOf429sPausesTheChannelForRetryAfter(t *testing.T) {
	clk := clock.NewFake(t0)
	p := worker.NewPacer(clk, 90)
	resume := t0.Add(3 * time.Second)
	paused := false
	for i := 0; i < 10; i++ {
		paused = p.RateLimited(resume)
	}
	if !paused {
		t.Fatal("ten 429s in one second did not pause the channel")
	}
	if got := waits(t, p, clk, 1)[0]; got.Before(resume) {
		t.Fatalf("sent at %v, before the pause ended at %v", got, resume)
	}
}

func TestOld429sDoNotAddUpToABurst(t *testing.T) {
	clk := clock.NewFake(t0)
	p := worker.NewPacer(clk, 90)
	for i := 0; i < 9; i++ {
		p.RateLimited(clk.Now().Add(time.Second))
		clk.Advance(200 * time.Millisecond) // 5 per second at most
	}
	if p.RateLimited(clk.Now().Add(time.Second)) {
		t.Fatal("429s spread over two seconds were treated as a burst")
	}
}
