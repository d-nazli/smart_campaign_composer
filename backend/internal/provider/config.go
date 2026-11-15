// Package provider implements the mock SMS/email provider: a stand-in for a
// real vendor that is slow, flaky and rate limited on purpose, so the worker's
// retry, backoff, 429 and idempotency handling can be exercised for real.
package provider

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Config controls the provider's behaviour. Rates are probabilities in [0,1];
// tests use only 0 or 1 so they never depend on randomness.
type Config struct {
	Latency            time.Duration // base delay before answering
	LatencyJitter      time.Duration // plus a random 0..Jitter
	ErrorRate          float64       // injected 500/502/503 before accepting
	Rate429            float64       // injected 429 (independent of the real limit)
	TimeoutRate        float64       // answer 504 only after TimeoutDelay
	TimeoutDelay       time.Duration
	FailAfterAccept    float64 // message IS recorded, but the response is a 5xx
	RetryAfterMin      int     // seconds, for injected 429s
	RetryAfterMax      int
	RateLimitPerSecond int // real limit, per channel
	Seed               uint64
}

// DefaultConfig is deliberately noisy (~11% of first attempts fail) so a demo
// shows retries; see .env.example for how to lower it.
func DefaultConfig() Config {
	return Config{
		Latency:            50 * time.Millisecond,
		LatencyJitter:      100 * time.Millisecond,
		ErrorRate:          0.05,
		Rate429:            0.03,
		TimeoutRate:        0.01,
		TimeoutDelay:       10 * time.Second,
		FailAfterAccept:    0.02,
		RetryAfterMin:      1,
		RetryAfterMax:      3,
		RateLimitPerSecond: 100,
		Seed:               1,
	}
}

// LoadConfig reads PROVIDER_* variables through getenv; unset or empty
// variables keep their defaults.
func LoadConfig(getenv func(string) string) (Config, error) {
	c := DefaultConfig()
	var errs []string

	dur := func(key string, dst *time.Duration) {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			ms, err := strconv.Atoi(v)
			if err != nil || ms < 0 {
				errs = append(errs, fmt.Sprintf("%s must be a non-negative integer (milliseconds), got %q", key, v))
				return
			}
			*dst = time.Duration(ms) * time.Millisecond
		}
	}
	rate := func(key string, dst *float64) {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f < 0 || f > 1 {
				errs = append(errs, fmt.Sprintf("%s must be a number between 0 and 1, got %q", key, v))
				return
			}
			*dst = f
		}
	}
	integer := func(key string, dst *int, min int) {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < min {
				errs = append(errs, fmt.Sprintf("%s must be an integer >= %d, got %q", key, min, v))
				return
			}
			*dst = n
		}
	}

	dur("PROVIDER_LATENCY_MS", &c.Latency)
	dur("PROVIDER_LATENCY_JITTER_MS", &c.LatencyJitter)
	dur("PROVIDER_TIMEOUT_MS", &c.TimeoutDelay)
	rate("PROVIDER_ERROR_RATE", &c.ErrorRate)
	rate("PROVIDER_429_RATE", &c.Rate429)
	rate("PROVIDER_TIMEOUT_RATE", &c.TimeoutRate)
	rate("PROVIDER_FAIL_AFTER_ACCEPT_RATE", &c.FailAfterAccept)
	integer("PROVIDER_RETRY_AFTER_MIN_SEC", &c.RetryAfterMin, 1)
	integer("PROVIDER_RETRY_AFTER_MAX_SEC", &c.RetryAfterMax, 1)
	integer("PROVIDER_RATE_LIMIT_PER_SEC", &c.RateLimitPerSecond, 1)
	if v := strings.TrimSpace(getenv("PROVIDER_SEED")); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			errs = append(errs, fmt.Sprintf("PROVIDER_SEED must be an unsigned integer, got %q", v))
		} else {
			c.Seed = n
		}
	}
	if c.RetryAfterMax < c.RetryAfterMin {
		errs = append(errs, "PROVIDER_RETRY_AFTER_MAX_SEC must be >= PROVIDER_RETRY_AFTER_MIN_SEC")
	}
	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid provider configuration: %s", strings.Join(errs, "; "))
	}
	return c, nil
}
