package worker_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"smartcampaign/internal/queue"
	"smartcampaign/internal/worker"
)

func policy(rnd float64) worker.RetryPolicy {
	return worker.RetryPolicy{
		MaxAttempts: 5, MaxAge: 10 * time.Minute,
		BaseDelay: 2 * time.Second, MaxDelay: 60 * time.Second, DefaultRetryAfter: time.Second,
		Rand: func() float64 { return rnd },
	}
}

func TestSuccessIsSent(t *testing.T) {
	t.Parallel()
	for _, status := range []int{200, 201, 202} {
		if d := policy(0.5).Decide(worker.Outcome{Status: status}, 0, 0); d.Action != worker.ActionSent {
			t.Errorf("%d: %+v", status, d)
		}
	}
}

// 429 means "slow down", not "this message is bad": it costs no attempt.
func TestRateLimitWaitsForRetryAfterAndNeverUsesUpAttempts(t *testing.T) {
	t.Parallel()
	p := policy(0.5)
	for _, attempts := range []int{0, 4, 40} {
		d := p.Decide(worker.Outcome{Status: 429, RetryAfter: 3 * time.Second}, attempts, time.Minute)
		if d.Action != worker.ActionRetry || d.Kind != queue.RetryRateLimited || d.Delay != 3*time.Second {
			t.Errorf("attempts=%d: %+v", attempts, d)
		}
	}
	if d := p.Decide(worker.Outcome{Status: 429}, 0, 0); d.Delay != time.Second {
		t.Errorf("429 without Retry-After should wait the default, got %+v", d)
	}
}

func TestRateLimitedDeliveryEventuallyGivesUpByAge(t *testing.T) {
	t.Parallel()
	p := policy(0.5)
	old := p.Decide(worker.Outcome{Status: 429, RetryAfter: time.Second}, 0, 11*time.Minute)
	if old.Action != worker.ActionFail || !strings.Contains(old.Reason, "too old") {
		t.Errorf("older than the limit: %+v", old)
	}
	// Still young, but the wait itself would cross the limit.
	late := p.Decide(worker.Outcome{Status: 429, RetryAfter: 30 * time.Second}, 0, 9*time.Minute+45*time.Second)
	if late.Action != worker.ActionFail {
		t.Errorf("the wait would exceed the age limit: %+v", late)
	}
	ok := p.Decide(worker.Outcome{Status: 429, RetryAfter: 5 * time.Second}, 0, 9*time.Minute)
	if ok.Action != worker.ActionRetry {
		t.Errorf("young enough: %+v", ok)
	}
}

func TestServerErrorsAndTransportErrorsBackOffWithFullJitter(t *testing.T) {
	t.Parallel()
	outcomes := map[string]worker.Outcome{
		"500":     {Status: 500},
		"503":     {Status: 503},
		"504":     {Status: 504},
		"408":     {Status: 408},
		"timeout": {Err: errors.New("context deadline exceeded"), Detail: "timeout"},
	}
	for name, o := range outcomes {
		// The ceiling doubles with each failed attempt: 2s, 4s, 8s, 16s.
		for attempts, ceiling := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second} {
			top := policy(1).Decide(o, attempts, time.Minute)
			if top.Action != worker.ActionRetry || top.Kind != queue.RetryTransient || top.Delay != ceiling {
				t.Errorf("%s attempts=%d: want retry after %v, got %+v", name, attempts, ceiling, top)
			}
			half := policy(0.5).Decide(o, attempts, time.Minute)
			if half.Delay != ceiling/2 {
				t.Errorf("%s attempts=%d: jitter should scale the ceiling, got %v", name, attempts, half.Delay)
			}
			if zero := policy(0).Decide(o, attempts, time.Minute); zero.Delay != 0 {
				t.Errorf("full jitter may be 0, got %v", zero.Delay)
			}
		}
	}
}

func TestTheFifthFailureIsFinal(t *testing.T) {
	t.Parallel()
	d := policy(0.5).Decide(worker.Outcome{Status: 503, Detail: "injected"}, 4, time.Minute)
	if d.Action != worker.ActionFail || !strings.Contains(d.Reason, "5") || !strings.Contains(d.Reason, "503") {
		t.Fatalf("%+v", d)
	}
}

func TestBackoffCeilingIsCapped(t *testing.T) {
	t.Parallel()
	p := policy(1)
	p.MaxAttempts = 100
	if d := p.Decide(worker.Outcome{Status: 500}, 50, time.Minute); d.Delay != 60*time.Second {
		t.Fatalf("delay = %v, want the 60s cap (and no overflow)", d.Delay)
	}
}

// Retrying a bad address can never help, so it must not be retried.
func TestOtherClientErrorsAreFinalAndNotRetried(t *testing.T) {
	t.Parallel()
	for _, status := range []int{400, 401, 403, 404, 410, 422} {
		d := policy(0.5).Decide(worker.Outcome{Status: status, Detail: "invalid_recipient"}, 0, 0)
		if d.Action != worker.ActionFail || !strings.Contains(d.Reason, "invalid_recipient") {
			t.Errorf("%d: %+v", status, d)
		}
	}
}

func TestTooOldBeatsRetryForTransientErrorsToo(t *testing.T) {
	t.Parallel()
	d := policy(0.5).Decide(worker.Outcome{Status: 503}, 1, 11*time.Minute)
	if d.Action != worker.ActionFail || !strings.Contains(d.Reason, "too old") {
		t.Fatalf("%+v", d)
	}
}

func TestDefaultRandomnessStaysInsideTheCeiling(t *testing.T) {
	t.Parallel()
	p := policy(0)
	p.Rand = nil
	for i := 0; i < 200; i++ {
		d := p.Decide(worker.Outcome{Status: 502}, 2, time.Minute)
		if d.Action != worker.ActionRetry || d.Delay < 0 || d.Delay > 8*time.Second {
			t.Fatalf("%+v", d)
		}
	}
}
