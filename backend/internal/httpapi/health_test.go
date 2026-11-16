package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/httpapi"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/metrics"
	"smartcampaign/internal/testdb"
)

// serverWithCheck builds the api with a database check the test controls.
func serverWithCheck(t *testing.T, check func(context.Context) error) *httptest.Server {
	t.Helper()
	pool := testdb.NewSeeded(t)
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := mock.New(0)
	svc := campaign.NewService(pool, client, policy, log)
	srv := httptest.NewServer(httpapi.New(svc, httpapi.Info{LLMMode: client.Mode()}, check, metrics.New(pool).Handler(), log))
	t.Cleanup(srv.Close)
	return srv
}

// /healthz is what docker-compose asks: a container whose database is gone is
// not healthy, even though the process is running.
func TestHealthzReportsTheDatabase(t *testing.T) {
	var down atomic.Bool
	srv := serverWithCheck(t, func(context.Context) error {
		if down.Load() {
			return errors.New("connection refused")
		}
		return nil
	})

	resp, body := do(t, "GET", srv.URL+"/healthz", nil)
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"ok"`) {
		t.Fatalf("healthy: %d %s", resp.StatusCode, body)
	}

	down.Store(true)
	resp, body = do(t, "GET", srv.URL+"/healthz", nil)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "unavailable") {
		t.Fatalf("database down: %d %s", resp.StatusCode, body)
	}

	down.Store(false)
	if resp, _ := do(t, "GET", srv.URL+"/healthz", nil); resp.StatusCode != 200 {
		t.Fatalf("recovered: %d", resp.StatusCode)
	}
}

// The real thing: a pool whose connections are gone makes /healthz fail.
func TestHealthzFailsWhenTheRealPoolIsClosed(t *testing.T) {
	pool := testdb.NewSeeded(t)
	policy, _ := content.NewPolicy(nil)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := mock.New(0)
	svc := campaign.NewService(pool, client, policy, log)
	srv := httptest.NewServer(httpapi.New(svc, httpapi.Info{LLMMode: client.Mode()}, pool.Ping, metrics.New(pool).Handler(), log))
	defer srv.Close()

	if resp, _ := do(t, "GET", srv.URL+"/healthz", nil); resp.StatusCode != 200 {
		t.Fatalf("before: %d", resp.StatusCode)
	}
	pool.Close()
	if resp, body := do(t, "GET", srv.URL+"/healthz", nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("after closing the pool: %d %s", resp.StatusCode, body)
	}
}

func TestReadyzStillChecksTheDatabaseToo(t *testing.T) {
	srv := serverWithCheck(t, func(context.Context) error { return errors.New("down") })
	if resp, _ := do(t, "GET", srv.URL+"/readyz", nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestMetricsAreServedByTheAPI(t *testing.T) {
	srv := serverWithCheck(t, func(context.Context) error { return nil })
	resp, body := do(t, "GET", srv.URL+"/metrics", nil)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("%d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, want := range []string{
		`smartcampaign_deliveries_total{channel="sms",status="sent"} 0`,
		`smartcampaign_delivery_queue_length{channel="email"} 0`,
		`smartcampaign_llm_generations_total{kind="messages"} 0`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestListCampaignsOverHTTP(t *testing.T) {
	srv := newServer(t, mock.New(0))
	first := createCampaign(t, srv)
	second := createCampaign(t, srv)

	var out struct {
		Campaigns []struct {
			ID            int64  `json:"id"`
			Status        string `json:"status"`
			SegmentStatus string `json:"segment_status"`
			Brief         string `json:"brief"`
		} `json:"campaigns"`
	}
	resp, body := do(t, "GET", srv.URL+"/api/campaigns", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Campaigns) != 2 || out.Campaigns[0].ID != second || out.Campaigns[1].ID != first || out.Campaigns[0].SegmentStatus != "ok" || out.Campaigns[0].Brief == "" {
		t.Fatalf("%s", body)
	}

	resp, body = do(t, "GET", srv.URL+"/api/campaigns?limit=1", nil)
	_ = json.Unmarshal(body, &out)
	if resp.StatusCode != 200 || len(out.Campaigns) != 1 || out.Campaigns[0].ID != second {
		t.Fatalf("limit=1: %d %s", resp.StatusCode, body)
	}

	for _, bad := range []string{"abc", "1.5", "-1x"} {
		if resp, body := do(t, "GET", srv.URL+"/api/campaigns?limit="+bad, nil); resp.StatusCode != 400 {
			t.Errorf("limit=%s: %d %s", bad, resp.StatusCode, body)
		}
	}
}

func TestListOfNoCampaignsIsAnEmptyArray(t *testing.T) {
	srv := newServer(t, mock.New(0))
	resp, body := do(t, "GET", srv.URL+"/api/campaigns", nil)
	if resp.StatusCode != 200 || strings.TrimSpace(string(body)) != `{"campaigns":[]}` {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
}

// /api/system is what the dashboard's health lights read: the api, its database
// and (asked over the network) the worker.
func TestSystemReportsDatabaseAndWorker(t *testing.T) {
	var workerUp atomic.Bool
	workerUp.Store(true)
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !workerUp.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(worker.Close)

	pool := testdb.NewSeeded(t)
	policy, _ := content.NewPolicy(nil)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := campaign.NewService(pool, mock.New(0), policy, log)
	var dbDown atomic.Bool
	check := func(context.Context) error {
		if dbDown.Load() {
			return errors.New("gone")
		}
		return nil
	}
	srv := httptest.NewServer(httpapi.New(svc, httpapi.Info{LLMMode: "mock", WorkerHealthURL: worker.URL}, check, nil, log))
	t.Cleanup(srv.Close)

	get := func() map[string]string {
		_, body := do(t, "GET", srv.URL+"/api/system", nil)
		var m map[string]string
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatal(err, string(body))
		}
		return m
	}
	if m := get(); m["api"] != "ok" || m["database"] != "ok" || m["worker"] != "ok" {
		t.Fatalf("all up: %v", m)
	}
	workerUp.Store(false)
	dbDown.Store(true)
	if m := get(); m["worker"] != "down" || m["database"] != "down" || m["api"] != "ok" {
		t.Fatalf("worker and db down: %v", m)
	}
	worker.Close()
	if m := get(); m["worker"] != "down" {
		t.Fatalf("worker unreachable: %v", m)
	}
}
