package campaign_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/llm/mock"
)

type auditRow struct {
	Attempt           int
	Valid, Refused    bool
	FellBack          bool
	InputTokens, Rows int
}

func auditRows(t *testing.T, pool *pgxpool.Pool, id int64, kind string) []auditRow {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT attempt, valid, refused, fell_back FROM llm_generations WHERE campaign_id=$1 AND kind=$2 AND locale='tr-TR' ORDER BY id`, id, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.Attempt, &r.Valid, &r.Refused, &r.FellBack); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// The metrics count refusals and fallbacks from these flags, so they must be
// written exactly when those things happen, and only then.

func TestAFallbackIsMarkedOnTheLastAttemptOfItsRunOnly(t *testing.T) {
	t.Parallel()
	bad := msgs("kısa ama teklif yok", "Konu", "Gövde")
	svc, pool := newService(t, mock.NewScripted(nil, []mock.Step{raw(bad)})) // repeats: 3 invalid attempts
	id := offerCampaign(t, svc)
	if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	rows := auditRows(t, pool, id, "messages")
	if len(rows) != 3 {
		t.Fatalf("attempts: %+v", rows)
	}
	for i, r := range rows {
		if r.Refused || r.Valid || r.FellBack != (i == 2) {
			t.Fatalf("attempt %d: %+v (only the last one may be marked fell_back)", i+1, r)
		}
	}
}

func TestARefusedRunIsMarkedRefusedAndFellBack(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.NewScripted(nil, []mock.Step{{Kind: mock.Refusal}}))
	id := offerCampaign(t, svc)
	if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	rows := auditRows(t, pool, id, "messages")
	if len(rows) != 1 || !rows[0].Refused || !rows[0].FellBack || rows[0].Valid {
		t.Fatalf("%+v", rows)
	}
}

func TestARepairedRunIsNotAFallback(t *testing.T) {
	t.Parallel()
	bad := msgs("kısa ama teklif yok", "Konu", "Gövde")
	svc, pool := newService(t, mock.NewScripted(nil, []mock.Step{raw(bad), {Kind: mock.Smart}}))
	id := offerCampaign(t, svc)
	if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
		t.Fatal(err)
	}
	rows := auditRows(t, pool, id, "messages")
	if len(rows) != 2 || rows[0].Valid || !rows[1].Valid || rows[0].FellBack || rows[1].FellBack {
		t.Fatalf("%+v", rows)
	}
}

// Only the run that just ended is marked: an older fallback keeps its mark and
// a newer, successful run stays clean.
func TestRegeneratingAfterAFallbackDoesNotMoveTheMark(t *testing.T) {
	t.Parallel()
	bad := msgs("kısa ama teklif yok", "Konu", "Gövde")
	svc, pool := newService(t, mock.NewScripted(nil, []mock.Step{raw(bad), raw(bad), raw(bad), {Kind: mock.Smart}}))
	id := offerCampaign(t, svc)
	for i := 0; i < 2; i++ {
		if err := svc.GenerateMessages(context.Background(), id, campaign.Events{}); err != nil {
			t.Fatal(err)
		}
	}
	rows := auditRows(t, pool, id, "messages")
	if len(rows) != 4 {
		t.Fatalf("%+v", rows)
	}
	marked := 0
	for i, r := range rows {
		if r.FellBack {
			marked++
			if i != 2 {
				t.Fatalf("mark on row %d: %+v", i, rows)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("%+v", rows)
	}
}

func TestARefusedSegmentIsMarkedRefused(t *testing.T) {
	t.Parallel()
	svc, pool := newService(t, mock.NewScripted([]mock.Step{{Kind: mock.Refusal}}, nil))
	_, err := svc.Create(context.Background(), "Son 30 günde sepete ürün ekleyip almayanlara %15 indirim hatırlatması gönder")
	asGenErr(t, err)
	var id int64
	if err := pool.QueryRow(context.Background(), `SELECT campaign_id FROM llm_generations WHERE kind='segment'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	rows := auditRows(t, pool, id, "segment")
	if len(rows) != 1 || !rows[0].Refused || rows[0].FellBack {
		t.Fatalf("%+v", rows)
	}
}
