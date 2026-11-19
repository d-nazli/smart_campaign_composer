package campaign_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/campaign"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/segment"
)

func campaignCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM campaigns`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The reported bug: answering the question opened a second campaign and left the
// first one waiting forever. The answer now resolves the audience on the same
// campaign, which becomes segment_ready; no campaign is added.
func TestAnAnswerResolvesTheSameCampaign(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()

	v, err := svc.Create(ctx, "Kış ürünlerinde 200 TL indirim")
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Status != segment.StatusNeedsClarification || v.Status != campaign.StatusDraft {
		t.Fatalf("the brief names no audience, so it is asked: %+v", v.Segment)
	}
	before := campaignCount(t, pool)
	question := v.Segment.Question

	got, err := svc.Clarify(ctx, v.ID, "Tüm müşterilere gönder.")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != v.ID || got.Status != campaign.StatusSegmentReady || got.Segment.Status != segment.StatusOK {
		t.Fatalf("the same campaign must be segment_ready: id %d status %s %+v", got.ID, got.Status, got.Segment)
	}
	if got.Segment.Audience == nil || got.Segment.Audience.Matched == 0 || got.Segment.Question != "" {
		t.Fatalf("the audience is counted and the question is gone: %+v", got.Segment)
	}
	if n := campaignCount(t, pool); n != before {
		t.Fatalf("campaigns %d → %d: an answer must not add one", before, n)
	}
	if got.Brief != "Kış ürünlerinde 200 TL indirim" {
		t.Fatalf("the brief stays as written: %q", got.Brief)
	}

	// The question and the answer are kept, and shown with the audience.
	again, err := svc.Get(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	cs := again.Segment.Clarifications
	if len(cs) != 1 || cs[0].Question != question || cs[0].Answer != "Tüm müşterilere gönder." || cs[0].AnsweredAt.IsZero() {
		t.Fatalf("history: %+v", cs)
	}

	// Answered once, it waits for nothing: a second answer is a conflict.
	var state *campaign.StateError
	if _, err := svc.Clarify(ctx, v.ID, "Ankara'daki müşterilere"); !errors.As(err, &state) {
		t.Fatalf("want a StateError, got %v", err)
	}
}

// An answer that still leaves the audience open keeps the campaign waiting, with
// the new question; the next answer resolves it. Every answer is kept.
func TestAnAnswerThatIsNotEnoughIsAskedAgain(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	v, err := svc.Create(ctx, "%10 indirim gönder")
	if err != nil {
		t.Fatal(err)
	}
	before := campaignCount(t, pool)

	v2, err := svc.Clarify(ctx, v.ID, "bilmiyorum")
	if err != nil {
		t.Fatal(err)
	}
	if v2.ID != v.ID || v2.Segment.Status != segment.StatusNeedsClarification || len(v2.Segment.Clarifications) != 1 {
		t.Fatalf("still waiting, one answer kept: %+v", v2.Segment)
	}
	v3, err := svc.Clarify(ctx, v.ID, "Ankara'daki müşterilere")
	if err != nil {
		t.Fatal(err)
	}
	if v3.Status != campaign.StatusSegmentReady || len(v3.Segment.Clarifications) != 2 || v3.Segment.Clarifications[1].Answer != "Ankara'daki müşterilere" {
		t.Fatalf("resolved with both answers kept: %+v", v3.Segment)
	}
	if len(v3.Segment.Conditions) != 1 || v3.Segment.Conditions[0].Field != "city" || v3.Segment.Conditions[0].Value != "Ankara" {
		t.Fatalf("the answer's audience: %+v", v3.Segment.Conditions)
	}
	if n := campaignCount(t, pool); n != before {
		t.Fatalf("campaigns %d → %d", before, n)
	}
}

// Only a campaign that waits for an answer takes one.
func TestAnAnswerToACampaignThatWaitsForNoneIsAConflict(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	ctx := context.Background()
	var state *campaign.StateError

	ready, err := svc.Create(ctx, exampleBrief)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Clarify(ctx, ready.ID, "evet"); !errors.As(err, &state) {
		t.Fatalf("segment_ready: want a StateError, got %v", err)
	}
	unsupported, err := svc.Create(ctx, "telefon kategorisine bakan müşterilere %10 indirim")
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Segment.Status != segment.StatusUnsupported {
		t.Fatalf("%+v", unsupported.Segment)
	}
	if _, err := svc.Clarify(ctx, unsupported.ID, "evet"); !errors.As(err, &state) {
		t.Fatalf("unsupported: want a StateError, got %v", err)
	}
	if _, err := svc.Clarify(ctx, 999999, "evet"); !errors.Is(err, campaign.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	var input *campaign.InputError
	if _, err := svc.Clarify(ctx, ready.ID, "   "); !errors.As(err, &input) {
		t.Fatalf("an empty answer is an input error: %v", err)
	}
}

// Two answers sent at once: the row lock lets exactly one of them through; the
// other finds the question answered.
func TestTwoAnswersAtOnceApplyOnlyOne(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	ctx := context.Background()
	v, err := svc.Create(ctx, "%10 indirim gönder")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, answer := range []string{"Tüm müşterilere gönder.", "Ankara'daki müşterilere"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.Clarify(ctx, v.ID, answer)
		}()
	}
	wg.Wait()
	ok, conflicts := 0, 0
	var state *campaign.StateError
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.As(err, &state):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	got, _ := svc.Get(ctx, v.ID)
	if ok != 1 || conflicts != 1 || len(got.Segment.Clarifications) != 1 || got.Status != campaign.StatusSegmentReady {
		t.Fatalf("ok %d conflicts %d history %+v status %s", ok, conflicts, got.Segment.Clarifications, got.Status)
	}
}

// With the real model's flow the answer is given to the model with its
// question; the model's new rule is validated and counted like any other.
func TestAnAnswerInModelDrivenMode(t *testing.T) {
	t.Parallel()
	ask := `{"name":"Kış ürünleri · 200 TL","status":"needs_clarification","match":"all","conditions":[],"explanation":"","question":"Kime gönderilsin?","note":"","offer":{"kind":"none","value":0,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}}`
	ok := `{"name":"Ankara kış ürünleri · 200 TL","status":"ok","match":"all","conditions":[{"field":"city","op":"eq","value":"Ankara"}],"explanation":"Ankara'daki müşteriler","question":"","note":"","offer":{"kind":"amount","value":200,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}}`
	m := mock.NewScripted([]mock.Step{raw(ask), raw(ok)}, nil)
	svc, pool := newService(t, m)
	svc.UseSegmentMode(brief.ModelDriven)
	ctx := context.Background()
	v, err := svc.Create(ctx, "Kış ürünlerinde 200 TL indirim")
	if err != nil {
		t.Fatal(err)
	}
	before := campaignCount(t, pool)
	got, err := svc.Clarify(ctx, v.ID, "Ankara'daki müşterilere")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != v.ID || got.Status != campaign.StatusSegmentReady || got.Segment.Audience == nil || got.Segment.Audience.Matched == 0 {
		t.Fatalf("%+v", got.Segment)
	}
	if cs := got.Segment.Clarifications; len(cs) != 1 || cs[0].Question != "Kime gönderilsin?" {
		t.Fatalf("history: %+v", cs)
	}
	if campaignCount(t, pool) != before || len(m.Calls()) != 2 {
		t.Fatalf("no new campaign, one more model call: %d calls", len(m.Calls()))
	}
}

// A campaign that was never approved can be deleted (for example one left
// waiting for an answer); an approved one cannot.
func TestDeleteOnlyBeforeApproval(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()

	waiting, err := svc.Create(ctx, "%10 indirim gönder")
	if err != nil {
		t.Fatal(err)
	}
	before := campaignCount(t, pool)
	if err := svc.Delete(ctx, waiting.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, waiting.ID); !errors.Is(err, campaign.ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
	if campaignCount(t, pool) != before-1 {
		t.Fatal("the campaign must be gone")
	}
	if err := svc.Delete(ctx, waiting.ID); !errors.Is(err, campaign.ErrNotFound) {
		t.Fatalf("twice: %v", err)
	}

	sent, err := svc.Create(ctx, exampleBrief)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.GenerateMessages(ctx, sent.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, sent.ID); err != nil {
		t.Fatal(err)
	}
	var state *campaign.StateError
	if err := svc.Delete(ctx, sent.ID); !errors.As(err, &state) {
		t.Fatalf("approved: want a StateError, got %v", err)
	}
}

// "Desteklenmiyor" and "Soru bekliyor": the edited brief is resolved on the
// same campaign, which moves to its new state; no campaign is added.
func TestEditingTheBriefResolvesTheSameCampaign(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()

	unsupported, err := svc.Create(ctx, "telefon kategorisine bakan müşterilere %15 indirim")
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Segment.Status != segment.StatusUnsupported {
		t.Fatalf("%+v", unsupported.Segment)
	}
	before := campaignCount(t, pool)
	got, err := svc.UpdateBrief(ctx, unsupported.ID, "İstanbul'daki müşterilere %15 indirim")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != unsupported.ID || got.Status != campaign.StatusSegmentReady || got.Segment.Status != segment.StatusOK || got.Brief != "İstanbul'daki müşterilere %15 indirim" {
		t.Fatalf("same id, segment_ready, the new brief: id %d %s %+v %q", got.ID, got.Status, got.Segment, got.Brief)
	}
	if got.Segment.Audience == nil || got.Segment.Audience.Matched == 0 || got.Segment.Explanation == "" {
		t.Fatalf("the audience is counted: %+v", got.Segment)
	}
	if n := campaignCount(t, pool); n != before {
		t.Fatalf("campaigns %d → %d: an edit must not add one", before, n)
	}

	// A campaign waiting for an answer: the edited brief replaces the question and its history.
	asking, err := svc.Create(ctx, "%10 indirim gönder")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Clarify(ctx, asking.ID, "bilmiyorum"); err != nil {
		t.Fatal(err)
	}
	edited, err := svc.UpdateBrief(ctx, asking.ID, "Ankara'daki müşterilere %10 indirim gönder")
	if err != nil {
		t.Fatal(err)
	}
	if edited.ID != asking.ID || edited.Status != campaign.StatusSegmentReady || edited.Segment.Question != "" || len(edited.Segment.Clarifications) != 0 {
		t.Fatalf("the old question and answers belong to the old brief: %+v", edited.Segment)
	}

	// An edit can also lead to a question: the same campaign waits for it.
	back, err := svc.UpdateBrief(ctx, asking.ID, "%20 indirim gönder")
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != asking.ID || back.Status != campaign.StatusDraft || back.Segment.Status != segment.StatusNeedsClarification || back.Segment.Audience != nil {
		t.Fatalf("waits again, no audience: %s %+v", back.Status, back.Segment)
	}
	if n := campaignCount(t, pool); n != before+1 {
		t.Fatalf("only the one Create added a campaign: %d → %d", before, n)
	}
}

// A campaign with texts: the edit drops them (they were written for the old
// brief) and the campaign is segment_ready again.
func TestEditingTheBriefDropsTheOldTexts(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()
	v, err := svc.Create(ctx, exampleBrief)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.UpdateBrief(ctx, v.ID, "Ankara'daki müşterilere %20 indirim")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != campaign.StatusSegmentReady || got.Content != nil || len(got.Translations) != 0 {
		t.Fatalf("texts must be gone: %s content=%v translations=%d", got.Status, got.Content != nil, len(got.Translations))
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM campaign_contents WHERE campaign_id=$1`, v.ID).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("contents rows %d %v", rows, err)
	}
}

// Once approved the brief stays: 409. A bad brief is an input error; an
// unknown campaign is not found.
func TestEditingTheBriefOfAnApprovedCampaignIsAConflict(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, mock.New(0))
	ctx := context.Background()
	v, err := svc.Create(ctx, exampleBrief)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	var state *campaign.StateError
	if _, err := svc.UpdateBrief(ctx, v.ID, "Ankara'daki müşterilere %20 indirim"); !errors.As(err, &state) {
		t.Fatalf("approved: want a StateError, got %v", err)
	}
	got, _ := svc.Get(ctx, v.ID)
	if got.Brief != exampleBrief {
		t.Fatalf("the brief must not change: %q", got.Brief)
	}
	var input *campaign.InputError
	if _, err := svc.UpdateBrief(ctx, v.ID, "abc"); !errors.As(err, &input) {
		t.Fatalf("too short: %v", err)
	}
	if _, err := svc.UpdateBrief(ctx, 999999, "Ankara'daki müşterilere %20 indirim"); !errors.Is(err, campaign.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
}
