package campaign_test

import (
	"context"
	"testing"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/campaign"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/segment"
)

// In the real model's mode (D113) the service gives the brief to the model even
// when the rule reader would have stopped it, validates the answer only
// technically, and counts the model's audience. The same brief in the mock's
// (strict) mode is still asked about.
func TestModelDrivenModeLetsTheModelReadTheBrief(t *testing.T) {
	t.Parallel()
	const b = "Bu ay sepete ürün ekleyip almayanlara %10 indirim"
	modelRule := `{"name":"Test kampanyası","status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"lte","value":30},{"field":"purchased_since_last_cart_add","op":"eq","value":false}],"explanation":"Bu ay sepete ekleyip almayanlar","question":"","note":"\"Bu ay\" son 30 gün olarak okundu.","offer":{"kind":"percent","value":10,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}}`

	m := mock.NewScripted([]mock.Step{raw(modelRule)}, nil)
	svc, _ := newService(t, m)
	svc.UseSegmentMode(brief.ModelDriven)
	v, err := svc.Create(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Status != segment.StatusOK || v.Segment.Audience == nil || v.Segment.Audience.Matched == 0 || len(m.Calls()) != 1 {
		t.Fatalf("model-driven: %+v (calls %d)", v.Segment, len(m.Calls()))
	}
	if v.Segment.Note != "\"Bu ay\" son 30 gün olarak okundu." || len(v.Segment.Warnings) != 0 {
		t.Fatalf("the model's note, no code warnings: %q %q", v.Segment.Note, v.Segment.Warnings)
	}

	strictModel := mock.NewScripted([]mock.Step{raw(modelRule)}, nil)
	strict, _ := newService(t, strictModel)
	v, err = strict.Create(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Status != segment.StatusNeedsClarification || len(strictModel.Calls()) != 0 {
		t.Fatalf("strict: %+v (calls %d)", v.Segment, len(strictModel.Calls()))
	}
}

// The model's rule is the proposal: it is shown with its exact audience count,
// and nothing is queued or frozen until the person approves. The rule is used
// as the model wrote it (here without the brief's "did not buy"); no code
// compares it with the brief, and the person sees exactly that rule.
func TestModelDrivenProposalIsCountedAndNothingIsSentBeforeApproval(t *testing.T) {
	t.Parallel()
	const b = "Son 30 günde sepete ürün ekleyip almayanlara %10 indirim"
	onlyWindow := `{"name":"Test kampanyası","status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"lte","value":30}],"explanation":"Son 30 günde sepete ekleyenler","question":"","note":"","offer":{"kind":"percent","value":10,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}}`

	m := mock.NewScripted([]mock.Step{raw(onlyWindow)}, nil)
	svc, pool := newService(t, m)
	svc.UseSegmentMode(brief.ModelDriven)
	ctx := context.Background()
	v, err := svc.Create(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Status != segment.StatusOK || len(m.Calls()) != 1 || len(v.Segment.Warnings) != 0 || len(v.Segment.Conditions) != 1 {
		t.Fatalf("the model's rule after one call, no warnings: %+v (calls %d)", v.Segment, len(m.Calls()))
	}

	// The count is the compiled rule run against the database, not an estimate.
	var want int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE last_cart_add_at >= now() - interval '30 days'`).Scan(&want); err != nil {
		t.Fatal(err)
	}
	if v.Segment.Audience == nil || v.Segment.Audience.Matched != want {
		t.Fatalf("audience %+v, want matched %d", v.Segment.Audience, want)
	}

	deliveries := func() (n int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM deliveries WHERE campaign_id=$1`, v.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(ctx, v.ID)
	if n := deliveries(); n != 0 || got.Status == "approved" || got.Status == "sending" {
		t.Fatalf("before the approval nothing may be queued: status %s, %d deliveries", got.Status, n)
	}
	if _, err := svc.Approve(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if deliveries() == 0 {
		t.Fatal("the approval queues the audience")
	}
}

// Invalid output that the model never repairs is an error for the screen,
// not a rule and not a question.
func TestModelDrivenInvalidOutputFailsTheStep(t *testing.T) {
	t.Parallel()
	m := mock.NewScripted([]mock.Step{raw(badSegment)}, nil)
	svc, _ := newService(t, m)
	svc.UseSegmentMode(brief.ModelDriven)
	if _, err := svc.Create(context.Background(), "İstanbul'daki müşterilere %10 indirim"); err == nil {
		t.Fatal("want an error")
	}
	if len(m.Calls()) != brief.MaxAttempts {
		t.Fatalf("calls %d", len(m.Calls()))
	}
}
