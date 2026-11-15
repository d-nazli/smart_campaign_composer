package provider

import (
	"sync"
	"time"

	"smartcampaign/internal/clock"
)

// slidingWindow admits at most `limit` requests in any window of length
// `window`. It is a strict sliding log (not a token bucket), so the provider
// can be used to prove that a client never exceeds the limit, even across
// window boundaries.
type slidingWindow struct {
	clk    clock.Clock
	limit  int
	window time.Duration

	mu      sync.Mutex
	admits  []time.Time // ascending
	maxSeen int         // highest number of admits ever inside one window
}

func newSlidingWindow(clk clock.Clock, limit int, window time.Duration) *slidingWindow {
	return &slidingWindow{clk: clk, limit: limit, window: window}
}

// allow admits one request if the window has room. Otherwise it returns how
// long until the oldest admitted request leaves the window.
func (w *slidingWindow) allow() (ok bool, retryAfter time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.clk.Now()
	cutoff := now.Add(-w.window)
	drop := 0
	for drop < len(w.admits) && !w.admits[drop].After(cutoff) {
		drop++
	}
	w.admits = w.admits[drop:]

	if len(w.admits) >= w.limit {
		return false, w.admits[0].Add(w.window).Sub(now)
	}
	w.admits = append(w.admits, now)
	if len(w.admits) > w.maxSeen {
		w.maxSeen = len(w.admits)
	}
	return true, 0
}

func (w *slidingWindow) max() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maxSeen
}
