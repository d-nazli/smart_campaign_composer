package campaign_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/segment"
	"smartcampaign/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

const exampleBrief = "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

func newService(t *testing.T, client llm.Client) (*campaign.Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.NewSeeded(t)
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	return campaign.NewService(pool, client, policy, slog.New(slog.NewTextHandler(io.Discard, nil))), pool
}

// newUnseededService is newService on a migrated database without the 5000
// users, for tests that never look at an audience.
func newUnseededService(t *testing.T, client llm.Client) (*campaign.Service, *pgxpool.Pool) {
	t.Helper()
	pool, _ := testdb.New(t)
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	return campaign.NewService(pool, client, policy, slog.New(slog.NewTextHandler(io.Discard, nil))), pool
}

func raw(s string) mock.Step { return mock.Step{Kind: mock.Raw, Raw: s} }

const badSegment = `{"status":"ok","match":"all","conditions":[{"field":"password","op":"eq","value":"x"}],"explanation":"x","question":""}`

func tooLongSMS() string {
	return `{"sms_body":"` + strings.Repeat("a", 170) + `","email_subject":"Konu","email_body":"Gövde"}`
}

const goodMessages = `{"sms_body":"{{first_name}}, %30 indirim seni bekliyor!","email_subject":"{{first_name}}, sepetin seni bekliyor","email_body":"Merhaba {{first_name}},\n\nSepetindeki ürünlerde %30 indirim var.","approach":"fayda"}`

func audit(t *testing.T, pool *pgxpool.Pool, campaignID int64, kind string) (total, valid int) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT count(*), count(*) FILTER (WHERE valid) FROM llm_generations WHERE campaign_id=$1 AND kind=$2 AND locale='tr-TR'`, campaignID, kind).Scan(&total, &valid)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func asGenErr(t *testing.T, err error) *campaign.GenerationError {
	t.Helper()
	var ge *campaign.GenerationError
	if !errors.As(err, &ge) {
		t.Fatalf("want *GenerationError, got %T: %v", err, err)
	}
	return ge
}

// ---- segment ---------------------------------------------------------------

func TestCreateExampleBriefWithSmartMock(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	v, err := svc.Create(context.Background(), exampleBrief)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != campaign.StatusSegmentReady || v.Segment == nil || v.Segment.Status != segment.StatusOK {
		t.Fatalf("view: %+v", v)
	}
	want := segment.Audience{Matched: 300, SMSReachable: 203, EmailReachable: 227, BothReachable: 165}
	if *v.Segment.Audience != want {
		t.Fatalf("audience = %+v, want %+v", *v.Segment.Audience, want)
	}
	if len(v.Segment.Conditions) != 3 || v.Segment.Match != segment.MatchAll {
		t.Fatalf("segment: %+v", v.Segment)
	}
	if total, valid := audit(t, pool, v.ID, "segment"); total != 1 || valid != 1 {
		t.Fatalf("audit rows: total=%d valid=%d", total, valid)
	}

	// Reload from the database.
	again, err := svc.Get(context.Background(), v.ID)
	if err != nil || again.Segment.Audience.Matched != 300 || len(again.Segment.Conditions) != 3 {
		t.Fatalf("reload: %+v %v", again, err)
	}
}

func TestCreateRepairsABrokenSegmentAndAuditsBothAttempts(t *testing.T) {
	t.Parallel()
	m := mock.NewScripted([]mock.Step{raw(badSegment), {Kind: mock.Smart}}, nil)
	svc, pool := newService(t, m)
	v, err := svc.Create(context.Background(), exampleBrief)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != campaign.StatusSegmentReady {
		t.Fatalf("status %s", v.Status)
	}
	if total, valid := audit(t, pool, v.ID, "segment"); total != 2 || valid != 1 {
		t.Fatalf("audit rows: total=%d valid=%d, want 2 and 1", total, valid)
	}
	var problems string
	if err := pool.QueryRow(context.Background(),
		`SELECT problems::text FROM llm_generations WHERE campaign_id=$1 AND attempt=1`, v.ID).Scan(&problems); err != nil || !strings.Contains(problems, "password") {
		t.Fatalf("rejected attempt must record why: %q %v", problems, err)
	}
}

func TestCreateGivesUpAfterThreeBadAnswers(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.NewScripted([]mock.Step{raw(badSegment)}, nil))
	_, err := svc.Create(context.Background(), exampleBrief)
	ge := asGenErr(t, err)
	if ge.Code != campaign.CodeSegmentFailed || len(ge.Attempts) != 3 {
		t.Fatalf("%+v", ge)
	}
	var status string
	var id int64
	if err := pool.QueryRow(context.Background(), `SELECT id, status FROM campaigns ORDER BY id DESC LIMIT 1`).Scan(&id, &status); err != nil {
		t.Fatal(err)
	}
	if status != campaign.StatusDraft {
		t.Fatalf("a failed segment must leave the campaign in draft, got %s", status)
	}
	if total, valid := audit(t, pool, id, "segment"); total != 3 || valid != 0 {
		t.Fatalf("audit rows: total=%d valid=%d", total, valid)
	}
}

func TestCreateMapsLLMFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		step mock.Step
		code string
	}{
		{"refusal", mock.Step{Kind: mock.Refusal}, campaign.CodeLLMRefused},
		{"transport errors", mock.Step{Kind: mock.Error, Err: errors.New("down")}, campaign.CodeLLMUnavailable},
		{"bad credentials", mock.Step{Kind: mock.Error, Err: &llm.AuthError{Err: errors.New("401")}}, campaign.CodeLLMMisconfigured},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newService(t, mock.NewScripted([]mock.Step{tt.step}, nil))
			_, err := svc.Create(context.Background(), exampleBrief)
			if ge := asGenErr(t, err); ge.Code != tt.code {
				t.Fatalf("code = %s, want %s", ge.Code, tt.code)
			}
		})
	}
}

func TestCreateClarificationAndUnsupportedStayInDraft(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))

	v, err := svc.Create(context.Background(), "Hoş geldin mesajı gönder")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != campaign.StatusDraft || v.Segment.Status != segment.StatusNeedsClarification || v.Segment.Question == "" || v.Segment.Audience != nil {
		t.Fatalf("clarification: %+v %+v", v, v.Segment)
	}

	v, err = svc.Create(context.Background(), "35 yaşındaki kadınlara kampanya")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != campaign.StatusDraft || v.Segment.Status != segment.StatusUnsupported || v.Segment.Explanation == "" {
		t.Fatalf("unsupported: %+v", v.Segment)
	}

	// Message generation needs a usable segment.
	err = svc.GenerateMessages(context.Background(), v.ID, campaign.Events{})
	var se *campaign.StateError
	if !errors.As(err, &se) {
		t.Fatalf("want StateError, got %v", err)
	}
}

func TestCreateRejectsBadBriefs(t *testing.T) {
	t.Parallel()
	svc, pool := newUnseededService(t, mock.New(0))
	for _, brief := range []string{"", "   ", "abc", strings.Repeat("a", 2001)} {
		_, err := svc.Create(context.Background(), brief)
		var ie *campaign.InputError
		if !errors.As(err, &ie) {
			t.Errorf("brief of %d chars: want InputError, got %v", len(brief), err)
		}
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM campaigns`).Scan(&n)
	if n != 0 {
		t.Fatalf("rejected briefs must not create campaigns, got %d", n)
	}
}

func TestGetUnknownCampaign(t *testing.T) {
	t.Parallel()
	svc, _ := newUnseededService(t, mock.New(0))
	if _, err := svc.Get(context.Background(), 999); !errors.Is(err, campaign.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

// ---- messages --------------------------------------------------------------

// The source is translated into every locale of the audience; these tests are
// about the source run, so the recorder only keeps its events.
const ref = "tr-TR"

type recorded struct {
	names  []string
	deltas map[string]string
	valid  *content.Content
	fbMsg  string
	fb     *content.Content
	retry  [][]string
}

func (r *recorded) events() campaign.Events {
	r.deltas = map[string]string{}
	return campaign.Events{
		Attempt: func(lang string, n, max int) {
			if lang == ref {
				r.names = append(r.names, "attempt")
			}
		},
		Delta: func(lang, f, text string) {
			if lang != ref {
				return
			}
			if len(r.names) == 0 || r.names[len(r.names)-1] != "delta" {
				r.names = append(r.names, "delta")
			}
			r.deltas[f] += text
		},
		Retry: func(lang string, _ int, p []string) {
			if lang == ref {
				r.names = append(r.names, "retry")
				r.retry = append(r.retry, p)
			}
		},
		Validated: func(lang string, c content.Content, _ content.Report) {
			if lang == ref {
				r.names = append(r.names, "validated")
				r.valid = &c
			}
		},
		Fallback: func(lang, reason string, c content.Content, _ content.Report) {
			if lang == ref {
				r.names = append(r.names, "fallback")
				r.fbMsg, r.fb = reason, &c
			}
		},
	}
}

func readyCampaign(t *testing.T, svc *campaign.Service) int64 {
	t.Helper()
	v, err := svc.Create(context.Background(), exampleBrief)
	if err != nil {
		t.Fatal(err)
	}
	return v.ID
}

func TestGenerateMessagesStreamsThenValidatesAndStores(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	id := readyCampaign(t, svc)

	var rec recorded
	if err := svc.GenerateMessages(context.Background(), id, rec.events()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rec.names, ","); got != "attempt,delta,validated" {
		t.Fatalf("event order: %s", got)
	}
	if rec.valid == nil {
		t.Fatal("no validated content")
	}
	// The live preview must add up to the final, validated text.
	if rec.deltas[content.FieldSMSBody] != rec.valid.SMSBody ||
		rec.deltas[content.FieldEmailSubject] != rec.valid.EmailSubject ||
		rec.deltas[content.FieldEmailBody] != rec.valid.EmailBody {
		t.Fatalf("streamed text differs from final text:\n%v\n%+v", rec.deltas, rec.valid)
	}

	v, err := svc.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != campaign.StatusContentReady || v.Content == nil || v.Content.Source != campaign.SourceLLM || !v.Content.Report.OK {
		t.Fatalf("stored view: %+v", v)
	}
	if total, valid := audit(t, pool, id, "messages"); total != 1 || valid != 1 {
		t.Fatalf("audit rows: %d/%d", total, valid)
	}
}

func TestGenerateMessagesRepairsOverlongSMS(t *testing.T) {
	t.Parallel()
	m := mock.NewScripted(nil, []mock.Step{raw(tooLongSMS()), raw(goodMessages)})
	svc, pool := newService(t, m)
	id := readyCampaign(t, svc)

	var rec recorded
	if err := svc.GenerateMessages(context.Background(), id, rec.events()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rec.names, ","); !strings.Contains(got, "retry") || !strings.HasSuffix(got, "validated") {
		t.Fatalf("event order: %s", got)
	}
	if len(rec.retry) != 1 || !strings.Contains(rec.retry[0][0], "160") {
		t.Fatalf("retry reasons: %v", rec.retry)
	}
	if rec.valid.SMSBody != "{{first_name}}, %30 indirim seni bekliyor!" {
		t.Fatalf("content: %+v", rec.valid)
	}
	if total, valid := audit(t, pool, id, "messages"); total != 2 || valid != 1 {
		t.Fatalf("audit rows: %d/%d", total, valid)
	}
	var calls []mock.Call
	for _, c := range m.Calls() {
		if c.Kind == "messages" {
			calls = append(calls, c)
		}
	}
	if last := calls[len(calls)-1]; len(last.Previous) != 1 || !strings.Contains(last.Previous[0].Problems[0], "160") {
		t.Fatalf("the model must see why its answer was rejected: %+v", last)
	}
}

func TestGenerateMessagesFallsBackAfterThreeInvalidAnswers(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.NewScripted(nil, []mock.Step{raw(tooLongSMS())}))
	id := readyCampaign(t, svc)

	var rec recorded
	if err := svc.GenerateMessages(context.Background(), id, rec.events()); err != nil {
		t.Fatal(err)
	}
	if rec.fb == nil || rec.fbMsg != campaign.FallbackInvalidOutput || rec.valid != nil {
		t.Fatalf("expected fallback(invalid_output), got reason=%q valid=%v", rec.fbMsg, rec.valid)
	}
	pol, _ := content.NewPolicy(nil)
	if _, rep := pol.Validate(*rec.fb); !rep.OK {
		t.Fatalf("the fallback must satisfy every limit: %+v", rep.Issues)
	}
	v, _ := svc.Get(context.Background(), id)
	if v.Status != campaign.StatusContentReady || v.Content.Source != campaign.SourceFallback {
		t.Fatalf("stored: %+v", v)
	}
	if total, valid := audit(t, pool, id, "messages"); total != 3 || valid != 0 {
		t.Fatalf("audit rows: %d/%d", total, valid)
	}
}

func TestGenerateMessagesFallbackReasons(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		step   mock.Step
		reason string
	}{
		{"refusal", mock.Step{Kind: mock.Refusal}, campaign.FallbackRefused},
		{"api down", mock.Step{Kind: mock.Error, Err: errors.New("down")}, campaign.FallbackUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newService(t, mock.NewScripted(nil, []mock.Step{tt.step}))
			id := readyCampaign(t, svc)
			var rec recorded
			if err := svc.GenerateMessages(context.Background(), id, rec.events()); err != nil {
				t.Fatal(err)
			}
			if rec.fbMsg != tt.reason {
				t.Fatalf("reason = %q, want %q", rec.fbMsg, tt.reason)
			}
		})
	}
}

func TestBadCredentialsAreNotHiddenBehindTheFallback(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.NewScripted(nil, []mock.Step{{Kind: mock.Error, Err: &llm.AuthError{Err: errors.New("401")}}}))
	id := readyCampaign(t, svc)
	var rec recorded
	err := svc.GenerateMessages(context.Background(), id, rec.events())
	if ge := asGenErr(t, err); ge.Code != campaign.CodeLLMMisconfigured {
		t.Fatalf("code = %s", ge.Code)
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM campaign_contents`).Scan(&n)
	if n != 0 || rec.fb != nil {
		t.Fatal("a misconfiguration must not silently store template content")
	}
}

func TestClientDisconnectStoresNothing(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	id := readyCampaign(t, svc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ev := campaign.Events{Delta: func(string, string, string) { cancel() }} // the browser closes the tab mid-stream
	err := svc.GenerateMessages(ctx, id, ev)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM campaign_contents`).Scan(&n)
	if n != 0 {
		t.Fatal("an abandoned generation must not store content")
	}
	v, _ := svc.Get(context.Background(), id)
	if v.Status != campaign.StatusSegmentReady {
		t.Fatalf("status = %s", v.Status)
	}
}

func TestRegeneratingReplacesTheContent(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	id := readyCampaign(t, svc)
	for i := 0; i < 2; i++ {
		if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
			t.Fatal(err)
		}
	}
	v, _ := svc.Get(context.Background(), id)
	if v.Status != campaign.StatusContentReady || v.Content.Source != campaign.SourceLLM {
		t.Fatalf("%+v", v)
	}
}

// ---- manual edits ----------------------------------------------------------

func TestSaveContentAppliesTheSameRulesAsModelOutput(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	id := readyCampaign(t, svc)
	ctx := context.Background()
	if err := svc.GenerateMessages(ctx, id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	before, _ := svc.Get(ctx, id)

	good := content.Content{SMSBody: "Merhaba {{ first_name }}!", EmailSubject: "Konu", EmailBody: "Gövde"}
	rejected := map[string]content.Content{
		"sms 161":     {SMSBody: strings.Repeat("a", 161), EmailSubject: "k", EmailBody: "b"},
		"sms 141+20":  {SMSBody: strings.Repeat("a", 141) + "{{first_name}}", EmailSubject: "k", EmailBody: "b"},
		"subject 81":  {SMSBody: "s", EmailSubject: strings.Repeat("b", 81), EmailBody: "b"},
		"subject \\n": {SMSBody: "s", EmailSubject: "iki\nsatır", EmailBody: "b"},
		"last_name":   {SMSBody: "{{last_name}}", EmailSubject: "k", EmailBody: "b"},
		"empty":       {},
	}
	for name, c := range rejected {
		_, err := saveSource(ctx, svc, id, c)
		var ic *campaign.InvalidContentError
		if !errors.As(err, &ic) || ic.Report.OK || len(ic.Report.Issues) == 0 {
			t.Errorf("%s: want InvalidContentError, got %v", name, err)
		}
	}
	after, _ := svc.Get(ctx, id)
	if after.Content.Content != before.Content.Content {
		t.Fatal("rejected edits must leave the stored content untouched")
	}

	v, err := saveSource(ctx, svc, id, good)
	if err != nil {
		t.Fatal(err)
	}
	if v.Content.Source != campaign.SourceEdited || v.Content.SMSBody != "Merhaba {{first_name}}!" {
		t.Fatalf("edit: %+v", v.Content)
	}

	// Saving the same text again keeps the source; saving model output unchanged
	// must not relabel it as edited.
	if err := svc.GenerateMessages(ctx, id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	llmView, _ := svc.Get(ctx, id)
	same, err := svc.SaveContent(ctx, id, llmView.Content.Content)
	if err != nil || same.Content.Source != campaign.SourceLLM {
		t.Fatalf("unchanged save: %v %+v", err, same.Content)
	}
}

func TestSaveContentNeedsASegmentFirst(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	v, err := svc.Create(context.Background(), "Hoş geldin mesajı gönder") // needs clarification -> draft
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SaveContent(context.Background(), v.ID, content.Content{SMSBody: "a", EmailSubject: "b", EmailBody: "c"})
	var se *campaign.StateError
	if !errors.As(err, &se) {
		t.Fatalf("got %v", err)
	}
}
