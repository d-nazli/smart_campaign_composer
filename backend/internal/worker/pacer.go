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

	mu   sync.Mutex
	next time.Time // earliest start of the next slot
}

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
