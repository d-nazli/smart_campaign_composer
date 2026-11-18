package worker

import (
	"context"
	"sync"
	"time"

	"smartcampaign/internal/clock"
)

// Pacer hands out send slots at a strict, even rate (no bursts), shared by all
// senders of one channel.
//
// Each Wait reserves the next slot (the previous slot + interval, but never in
// the past) and sleeps until it. Slots are exactly interval apart, so in ANY
// one-second window there are at most perSecond of them. Idle time is not saved
// up: after a quiet minute the next slot is "now", not a burst of credits.
type Pacer struct {
	clk      clock.Clock
	interval time.Duration

	mu      sync.Mutex
	next    time.Time   // earliest start of the next slot
	limited []time.Time // recent 429s, ascending (see RateLimited)
}

// A lone 429 only delays its own message; the whole channel backs off when
// limitedBurst of them arrive within limitedWindow. Pacing stays under the
// provider's limit, so a stray 429 is noise (the mock injects 3% of them), while
// a burst means the provider really is refusing us. Pausing the channel for
// every single 429 held a 90/s channel at ~15/s.
const (
	limitedWindow = time.Second
	limitedBurst  = 10
)

// NewPacer paces perSecond sends per second. The interval is rounded UP to a
// whole nanosecond, so the real rate is never above perSecond.
func NewPacer(clk clock.Clock, perSecond int) *Pacer {
	if perSecond < 1 {
		perSecond = 1
	}
	n := time.Duration(perSecond)
	return &Pacer{clk: clk, interval: (time.Second + n - 1) / n}
}

// Wait blocks until the caller may send, or the context ends.
func (p *Pacer) Wait(ctx context.Context) error {
	p.mu.Lock()
	now := p.clk.Now()
	slot := p.next
	if slot.Before(now) {
		slot = now
	}
	p.next = slot.Add(p.interval)
	p.mu.Unlock()
	return p.clk.Sleep(ctx, slot.Sub(now))
}

// PauseUntil makes every later slot start at or after t (used after a 429). An
// earlier deadline never shortens a later one.
func (p *Pacer) PauseUntil(t time.Time) {
	p.mu.Lock()
	if t.After(p.next) {
		p.next = t
	}
	p.mu.Unlock()
}

// RateLimited records a 429 whose Retry-After ends at t. It pauses the channel
// (PauseUntil) only when 429s come in a burst; it reports whether it did.
func (p *Pacer) RateLimited(t time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.clk.Now()
	p.limited = append(p.limited, now)
	drop := 0
	for drop < len(p.limited) && now.Sub(p.limited[drop]) >= limitedWindow {
		drop++
	}
	p.limited = p.limited[drop:]
	if len(p.limited) < limitedBurst {
		return false
	}
	if t.After(p.next) {
		p.next = t
	}
	return true
}
