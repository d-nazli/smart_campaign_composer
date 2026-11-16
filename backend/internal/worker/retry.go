// Package worker sends the queued deliveries to the provider.
package worker

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"smartcampaign/internal/queue"
)

// Outcome is what one provider call produced. Err is set for transport
// problems (timeout, connection reset); Status is then 0.
type Outcome struct {
	Status     int
	RetryAfter time.Duration // from the Retry-After header, 0 when absent
	Err        error
	Detail     string // short, safe text for last_error (reason / error)
}

// Action is what to do with the delivery next.
type Action int

const (
	ActionSent Action = iota
	ActionRetry
	ActionFail
)

// Decision is the policy's answer.
type Decision struct {
	Action Action
	Kind   queue.RetryKind // for ActionRetry
	Delay  time.Duration   // for ActionRetry
	Reason string          // for ActionFail and ActionRetry (goes to last_error)
}

// RetryPolicy decides what a result means. It is a pure function of its inputs;
// the randomness is injected, so tests are exact.
type RetryPolicy struct {
	MaxAttempts       int           // failed attempts allowed before giving up (5)
	MaxAge            time.Duration // a delivery older than this is not retried (10 min)
	BaseDelay         time.Duration // first backoff ceiling (2s)
	MaxDelay          time.Duration // backoff ceiling cap (60s)
	DefaultRetryAfter time.Duration // used for a 429 without Retry-After (1s)
	Rand              func() float64
}

// Decide says what to do with a delivery after one provider call. attempts is
// the number of failed attempts so far (this one not included), age how long the
// delivery has existed.
func (p RetryPolicy) Decide(o Outcome, attempts int, age time.Duration) Decision {
	if o.Err == nil && o.Status >= 200 && o.Status < 300 {
		return Decision{Action: ActionSent}
	}
	what := describe(o)
	tooOld := Decision{Action: ActionFail, Reason: fmt.Sprintf("delivery too old (%s) to retry; last result: %s", age.Round(time.Second), what)}

	switch {
	case o.Err == nil && o.Status == http.StatusTooManyRequests:
		// "Slow down": wait as told, never use up an attempt (the message is fine).
		wait := o.RetryAfter
		if wait <= 0 {
			wait = p.DefaultRetryAfter
		}
		if age+wait > p.MaxAge {
			return tooOld
		}
		return Decision{Action: ActionRetry, Kind: queue.RetryRateLimited, Delay: wait, Reason: what}

	case o.Err != nil || o.Status >= 500 || o.Status == http.StatusRequestTimeout:
		// The provider may or may not have the message; the idempotency key makes a
		// repeat safe. Back off with full jitter, give up after MaxAttempts.
		if age > p.MaxAge {
			return tooOld
		}
		if attempts+1 >= p.MaxAttempts {
			return Decision{Action: ActionFail, Reason: fmt.Sprintf("gave up after %d attempts; last result: %s", attempts+1, what)}
		}
		return Decision{Action: ActionRetry, Kind: queue.RetryTransient, Delay: p.backoff(attempts), Reason: what}
	}
	// Any other answer (400, 422 invalid recipient, ...) will not change on retry.
	return Decision{Action: ActionFail, Reason: what}
}

// backoff is full jitter: uniform in [0, min(MaxDelay, BaseDelay*2^attempts)].
func (p RetryPolicy) backoff(attempts int) time.Duration {
	ceiling := p.MaxDelay
	if attempts < 30 { // beyond that the shift would overflow; the cap applies anyway
		if c := p.BaseDelay << uint(attempts); c > 0 && c < ceiling {
			ceiling = c
		}
	}
	r := rand.Float64
	if p.Rand != nil {
		r = p.Rand
	}
	return time.Duration(r() * float64(ceiling))
}

func describe(o Outcome) string {
	switch {
	case o.Err != nil && o.Detail != "":
		return o.Detail
	case o.Err != nil:
		return o.Err.Error()
	case o.Detail != "":
		return fmt.Sprintf("http %d %s", o.Status, o.Detail)
	}
	return fmt.Sprintf("http %d", o.Status)
}
