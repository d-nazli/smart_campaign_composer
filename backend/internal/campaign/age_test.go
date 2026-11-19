package campaign_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/segment"
)

// The mock flow end to end: an age group without numbers is asked about; the
// answer with the numbers resolves the same campaign, and the audience is the
// exact number PostgreSQL's age() gives.
func TestAVagueAgeIsAskedAndTheAnswerGivesAnExactAudience(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.New(0))
	ctx := context.Background()

	v, err := svc.Create(ctx, "Gençlere özel %10 indirim")
	if err != nil {
		t.Fatal(err)
	}
	if v.Segment.Status != segment.StatusNeedsClarification || !strings.Contains(v.Segment.Question, "yaş aralığı") || v.Segment.Audience != nil {
		t.Fatalf("'gençlere' must be asked about: %+v", v.Segment)
	}

	got, err := svc.Clarify(ctx, v.ID, "18-25 yaş arası")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != v.ID || got.Status != campaign.StatusSegmentReady || got.Segment.Status != segment.StatusOK {
		t.Fatalf("the answer resolves the same campaign: %+v", got.Segment)
	}
	var conds []string
	for _, c := range got.Segment.Conditions {
		conds = append(conds, c.Field+" "+c.Op)
	}
	if strings.Join(conds, ", ") != "age gte, age lte" {
		t.Fatalf("conditions: %+v", got.Segment.Conditions)
	}

	var want int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users
		WHERE extract(year FROM age($1::date, birth_date))::int BETWEEN 18 AND 25`, time.Now().UTC().Format(time.DateOnly)).Scan(&want); err != nil {
		t.Fatal(err)
	}
	if want == 0 || got.Segment.Audience == nil || got.Segment.Audience.Matched != want {
		t.Fatalf("audience %+v, want exactly %d", got.Segment.Audience, want)
	}
}
