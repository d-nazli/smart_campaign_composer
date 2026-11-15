// Package clock is a tiny time abstraction.
//
// It exists ONLY for code whose behaviour depends on elapsed time and must be
// testable without real sleeping: the mock provider's rate limiter and
// latency simulation now, and the worker's rate limiter / retry backoff later.
// Do not spread it to unrelated code; everything else uses package time.
package clock

import (
	"context"
	"sync"
	"time"
)

// Clock reports the time and can wait.
type Clock interface {
	Now() time.Time
	// Sleep waits for d or until ctx is done, returning ctx.Err() in that case.
	Sleep(ctx context.Context, d time.Duration) error
}

// Real is the wall clock.
type Real struct{}

func (Real) Now() time.Time { return time.Now() }

func (Real) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Fake is a manually driven clock for tests. Sleep never blocks: it advances
// the fake time by d and returns immediately, so tests contain no real sleeps.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake starting at start.
func NewFake(start time.Time) *Fake { return &Fake{now: start} }

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the fake time forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}

func (f *Fake) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d > 0 {
		f.Advance(d)
	}
	return nil
}
