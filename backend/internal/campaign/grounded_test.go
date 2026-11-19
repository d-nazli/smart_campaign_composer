package campaign_test

import (
	"context"
	"testing"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/campaign"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/segment"
)

// In the real model's mode (D87) the service gives the brief to the model even
// when the rule reader would have stopped it, and counts the model's audience.
// The same brief in the mock's (strict) mode is still asked about.
func TestGroundedModeLetsTheModelReadTheBrief(t *testing.T) {
	t.Parallel()
	const b = "Bu ay sepete ürün ekleyip almayanlara %10 indirim"
	modelRule := `{"name":"Test kampanyası","status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"lte","value":30},{"field":"purchased_since_last_cart_add","op":"eq","value":false}],"explanation":"Bu ay sepete ekleyip almayanlar","question":"","offer":{"kind":"percent","value":10,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}}`

	m := mock.NewScripted([]mock.Step{raw(modelRule)}, nil)
	svc, _ := newService(t, m)
	svc.UseSegmentMode(brief.Grounded)
	v, err := svc.Create(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Status != segment.StatusOK || v.Segment.Audience == nil || v.Segment.Audience.Matched == 0 || len(m.Calls()) != 1 {
		t.Fatalf("grounded: %+v (calls %d)", v.Segment, len(m.Calls()))
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

// A part of the brief the model keeps leaving out of the rule does not block
// the campaign: the audience step shows it as a warning, and the stored rule
// (warning included) is approved like any other (LLM-1, D111).
func TestGroundedADroppedConditionIsAWarningOnTheAudience(t *testing.T) {
	t.Parallel()
	const b = "Son 30 günde sepete ürün ekleyip almayanlara %10 indirim"
	noNotBought := `{"name":"Test kampanyası","status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"lte","value":30}],"explanation":"Son 30 günde sepete ekleyenler","question":"","offer":{"kind":"percent","value":10,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}}`

	m := mock.NewScripted([]mock.Step{raw(noNotBought)}, nil)
	svc, _ := newService(t, m)
	svc.UseSegmentMode(brief.Grounded)
	ctx := context.Background()
	v, err := svc.Create(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Status != segment.StatusOK || len(m.Calls()) != 2 || len(v.Segment.Warnings) != 1 {
		t.Fatalf("want an ok audience with one warning after one feedback: %+v (calls %d)", v.Segment, len(m.Calls()))
	}
	got, _ := svc.Get(ctx, v.ID)
	if len(got.Segment.Warnings) != 1 || got.Segment.Warnings[0] != v.Segment.Warnings[0] {
		t.Fatalf("the warning must be stored with the rule: %q", got.Segment.Warnings)
	}
	if err := svc.GenerateMessages(ctx, v.ID, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, v.ID); err != nil {
		t.Fatalf("a warning never blocks the approval: %v", err)
	}
}
