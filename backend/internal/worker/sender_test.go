package worker_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"smartcampaign/internal/worker"
)

func stub(t *testing.T, h http.HandlerFunc) *worker.HTTPSender {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return worker.NewHTTPSender(srv.URL, 5*time.Second)
}

func TestSendPostsTheMessageWithAnIdempotencyKey(t *testing.T) {
	t.Parallel()
	var gotPath, gotKey, gotCT string
	var gotBody map[string]string
	s := stub(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey, gotCT = r.URL.Path, r.Header.Get("Idempotency-Key"), r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"provider_message_id":"pm_abc","status":"accepted"}`))
	})

	res := s.Send(context.Background(), "email", worker.Message{To: "a@b.co", Subject: "Konu", Body: "Gövde"}, "42")
	if res.Status != 200 || res.Err != nil || res.MessageID != "pm_abc" {
		t.Fatalf("%+v", res)
	}
	if gotPath != "/v1/email/send" || gotKey != "42" || gotCT != "application/json" {
		t.Fatalf("path=%s key=%s ct=%s", gotPath, gotKey, gotCT)
	}
	if gotBody["to"] != "a@b.co" || gotBody["subject"] != "Konu" || gotBody["body"] != "Gövde" {
		t.Fatalf("body: %v", gotBody)
	}

	s.Send(context.Background(), "sms", worker.Message{To: "+905551234567", Body: "Merhaba"}, "43")
	if gotPath != "/v1/sms/send" {
		t.Fatalf("sms path: %s", gotPath)
	}
}

func TestRetryAfterAndReasonAreExtracted(t *testing.T) {
	t.Parallel()
	s := stub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"rate_limit"}`))
	})
	res := s.Send(context.Background(), "sms", worker.Message{To: "+905551234567", Body: "x"}, "1")
	if res.Status != 429 || res.RetryAfter != 7*time.Second || res.Detail != "rate_limit" || res.Err != nil {
		t.Fatalf("%+v", res)
	}
}

func TestPermanentRejectionKeepsItsReason(t *testing.T) {
	t.Parallel()
	s := stub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_recipient","message":"to must be a phone number"}`))
	})
	res := s.Send(context.Background(), "sms", worker.Message{To: "bad", Body: "x"}, "1")
	if res.Status != 400 || res.Detail != "invalid_recipient" {
		t.Fatalf("%+v", res)
	}
}

func TestAnUnreadableErrorBodyDoesNotBreakAnything(t *testing.T) {
	t.Parallel()
	s := stub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	})
	res := s.Send(context.Background(), "sms", worker.Message{To: "+905551234567", Body: "x"}, "1")
	if res.Status != 502 || res.Err != nil {
		t.Fatalf("%+v", res)
	}
}

// A provider that never answers must not hang a sender: the call times out and
// is reported as a transport error (which the policy retries).
func TestASlowProviderTimesOutAsATransportError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	s := worker.NewHTTPSender(srv.URL, 100*time.Millisecond)
	start := time.Now()
	res := s.Send(context.Background(), "sms", worker.Message{To: "+905551234567", Body: "x"}, "1")
	if res.Err == nil || res.Status != 0 || res.Detail != "timeout" {
		t.Fatalf("%+v", res)
	}
	if time.Since(start) > time.Second {
		t.Fatal("did not give up in time")
	}
}

func TestAnUnreachableProviderIsATransportError(t *testing.T) {
	t.Parallel()
	s := worker.NewHTTPSender("http://127.0.0.1:1", time.Second)
	res := s.Send(context.Background(), "sms", worker.Message{To: "+905551234567", Body: "x"}, "1")
	if res.Err == nil || res.Status != 0 {
		t.Fatalf("%+v", res)
	}
}
