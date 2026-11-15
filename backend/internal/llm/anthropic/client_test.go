package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
)

// fake is a stand-in for api.anthropic.com. No key, no network.
type fake struct {
	t      *testing.T
	mu     sync.Mutex
	bodies []map[string]any
	handle func(w http.ResponseWriter, r *http.Request)
}

func (f *fake) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			f.t.Errorf("request body is not JSON: %v", err)
		}
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.mu.Unlock()
		f.handle(w, r)
	}))
}

func (f *fake) client(srv *httptest.Server) *Client {
	return New("test-key", "claude-sonnet-5-5", option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
}

func messageJSON(text, stop string, extra string) string {
	return fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5",
		"content":[{"type":"text","text":%q}],"stop_reason":%q,"stop_sequence":null%s,
		"usage":{"input_tokens":11,"output_tokens":22}}`, text, stop, extra)
}

func respondJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

const segmentJSON = `{"status":"ok","match":"all","conditions":[{"field":"city","op":"eq","value":"İstanbul"}],"explanation":"x","question":""}`

func TestSegmentRequestShape(t *testing.T) {
	f := &fake{t: t, handle: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "test-key" && !strings.Contains(r.Header.Get("Authorization"), "test-key") {
			t.Errorf("api key not sent; headers: %v", r.Header)
		}
		respondJSON(w, 200, messageJSON(segmentJSON, "end_turn", ""))
	}}
	srv := f.server()
	defer srv.Close()

	out, err := f.client(srv).Segment(context.Background(), llm.SegmentRequest{Brief: "İstanbul'daki müşteriler"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Raw != segmentJSON || out.StopReason != llm.StopEnd || out.Usage.InputTokens != 11 || out.Usage.OutputTokens != 22 {
		t.Fatalf("output: %+v", out)
	}

	body := f.bodies[0]
	if body["model"] != "claude-sonnet-5-5" {
		t.Errorf("model = %v", body["model"])
	}
	// Parameters this model family rejects (HTTP 400) must never be sent.
	for _, banned := range []string{"temperature", "top_p", "top_k", "tool_choice", "tools", "thinking"} {
		if _, present := body[banned]; present {
			t.Errorf("request must not contain %q", banned)
		}
	}
	oc, _ := body["output_config"].(map[string]any)
	format, _ := oc["format"].(map[string]any)
	if format["type"] != "json_schema" || format["schema"] == nil {
		t.Errorf("structured output format missing: %+v", oc)
	}
	if oc["effort"] != "low" {
		t.Errorf("segment effort = %v, want low", oc["effort"])
	}
	if n, _ := body["max_tokens"].(float64); n != 8000 {
		t.Errorf("max_tokens = %v", body["max_tokens"])
	}
	system, _ := json.Marshal(body["system"])
	if !strings.Contains(string(system), "days_since_last_cart_add") {
		t.Error("system prompt must describe the field whitelist")
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages: %v", msgs)
	}
	// json.Marshal escapes < and > as < / >, so look at the decoded text.
	turn := msgs[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(turn, "<brief>") || !strings.Contains(turn, "İstanbul") {
		first, _ := json.Marshal(msgs[0])
		t.Errorf("user turn: %s", first)
	}
}

func TestRetryTurnShowsRejectedAnswerAndReasons(t *testing.T) {
	f := &fake{t: t, handle: func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, 200, messageJSON(segmentJSON, "end_turn", ""))
	}}
	srv := f.server()
	defer srv.Close()

	_, err := f.client(srv).Segment(context.Background(), llm.SegmentRequest{
		Brief:    "x",
		Previous: []llm.Attempt{{Raw: `{"status":"ok"}`, Problems: []string{"conditions[0].field bilinmiyor"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := f.bodies[0]["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("want user, assistant, user; got %d turns", len(msgs))
	}
	roles := []string{}
	for _, m := range msgs {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	if strings.Join(roles, ",") != "user,assistant,user" {
		t.Fatalf("roles: %v", roles)
	}
	last, _ := json.Marshal(msgs[2])
	if !strings.Contains(string(last), "conditions[0].field bilinmiyor") {
		t.Errorf("repair turn must carry the reasons: %s", last)
	}
}

func TestEmptyRejectedAnswerIsNotSentAsEmptyAssistantTurn(t *testing.T) {
	msgs := conversation("hi", []llm.Attempt{{Raw: "  ", Problems: []string{"p"}}})
	b, _ := json.Marshal(msgs[1])
	if strings.Contains(string(b), `"text":"  "`) || strings.Contains(string(b), `"text":""`) {
		t.Fatalf("empty assistant turn would be rejected by the API: %s", b)
	}
}

func TestRefusalBecomesRefusalError(t *testing.T) {
	f := &fake{t: t, handle: func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, 200, messageJSON("", "refusal", `,"stop_details":{"type":"refusal","category":"cyber","explanation":null}`))
	}}
	srv := f.server()
	defer srv.Close()

	_, err := f.client(srv).Segment(context.Background(), llm.SegmentRequest{Brief: "x"})
	var r *llm.RefusalError
	if !errors.As(err, &r) || r.Category != "cyber" {
		t.Fatalf("want RefusalError(cyber), got %v", err)
	}
	if !llm.IsFatal(err) {
		t.Error("a refusal must be fatal for the repair loop")
	}
}

func TestMaxTokensIsReported(t *testing.T) {
	f := &fake{t: t, handle: func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, 200, messageJSON(`{"status":"o`, "max_tokens", ""))
	}}
	srv := f.server()
	defer srv.Close()

	out, err := f.client(srv).Segment(context.Background(), llm.SegmentRequest{Brief: "x"})
	if err != nil || out.StopReason != llm.StopMaxTokens {
		t.Fatalf("%v %+v", err, out)
	}
}

func TestAuthErrorsAreFatalOthersAreNot(t *testing.T) {
	status := 401
	f := &fake{t: t, handle: func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, status, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}}
	srv := f.server()
	defer srv.Close()
	c := f.client(srv)

	_, err := c.Segment(context.Background(), llm.SegmentRequest{Brief: "x"})
	var a *llm.AuthError
	if !errors.As(err, &a) || !llm.IsFatal(err) {
		t.Fatalf("401 must be an AuthError, got %v", err)
	}

	status = 500
	f.handle = func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, 500, `{"type":"error","error":{"type":"api_error","message":"boom"}}`)
	}
	_, err = c.Segment(context.Background(), llm.SegmentRequest{Brief: "x"})
	if err == nil || llm.IsFatal(err) {
		t.Fatalf("500 must be an ordinary, retryable error, got %v", err)
	}
}

func sse(w http.ResponseWriter, events ...[2]string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, e := range events {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e[0], e[1])
	}
}

func TestMessagesStreamsTextDeltasAndAccumulates(t *testing.T) {
	const full = `{"sms_body":"Merhaba {{first_name}}!","email_subject":"Konu","email_body":"Gövde"}`
	parts := []string{full[:20], full[20:45], full[45:]}

	f := &fake{t: t, handle: func(w http.ResponseWriter, r *http.Request) {
		events := [][2]string{
			{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":30,"output_tokens":1}}}`},
			{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		}
		for _, p := range parts {
			b, _ := json.Marshal(p)
			events = append(events, [2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + string(b) + `}}`})
		}
		events = append(events,
			[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":40}}`},
			[2]string{"message_stop", `{"type":"message_stop"}`},
		)
		sse(w, events...)
	}}
	srv := f.server()
	defer srv.Close()

	pol, _ := content.NewPolicy(nil)
	var got []string
	out, err := f.client(srv).Messages(context.Background(),
		llm.MessageRequest{Brief: "b", Explanation: "e", Policy: pol},
		func(s string) { got = append(got, s) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "") != full || len(got) != 3 {
		t.Fatalf("deltas: %q", got)
	}
	if out.Raw != full || out.StopReason != llm.StopEnd {
		t.Fatalf("output: %+v", out)
	}

	body := f.bodies[0]
	if body["stream"] != true {
		t.Error("message generation must stream")
	}
	oc, _ := body["output_config"].(map[string]any)
	if oc["effort"] != "medium" {
		t.Errorf("messages effort = %v", oc["effort"])
	}
	if format, _ := oc["format"].(map[string]any); format["type"] != "json_schema" {
		t.Errorf("format: %+v", oc)
	}
	system, _ := json.Marshal(body["system"])
	for _, want := range []string{"160", "{{first_name}} counts as 20", "{{city}} counts as 15"} {
		if !strings.Contains(string(system), want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestStreamErrorStatusIsMapped(t *testing.T) {
	f := &fake{t: t, handle: func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, 403, `{"type":"error","error":{"type":"permission_error","message":"nope"}}`)
	}}
	srv := f.server()
	defer srv.Close()
	pol, _ := content.NewPolicy(nil)
	_, err := f.client(srv).Messages(context.Background(), llm.MessageRequest{Brief: "b", Policy: pol}, nil)
	var a *llm.AuthError
	if !errors.As(err, &a) {
		t.Fatalf("want AuthError, got %v", err)
	}
}
