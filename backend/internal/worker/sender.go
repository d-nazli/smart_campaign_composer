package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Message is one outgoing SMS or email, already personalised.
type Message struct {
	To      string
	Subject string // email only
	Body    string
}

// Result is the provider's answer to one send.
type Result struct {
	Outcome
	MessageID string // provider_message_id on success
}

// Sender delivers a message. The idempotency key makes a repeated send safe.
type Sender interface {
	Send(ctx context.Context, channel string, m Message, idempotencyKey string) Result
}

// HTTPSender talks to the provider over HTTP.
type HTTPSender struct {
	BaseURL string
	Client  *http.Client
}

const maxResponseBytes = 64 << 10

// NewHTTPSender builds a sender whose every call gives up after timeout. The
// timeout is deliberately shorter than the provider's 10s "hang" chaos mode.
func NewHTTPSender(baseURL string, timeout time.Duration) *HTTPSender {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConnsPerHost = 64 // many senders share one provider host
	return &HTTPSender{BaseURL: strings.TrimRight(baseURL, "/"), Client: &http.Client{Timeout: timeout, Transport: tr}}
}

// Send posts one message. Transport problems come back as Outcome.Err (no
// Status); every HTTP answer, successful or not, comes back as Status.
func (s *HTTPSender) Send(ctx context.Context, channel string, m Message, key string) Result {
	payload := map[string]string{"to": m.To, "body": m.Body}
	if channel == "email" {
		payload["subject"] = m.Subject
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Result{Outcome: Outcome{Err: err, Detail: "cannot encode message"}}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/v1/"+channel+"/send", bytes.NewReader(raw))
	if err != nil {
		return Result{Outcome: Outcome{Err: err, Detail: "cannot build request"}}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)

	resp, err := s.Client.Do(req)
	if err != nil {
		return Result{Outcome: Outcome{Err: err, Detail: transportDetail(ctx, err)}}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))

	var parsed struct {
		Error     string `json:"error"`
		MessageID string `json:"provider_message_id"`
	}
	_ = json.Unmarshal(body, &parsed) // an unreadable body is fine; the status counts

	res := Result{Outcome: Outcome{Status: resp.StatusCode, Detail: parsed.Error}, MessageID: parsed.MessageID}
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs > 0 {
			res.RetryAfter = time.Duration(secs) * time.Second
		}
	}
	return res
}

func transportDetail(ctx context.Context, err error) string {
	var ne net.Error
	switch {
	case ctx.Err() != nil:
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return "timeout"
	}
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return "network error: " + msg
}
