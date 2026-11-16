package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/httpapi"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/metrics"
	"smartcampaign/internal/testdb"
)

const exampleBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

func newServer(t *testing.T, client llm.Client) *httptest.Server {
	t.Helper()
	pool := testdb.NewSeeded(t)
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := campaign.NewService(pool, client, policy, log)
	srv := httptest.NewServer(httpapi.New(svc, httpapi.Info{LLMMode: client.Mode(), Model: client.Model()}, pool.Ping, metrics.New(pool).Handler(), log))
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, method, url string, body any) (*http.Response, []byte) {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, url, rd)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

func createCampaign(t *testing.T, srv *httptest.Server) int64 {
	t.Helper()
	resp, body := do(t, "POST", srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief})
	if resp.StatusCode != 201 {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var v struct{ ID int64 }
	_ = json.Unmarshal(body, &v)
	return v.ID
}

type event struct {
	Name string
	Data map[string]any
}

func readEvents(t *testing.T, body io.Reader) []event {
	t.Helper()
	var events []event
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var cur event
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.Name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &cur.Data)
		case line == "" && cur.Name != "":
			events = append(events, cur)
			cur = event{}
		}
	}
	return events
}

func names(events []event) string {
	var out []string
	for _, e := range events {
		if len(out) == 0 || out[len(out)-1] != e.Name || e.Name != "delta" {
			out = append(out, e.Name)
		}
	}
	return strings.Join(out, ",")
}

func TestInfoExposesLimitsAndVariables(t *testing.T) {
	srv := newServer(t, mock.New(0))
	resp, body := do(t, "GET", srv.URL+"/api/info", nil)
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	var info struct {
		LLMMode   string         `json:"llm_mode"`
		Limits    map[string]int `json:"limits"`
		Variables []struct {
			Name   string
			Budget int
		} `json:"variables"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatal(err)
	}
	if info.LLMMode != "mock" || info.Limits["sms_max"] != 160 || info.Limits["subject_max"] != 80 {
		t.Fatalf("info: %s", body)
	}
	if len(info.Variables) != 2 || info.Variables[0].Name != "first_name" || info.Variables[0].Budget != 20 ||
		info.Variables[1].Name != "city" || info.Variables[1].Budget != 15 {
		t.Fatalf("variables: %+v", info.Variables)
	}
}

// The screen reads the language list with exactly these lowercase keys; with
// other keys every language code is undefined and the generated messages never
// show up ("henüz mesaj yok", approval stays disabled).
func TestInfoListsTheLanguagesWithTheKeysTheScreenReads(t *testing.T) {
	srv := newServer(t, mock.New(0))
	_, body := do(t, "GET", srv.URL+"/api/info", nil)
	var info struct {
		Languages       []map[string]string `json:"languages"`
		DefaultLanguage string              `json:"default_language"`
		Variables       []struct {
			Fallbacks map[string]string `json:"fallbacks"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatal(err)
	}
	if info.DefaultLanguage != "tr" || len(info.Languages) != 3 {
		t.Fatalf("info: %s", body)
	}
	for i, want := range []string{"tr", "en", "de"} {
		l := info.Languages[i]
		if l["code"] != want || l["name"] == "" || l["display"] == "" || len(l) != 3 {
			t.Fatalf("language %d: %v (want lowercase keys code, name, display)", i, l)
		}
	}
	for _, v := range info.Variables {
		if len(v.Fallbacks) != 3 {
			t.Fatalf("fallbacks: %v", v.Fallbacks)
		}
	}
}

func TestCreateAndGetCampaign(t *testing.T) {
	srv := newServer(t, mock.New(0))
	resp, body := do(t, "POST", srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief})
	if resp.StatusCode != 201 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	var v struct {
		ID      int64  `json:"id"`
		Status  string `json:"status"`
		Segment struct {
			Status     string `json:"status"`
			Conditions []struct {
				Field string `json:"field"`
				Op    string `json:"op"`
				Value any    `json:"value"`
			} `json:"conditions"`
			Audience map[string]int `json:"audience"`
		} `json:"segment"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	if v.Status != "segment_ready" || v.Segment.Audience["matched"] != 300 || v.Segment.Audience["sms_reachable"] != 203 ||
		v.Segment.Audience["email_reachable"] != 227 || len(v.Segment.Conditions) != 3 {
		t.Fatalf("view: %s", body)
	}

	resp, body = do(t, "GET", fmt.Sprintf("%s/api/campaigns/%d", srv.URL, v.ID), nil)
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"segment_ready"`) {
		t.Fatalf("get: %d %s", resp.StatusCode, body)
	}
}

func TestBadRequests(t *testing.T) {
	srv := newServer(t, mock.New(0))
	tests := []struct {
		name         string
		method, path string
		body         any
		status       int
		code         string
	}{
		{"not json", "POST", "/api/campaigns", "hello", 400, "invalid_json"},
		{"unknown field", "POST", "/api/campaigns", `{"brief":"abcdefgh","x":1}`, 400, "invalid_json"},
		{"two documents", "POST", "/api/campaigns", `{"brief":"abcdefgh"}{"brief":"x"}`, 400, "invalid_json"},
		{"brief too short", "POST", "/api/campaigns", map[string]string{"brief": "abc"}, 400, "invalid_input"},
		{"unknown campaign", "GET", "/api/campaigns/12345", nil, 404, "not_found"},
		{"bad id", "GET", "/api/campaigns/abc", nil, 404, "not_found"},
		{"stream unknown campaign", "POST", "/api/campaigns/12345/messages/stream", nil, 404, "not_found"},
		{"put unknown campaign", "PUT", "/api/campaigns/12345/content", `{"sms_body":"a","email_subject":"b","email_body":"c"}`, 404, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := do(t, tt.method, srv.URL+tt.path, tt.body)
			if resp.StatusCode != tt.status || !strings.Contains(string(body), `"`+tt.code+`"`) {
				t.Fatalf("%d %s", resp.StatusCode, body)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("errors must be JSON, got %q", ct)
			}
		})
	}
}

func TestGenerationFailuresUseStableErrorCodes(t *testing.T) {
	bad := `{"status":"ok","match":"all","conditions":[],"explanation":"x","question":""}`
	tests := []struct {
		name   string
		step   mock.Step
		status int
		code   string
	}{
		{"three invalid answers", mock.Step{Kind: mock.Raw, Raw: bad}, 422, "segment_generation_failed"},
		{"refusal", mock.Step{Kind: mock.Refusal}, 422, "llm_refused"},
		{"provider down", mock.Step{Kind: mock.Error, Err: errors.New("down")}, 502, "llm_unavailable"},
		{"bad key", mock.Step{Kind: mock.Error, Err: &llm.AuthError{Err: errors.New("401")}}, 503, "llm_misconfigured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer(t, mock.NewScripted([]mock.Step{tt.step}, nil))
			resp, body := do(t, "POST", srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief})
			if resp.StatusCode != tt.status || !strings.Contains(string(body), `"`+tt.code+`"`) {
				t.Fatalf("%d %s", resp.StatusCode, body)
			}
			if strings.Contains(string(body), "401") && tt.code == "llm_misconfigured" && strings.Contains(string(body), "sk-") {
				t.Fatal("never echo credentials")
			}
		})
	}
	// The failed attempts are described to the client.
	srv := newServer(t, mock.NewScripted([]mock.Step{{Kind: mock.Raw, Raw: bad}}, nil))
	_, body := do(t, "POST", srv.URL+"/api/campaigns", map[string]string{"brief": exampleBrief})
	if !strings.Contains(string(body), `"attempts"`) || !strings.Contains(string(body), "en az bir koşul") {
		t.Fatalf("details missing: %s", body)
	}
}

func TestStreamMessagesOverSSE(t *testing.T) {
	srv := newServer(t, mock.New(0))
	id := createCampaign(t, srv)

	resp, err := http.Post(fmt.Sprintf("%s/api/campaigns/%d/messages/stream", srv.URL, id), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("%d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("headers: %v", resp.Header)
	}

	events := readEvents(t, resp.Body)
	// Every event says which language it is about; the reference language's own
	// sequence is the familiar one, and the others each end validated.
	var refEvents, all []event
	validated := map[string]map[string]any{}
	built := map[string]map[string]string{}
	for _, e := range events {
		lang, _ := e.Data["language"].(string)
		if e.Name == "done" {
			refEvents = append(refEvents, e)
			continue
		}
		if lang == "" {
			t.Fatalf("event %s has no language: %v", e.Name, e.Data)
		}
		all = append(all, e)
		if lang == "tr" {
			refEvents = append(refEvents, e)
		}
		switch e.Name {
		case "validated":
			validated[lang] = e.Data
		case "delta":
			if built[lang] == nil {
				built[lang] = map[string]string{}
			}
			built[lang][e.Data["field"].(string)] += e.Data["text"].(string)
		}
	}
	if got := names(refEvents); got != "attempt,delta,validated,done" {
		t.Fatalf("events: %s", got)
	}
	if events[len(events)-1].Name != "done" {
		t.Fatalf("the stream must end with done: %s", names(events))
	}
	for _, lang := range []string{"tr", "en", "de"} {
		v, ok := validated[lang]
		if !ok {
			t.Fatalf("no validated event for %s", lang)
		}
		final := v["content"].(map[string]any)
		for _, f := range []string{"sms_body", "email_subject", "email_body"} {
			if built[lang][f] != final[f] || built[lang][f] == "" {
				t.Fatalf("%s field %s: streamed %q, final %q", lang, f, built[lang][f], final[f])
			}
		}
		if report := v["report"].(map[string]any); report["ok"] != true {
			t.Fatalf("%s report: %v", lang, report)
		}
	}
	_ = all

	// The result is stored.
	_, body := do(t, "GET", fmt.Sprintf("%s/api/campaigns/%d", srv.URL, id), nil)
	if !strings.Contains(string(body), `"content_ready"`) || !strings.Contains(string(body), `"source":"llm"`) {
		t.Fatalf("stored: %s", body)
	}
}

func TestStreamReportsRetriesAndFallback(t *testing.T) {
	long := `{"sms_body":"` + strings.Repeat("a", 170) + `","email_subject":"k","email_body":"b"}`
	srv := newServer(t, mock.NewScripted(nil, []mock.Step{{Kind: mock.Raw, Raw: long}}))
	id := createCampaign(t, srv)

	resp, err := http.Post(fmt.Sprintf("%s/api/campaigns/%d/messages/stream", srv.URL, id), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := readEvents(t, resp.Body)
	got := names(events)
	if !strings.HasPrefix(got, "attempt,delta,retry,attempt,delta,retry,attempt,delta,fallback") || !strings.HasSuffix(got, ",done") {
		t.Fatalf("events: %s", got)
	}
	for _, e := range events {
		if e.Name == "fallback" {
			if e.Data["reason"] != "invalid_output" || e.Data["source"] != "fallback" {
				t.Fatalf("fallback event: %v", e.Data)
			}
		}
		if e.Name == "retry" {
			if p, _ := e.Data["problems"].([]any); len(p) == 0 {
				t.Fatalf("retry must say why: %v", e.Data)
			}
		}
	}
}

func TestStreamBeforeSegmentIsAPlainJSONConflict(t *testing.T) {
	srv := newServer(t, mock.New(0))
	_, body := do(t, "POST", srv.URL+"/api/campaigns", map[string]string{"brief": "Hoş geldin mesajı gönder"})
	var v struct{ ID int64 }
	_ = json.Unmarshal(body, &v)

	resp, body := do(t, "POST", fmt.Sprintf("%s/api/campaigns/%d/messages/stream", srv.URL, v.ID), nil)
	if resp.StatusCode != 409 || !strings.Contains(string(body), "invalid_state") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatal("an error before any event must not be an SSE response")
	}
}

func TestStreamMisconfiguredKeySendsAnErrorEventNotATemplate(t *testing.T) {
	srv := newServer(t, mock.NewScripted(nil, []mock.Step{{Kind: mock.Error, Err: &llm.AuthError{Err: errors.New("401")}}}))
	id := createCampaign(t, srv)
	resp, err := http.Post(fmt.Sprintf("%s/api/campaigns/%d/messages/stream", srv.URL, id), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// The stream was already open (the first attempt was announced), so the
	// failure travels as an event.
	events := readEvents(t, resp.Body)
	if got := names(events); got != "attempt,error" {
		t.Fatalf("events: %s", got)
	}
	if events[1].Data["code"] != "llm_misconfigured" {
		t.Fatalf("error event: %v", events[1].Data)
	}
	_, body := do(t, "GET", fmt.Sprintf("%s/api/campaigns/%d", srv.URL, id), nil)
	if strings.Contains(string(body), `"content"`) {
		t.Fatalf("no template content may be stored: %s", body)
	}
}

func TestPutContentValidatesLikeTheModelOutput(t *testing.T) {
	srv := newServer(t, mock.New(0))
	id := createCampaign(t, srv)
	url := fmt.Sprintf("%s/api/campaigns/%d/content", srv.URL, id)

	resp, body := do(t, "PUT", url, map[string]string{
		"sms_body": strings.Repeat("a", 161), "email_subject": "iki\nsatır", "email_body": "{{last_name}}",
	})
	if resp.StatusCode != 422 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	var out struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Report content.Report `json:"report"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, i := range out.Error.Details.Report.Issues {
		have[i.Field+":"+i.Code] = true
	}
	for _, want := range []string{"sms_body:too_long", "email_subject:newline_in_subject", "email_body:unknown_variable"} {
		if !have[want] {
			t.Errorf("missing issue %s in %s", want, body)
		}
	}
	if out.Error.Code != "invalid_content" {
		t.Fatalf("code = %s", out.Error.Code)
	}

	one := map[string]string{"sms_body": "Merhaba {{ first_name }}!", "email_subject": "Konu", "email_body": "Gövde"}
	resp, body = do(t, "PUT", url, map[string]any{"languages": map[string]any{"tr": one, "en": one, "de": one}})
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"source":"edited"`) || !strings.Contains(string(body), "Merhaba {{first_name}}!") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
}

func TestApproveOverHTTP(t *testing.T) {
	srv := newServer(t, mock.New(0))
	id := createCampaign(t, srv)
	approve := fmt.Sprintf("%s/api/campaigns/%d/approve", srv.URL, id)

	// No content yet: a plain conflict, nothing queued.
	resp, body := do(t, "POST", approve, nil)
	if resp.StatusCode != 409 || !strings.Contains(string(body), `"invalid_state"`) {
		t.Fatalf("before content: %d %s", resp.StatusCode, body)
	}

	resp, body = do(t, "PUT", fmt.Sprintf("%s/api/campaigns/%d/content", srv.URL, id),
		map[string]any{"languages": map[string]any{
			"tr": map[string]string{"sms_body": "{{first_name}}, %30 indirim seni bekliyor!", "email_subject": "%30 indirim", "email_body": "Merhaba {{first_name}}, %30 indirim var."},
			"en": map[string]string{"sms_body": "{{first_name}}, 30% off is waiting for you!", "email_subject": "30% off", "email_body": "Hi {{first_name}}, enjoy 30% off."},
			"de": map[string]string{"sms_body": "{{first_name}}, 30 % Rabatt wartet auf Sie!", "email_subject": "30 % Rabatt", "email_body": "Hallo {{first_name}}, 30 % Rabatt für Sie."},
		}})
	if resp.StatusCode != 200 {
		t.Fatalf("put: %d %s", resp.StatusCode, body)
	}

	type approved struct {
		Status          string `json:"status"`
		AlreadyApproved bool   `json:"already_approved"`
		Queued          struct{ SMS, Email int }
		Audience        *struct{ Matched int }
		Warnings        []any `json:"warnings"`
	}
	var first approved
	resp, body = do(t, "POST", approve, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("approve: %d %s", resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, &first); err != nil {
		t.Fatal(err)
	}
	if first.Status != "approved" || first.AlreadyApproved || first.Queued.SMS == 0 || first.Queued.Email == 0 || first.Audience == nil || first.Warnings == nil {
		t.Fatalf("approve body: %s", body)
	}

	var again approved
	resp, body = do(t, "POST", approve, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("repeat: %d %s", resp.StatusCode, body)
	}
	_ = json.Unmarshal(body, &again)
	if !again.AlreadyApproved || again.Queued != first.Queued {
		t.Fatalf("repeat body: %s", body)
	}

	// Frozen after approval.
	resp, body = do(t, "PUT", fmt.Sprintf("%s/api/campaigns/%d/content", srv.URL, id),
		map[string]string{"sms_body": "x", "email_subject": "y", "email_body": "z"})
	if resp.StatusCode != 409 {
		t.Fatalf("edit after approval: %d %s", resp.StatusCode, body)
	}

	if resp, _ := do(t, "POST", srv.URL+"/api/campaigns/999999/approve", nil); resp.StatusCode != 404 {
		t.Fatalf("unknown campaign: %d", resp.StatusCode)
	}
}

var _ = context.Background

func putAllLanguages(t *testing.T, srv *httptest.Server, id int64) {
	t.Helper()
	one := func(sms, subject, body string) map[string]string {
		return map[string]string{"sms_body": sms, "email_subject": subject, "email_body": body}
	}
	resp, body := do(t, "PUT", fmt.Sprintf("%s/api/campaigns/%d/content", srv.URL, id), map[string]any{"languages": map[string]any{
		"tr": one("{{first_name}}, %30 indirim seni bekliyor!", "%30 indirim", "Merhaba {{first_name}}, %30 indirim var."),
		"en": one("{{first_name}}, 30% off is waiting for you!", "30% off", "Hi {{first_name}}, enjoy 30% off."),
		"de": one("{{first_name}}, 30 % Rabatt wartet auf Sie!", "30 % Rabatt", "Hallo {{first_name}}, 30 % Rabatt für Sie."),
	}})
	if resp.StatusCode != 200 {
		t.Fatalf("put: %d %s", resp.StatusCode, body)
	}
}

func TestScheduleAndCancelOverHTTP(t *testing.T) {
	srv := newServer(t, mock.New(0))
	id := createCampaign(t, srv)
	putAllLanguages(t, srv, id)
	approve := fmt.Sprintf("%s/api/campaigns/%d/approve", srv.URL, id)
	cancel := fmt.Sprintf("%s/api/campaigns/%d/cancel", srv.URL, id)
	at := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)

	// Bad requests: a time in the past, nonsense, unknown fields. Nothing changes.
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if resp, body := do(t, "POST", approve, map[string]string{"scheduled_at": past}); resp.StatusCode != 400 || !strings.Contains(string(body), `"invalid_input"`) {
		t.Fatalf("past: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "POST", approve, map[string]string{"scheduled_at": "tomorrow"}); resp.StatusCode != 400 {
		t.Fatalf("nonsense: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", approve, map[string]string{"when": "now"}); resp.StatusCode != 400 {
		t.Fatalf("unknown field: %d", resp.StatusCode)
	}
	if resp, body := do(t, "POST", cancel, nil); resp.StatusCode != 409 {
		t.Fatalf("cancel before scheduling: %d %s", resp.StatusCode, body)
	}

	resp, body := do(t, "POST", approve, map[string]string{"scheduled_at": at.Format(time.RFC3339)})
	if resp.StatusCode != 200 {
		t.Fatalf("schedule: %d %s", resp.StatusCode, body)
	}
	var res struct {
		Status      string    `json:"status"`
		ScheduledAt time.Time `json:"scheduled_at"`
		Queued      struct{ SMS, Email int }
		Audience    *struct{ Matched int }
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != "scheduled" || !res.ScheduledAt.Equal(at) || res.Queued.SMS != 0 || res.Queued.Email != 0 || res.Audience != nil {
		t.Fatalf("schedule body: %s", body)
	}

	_, body = do(t, "GET", fmt.Sprintf("%s/api/campaigns/%d", srv.URL, id), nil)
	if !strings.Contains(string(body), `"status":"scheduled"`) || !strings.Contains(string(body), `"scheduled_at"`) {
		t.Fatalf("get: %s", body)
	}
	_, body = do(t, "GET", srv.URL+"/api/campaigns", nil)
	if !strings.Contains(string(body), `"status":"scheduled"`) || !strings.Contains(string(body), `"scheduled_at"`) {
		t.Fatalf("list: %s", body)
	}

	resp, body = do(t, "POST", cancel, nil)
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"status":"cancelled"`) {
		t.Fatalf("cancel: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "POST", cancel, nil); resp.StatusCode != 200 {
		t.Fatalf("cancelling twice: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", approve, nil); resp.StatusCode != 409 {
		t.Fatalf("approve after cancel: %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", srv.URL+"/api/campaigns/999999/cancel", nil); resp.StatusCode != 404 {
		t.Fatalf("unknown campaign: %d", resp.StatusCode)
	}

	// A campaign that already started cannot be cancelled.
	id2 := createCampaign(t, srv)
	putAllLanguages(t, srv, id2)
	if resp, body := do(t, "POST", fmt.Sprintf("%s/api/campaigns/%d/approve", srv.URL, id2), nil); resp.StatusCode != 200 {
		t.Fatalf("send now: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(t, "POST", fmt.Sprintf("%s/api/campaigns/%d/cancel", srv.URL, id2), nil); resp.StatusCode != 409 {
		t.Fatalf("cancel a started campaign: %d %s", resp.StatusCode, body)
	}
}
