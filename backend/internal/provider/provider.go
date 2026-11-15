package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"smartcampaign/internal/clock"
)

const (
	ChannelSMS   = "sms"
	ChannelEmail = "email"

	headerIdempotencyKey = "Idempotency-Key"
	headerReplay         = "Idempotent-Replay"

	maxBodyBytes = 64 << 10
	maxKeyLength = 200
	rateWindow   = time.Second
)

// Reasons attached to every non-2xx response and written to the log.
const (
	ReasonMissingKey      = "missing_idempotency_key"
	ReasonInvalidPayload  = "invalid_payload"
	ReasonInvalidReceiver = "invalid_recipient"
	ReasonKeyReused       = "idempotency_key_reused"
	ReasonRateLimit       = "rate_limit"        // real limit exceeded
	ReasonInjected        = "injected"          // chaos: failed before accepting
	ReasonTimeout         = "timeout"           // chaos: answered only after a long delay
	ReasonFailAfterAccept = "fail_after_accept" // chaos: message recorded, response is an error
	ReasonReplay          = "replay"            // not an error: duplicate key answered from memory
)

var (
	phoneRe = regexp.MustCompile(`^\+90[0-9]{10}$`)
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// ChannelStats are in-memory counters, read directly by tests via Stats().
// There is intentionally no HTTP endpoint for them.
type ChannelStats struct {
	Received          int // every request that reached the handler
	BadRequests       int
	RateLimited       int // 429, real limit exceeded
	Injected429       int
	Injected5xx       int
	Timeouts          int
	FailedAfterAccept int // recorded as delivered AND answered with 5xx
	Replays           int // duplicate Idempotency-Key answered from memory
	UniqueDeliveries  int // distinct keys that were accepted: the number that matters
	MaxPerWindow      int // most requests ever admitted inside one rate window
}

type entry struct {
	mu          sync.Mutex
	done        bool
	fingerprint string
	messageID   string
}

// Server is the mock provider.
type Server struct {
	cfg Config
	clk clock.Clock
	log *slog.Logger

	rngMu sync.Mutex
	rng   *rand.Rand

	limiters map[string]*slidingWindow

	mu      sync.Mutex
	entries map[string]*entry
	stats   map[string]*ChannelStats
}

// New builds a provider. clk drives the rate limiter and every simulated
// delay, so tests can use clock.Fake and never sleep for real.
func New(cfg Config, clk clock.Clock, log *slog.Logger) *Server {
	s := &Server{
		cfg:      cfg,
		clk:      clk,
		log:      log,
		rng:      rand.New(rand.NewPCG(cfg.Seed, 0xda942042e4dd58b5)),
		limiters: map[string]*slidingWindow{},
		entries:  map[string]*entry{},
		stats:    map[string]*ChannelStats{},
	}
	for _, ch := range []string{ChannelSMS, ChannelEmail} {
		s.limiters[ch] = newSlidingWindow(clk, cfg.RateLimitPerSecond, rateWindow)
		s.stats[ch] = &ChannelStats{}
	}
	return s
}

// Handler exposes exactly three routes: the two send endpoints and /healthz.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/sms/send", s.handleSend(ChannelSMS))
	mux.HandleFunc("POST /v1/email/send", s.handleSend(ChannelEmail))
	return mux
}

// Stats returns a snapshot of the counters per channel.
func (s *Server) Stats() map[string]ChannelStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]ChannelStats, len(s.stats))
	for ch, st := range s.stats {
		c := *st
		c.MaxPerWindow = s.limiters[ch].max()
		out[ch] = c
	}
	return out
}

type sendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *Server) bump(channel string, f func(*ChannelStats)) {
	s.mu.Lock()
	f(s.stats[channel])
	s.mu.Unlock()
}

func (s *Server) handleSend(channel string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.bump(channel, func(c *ChannelStats) { c.Received++ })
		key := r.Header.Get(headerIdempotencyKey)

		// 1. Validation. Permanent 4xx: retrying can never help.
		var req sendRequest
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if key == "" || len(key) > maxKeyLength {
			s.bad(w, channel, key, ReasonMissingKey, "Idempotency-Key header is required (max 200 chars)")
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.bad(w, channel, key, ReasonInvalidPayload, "body must be valid JSON")
			return
		}
		if reason, msg := validate(channel, req); reason != "" {
			s.bad(w, channel, key, reason, msg)
			return
		}

		// 2. Real rate limit, per channel.
		if ok, retry := s.limiters[channel].allow(); !ok {
			secs := int(math.Ceil(retry.Seconds()))
			if secs < 1 {
				secs = 1
			}
			s.bump(channel, func(c *ChannelStats) { c.RateLimited++ })
			s.reject(w, channel, key, http.StatusTooManyRequests, ReasonRateLimit, secs)
			return
		}

		// 3. Idempotency. Requests with the same key are serialised.
		e := s.entryFor(channel, key)
		e.mu.Lock()
		defer e.mu.Unlock()
		fp := fingerprint(channel, req)
		if e.done {
			if e.fingerprint != fp {
				s.bad(w, channel, key, ReasonKeyReused, "Idempotency-Key was already used with a different payload")
				return
			}
			s.bump(channel, func(c *ChannelStats) { c.Replays++ })
			s.log.Info("idempotent replay", "reason", ReasonReplay, "channel", channel,
				"idempotency_key", key, "provider_message_id", e.messageID)
			w.Header().Set(headerReplay, "true")
			writeJSON(w, http.StatusOK, accepted(e.messageID))
			return
		}

		// 4. Chaos. One fixed sequence of random draws per request keeps runs
		// with the same seed reproducible.
		d := s.draw()
		if err := s.clk.Sleep(r.Context(), s.cfg.Latency+d.jitter); err != nil {
			return // client went away
		}
		switch {
		case d.timeout < s.cfg.TimeoutRate:
			if err := s.clk.Sleep(r.Context(), s.cfg.TimeoutDelay); err != nil {
				return
			}
			s.bump(channel, func(c *ChannelStats) { c.Timeouts++ })
			s.reject(w, channel, key, http.StatusGatewayTimeout, ReasonTimeout, 0)
			return
		case d.inject429 < s.cfg.Rate429:
			s.bump(channel, func(c *ChannelStats) { c.Injected429++ })
			s.reject(w, channel, key, http.StatusTooManyRequests, ReasonInjected, d.retryAfter)
			return
		case d.inject5xx < s.cfg.ErrorRate:
			s.bump(channel, func(c *ChannelStats) { c.Injected5xx++ })
			s.reject(w, channel, key, d.status5xx, ReasonInjected, 0)
			return
		}

		// Accepted: from here on the message counts as delivered.
		e.done, e.fingerprint, e.messageID = true, fp, messageID(channel, key)
		s.bump(channel, func(c *ChannelStats) { c.UniqueDeliveries++ })

		if d.failAfter < s.cfg.FailAfterAccept {
			// The scenario idempotency exists for: the provider has the message,
			// but the client sees an error and will retry the same key.
			s.bump(channel, func(c *ChannelStats) { c.FailedAfterAccept++ })
			s.reject(w, channel, key, d.status5xx, ReasonFailAfterAccept, 0)
			return
		}
		writeJSON(w, http.StatusOK, accepted(e.messageID))
	}
}

type draws struct {
	jitter     time.Duration
	timeout    float64
	inject429  float64
	inject5xx  float64
	failAfter  float64
	retryAfter int
	status5xx  int
}

func (s *Server) draw() draws {
	s.rngMu.Lock()
	defer s.rngMu.Unlock()
	var d draws
	d.jitter = time.Duration(s.rng.Int64N(int64(s.cfg.LatencyJitter) + 1))
	d.timeout = s.rng.Float64()
	d.inject429 = s.rng.Float64()
	d.inject5xx = s.rng.Float64()
	d.failAfter = s.rng.Float64()
	d.retryAfter = s.cfg.RetryAfterMin + s.rng.IntN(s.cfg.RetryAfterMax-s.cfg.RetryAfterMin+1)
	d.status5xx = []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable}[s.rng.IntN(3)]
	return d
}

func (s *Server) entryFor(channel, key string) *entry {
	id := channel + "|" + key
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		e = &entry{}
		s.entries[id] = e
	}
	return e
}

func validate(channel string, req sendRequest) (reason, msg string) {
	switch channel {
	case ChannelSMS:
		if !phoneRe.MatchString(req.To) {
			return ReasonInvalidReceiver, "to must be a phone number like +905551234567"
		}
		if req.Body == "" {
			return ReasonInvalidPayload, "body is required"
		}
	case ChannelEmail:
		if !emailRe.MatchString(req.To) {
			return ReasonInvalidReceiver, "to must be a valid email address"
		}
		if req.Subject == "" || req.Body == "" {
			return ReasonInvalidPayload, "subject and body are required"
		}
	}
	return "", ""
}

func fingerprint(channel string, req sendRequest) string {
	h := sha256.New()
	for _, part := range []string{channel, req.To, req.Subject, req.Body} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func messageID(channel, key string) string {
	sum := sha256.Sum256([]byte(channel + "|" + key))
	return "pm_" + hex.EncodeToString(sum[:8])
}

func accepted(id string) map[string]string {
	return map[string]string{"provider_message_id": id, "status": "accepted"}
}

// bad answers a permanent client error (400/422).
func (s *Server) bad(w http.ResponseWriter, channel, key, reason, msg string) {
	status := http.StatusBadRequest
	if reason == ReasonKeyReused {
		status = http.StatusUnprocessableEntity
	}
	s.bump(channel, func(c *ChannelStats) { c.BadRequests++ })
	s.log.Warn("request rejected", "status", status, "reason", reason, "channel", channel, "idempotency_key", key)
	writeJSON(w, status, map[string]string{"error": reason, "message": msg})
}

// reject answers a 429/5xx and logs it with its cause.
func (s *Server) reject(w http.ResponseWriter, channel, key string, status int, reason string, retryAfterSec int) {
	attrs := []any{"status", status, "reason", reason, "channel", channel, "idempotency_key", key}
	if retryAfterSec > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
		attrs = append(attrs, "retry_after_sec", retryAfterSec)
	}
	s.log.Warn("request failed", attrs...)
	writeJSON(w, status, map[string]string{"error": reason})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
