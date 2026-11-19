package worker_test

import (
	"strings"
	"testing"
	"time"

	"smartcampaign/internal/worker"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadConfigDefaults(t *testing.T) {
	t.Parallel()
	c, err := worker.LoadConfig(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	d := worker.DefaultConfig()
	if c.RatePerSecond != 97 || c.Concurrency != 64 || c.Policy.MaxAttempts != 5 || c.RequestTimeout != 5*time.Second || c.Lease != d.Lease {
		t.Fatalf("%+v", c)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	t.Parallel()
	c, err := worker.LoadConfig(env(map[string]string{
		"WORKER_RATE_PER_SEC": "50", "WORKER_CONCURRENCY": "8", "WORKER_MAX_ATTEMPTS": "3", "WORKER_REQUEST_TIMEOUT_MS": "1500",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.RatePerSecond != 50 || c.Concurrency != 8 || c.Policy.MaxAttempts != 3 || c.RequestTimeout != 1500*time.Millisecond {
		t.Fatalf("%+v", c)
	}
}

func TestLoadConfigRejectsNonsenseAndNamesTheVariable(t *testing.T) {
	t.Parallel()
	for key, bad := range map[string]string{
		"WORKER_RATE_PER_SEC": "0", "WORKER_CONCURRENCY": "-1", "WORKER_MAX_ATTEMPTS": "abc", "WORKER_REQUEST_TIMEOUT_MS": "0",
	} {
		_, err := worker.LoadConfig(env(map[string]string{key: bad}))
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%q: %v", key, bad, err)
		}
	}
}
