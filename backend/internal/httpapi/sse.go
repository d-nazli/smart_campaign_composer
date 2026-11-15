package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const keepAliveEvery = 15 * time.Second

// sse writes Server-Sent Events. Headers are sent lazily with the first event,
// so a handler that fails before producing anything can still answer with a
// normal JSON error and status code.
type sse struct {
	w   http.ResponseWriter
	rc  *http.ResponseController
	ctx context.Context

	mu      sync.Mutex
	started bool
	closed  bool
	stop    chan struct{}
}

func newSSE(w http.ResponseWriter, ctx context.Context) *sse {
	return &sse{w: w, rc: http.NewResponseController(w), ctx: ctx, stop: make(chan struct{})}
}

func (s *sse) start() {
	h := s.w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // tell nginx not to buffer this response
	s.w.WriteHeader(http.StatusOK)
	s.started = true
	go s.keepAlive()
}

// send writes one event and flushes it immediately.
func (s *sse) send(event string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if !s.started {
		s.start()
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return
	}
	_ = s.rc.Flush()
}

// keepAlive sends a comment line so proxies do not close an idle stream while
// the model is thinking.
func (s *sse) keepAlive() {
	t := time.NewTicker(keepAliveEvery)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.mu.Lock()
			if !s.closed {
				_, _ = fmt.Fprint(s.w, ": keepalive\n\n")
				_ = s.rc.Flush()
			}
			s.mu.Unlock()
		}
	}
}

func (s *sse) close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	close(s.stop)
}
