package campaign_test

import (
	"context"
	"testing"

	"smartcampaign/internal/brief"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/segment"
)

// In the real model's mode (D87) the service gives the brief to the model even
// when the rule reader would have stopped it, and counts the model's audience.
// The same brief in the mock's (strict) mode is still asked about.
func TestGroundedModeLetsTheModelReadTheBrief(t *testing.T) {
	const b = "Bu ay sepete ürün ekleyip almayanlara %10 indirim"
	modelRule := `{"status":"ok","match":"all","conditions":[{"field":"days_since_last_cart_add","op":"lte","value":30},{"field":"purchased_since_last_cart_add","op":"eq","value":false}],"explanation":"Bu ay sepete ekleyip almayanlar","question":"","offer":{"kind":"percent","value":10,"code":"","min_spend":0,"expires_at":"","step_spend":0,"max_discount":0,"extras":[]}}`

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
