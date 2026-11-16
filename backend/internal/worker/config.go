package worker

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LoadConfig reads WORKER_* variables through getenv; unset or empty ones keep
// the defaults. Every problem is reported with the variable's name.
func LoadConfig(getenv func(string) string) (Config, error) {
	c := DefaultConfig()
	var errs []string

	integer := func(key string, dst *int) {
		v := strings.TrimSpace(getenv(key))
		if v == "" {
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			errs = append(errs, fmt.Sprintf("%s must be an integer >= 1, got %q", key, v))
			return
		}
		*dst = n
	}
	integer("WORKER_RATE_PER_SEC", &c.RatePerSecond)
	integer("WORKER_CONCURRENCY", &c.Concurrency)
	integer("WORKER_MAX_ATTEMPTS", &c.Policy.MaxAttempts)

	ms := int(c.RequestTimeout / time.Millisecond)
	integer("WORKER_REQUEST_TIMEOUT_MS", &ms)
	c.RequestTimeout = time.Duration(ms) * time.Millisecond

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid worker configuration: %s", strings.Join(errs, "; "))
	}
	return c, nil
}
