package campaign

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/content"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/mock"
	"smartcampaign/internal/testdb"
)

// Once a campaign is approved its text is what gets sent (the queue reads it at
// send time), so no write may change it afterwards: not a generation that was
// still running when the approval came, not an edit, not a translation.

// gatedLLM holds the next Messages call (once armed) until released, so that
// the campaign can be approved while the model is still writing.
type gatedLLM struct {
	llm.Client
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func newGatedLLM() *gatedLLM {
	return &gatedLLM{Client: mock.New(0), entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *gatedLLM) Messages(ctx context.Context, req llm.MessageRequest, onDelta func(string)) (llm.Output, error) {
	if g.armed.CompareAndSwap(true, false) {
		close(g.entered)
		select {
		case <-g.release:
		case <-ctx.Done():
			return llm.Output{}, ctx.Err()
		}
	}
	return g.Client.Messages(ctx, req, onDelta)
}

func raceService(t *testing.T, client llm.Client) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.NewSeeded(t)
	policy, err := content.NewPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(pool, client, policy, slog.New(slog.NewTextHandler(io.Discard, nil))), pool
}

// raceCampaign is a campaign in content_ready: Variant A written and translated.
func raceCampaign(t *testing.T, s *Service) int64 {
	t.Helper()
	ctx := context.Background()
	v, err := s.Create(ctx, "Son 30 günde sepete ürün ekleyip almayanlara %15 indirim hatırlatması gönder")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.GenerateMessages(ctx, v.ID, Events{}); err != nil {
		t.Fatal(err)
	}
	return v.ID
}

// contentRow is everything a write changes in one campaign_contents row.
type contentRow struct {
	SMS, Subject, Body, Source string
	Version                    int
	Updated                    time.Time
}

// variantRows returns a variant's rows by locale.
func variantRows(t *testing.T, pool *pgxpool.Pool, id int64, variant string) map[string]contentRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT locale, sms_body, email_subject, email_body, source, version, updated_at
		FROM campaign_contents WHERE campaign_id=$1 AND variant=$2`, id, variant)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]contentRow{}
	for rows.Next() {
		var code string
		var r contentRow
		if err := rows.Scan(&code, &r.SMS, &r.Subject, &r.Body, &r.Source, &r.Version, &r.Updated); err != nil {
			t.Fatal(err)
		}
		out[code] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func sameRows(a, b map[string]contentRow) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || !v.Updated.Equal(w.Updated) || v.SMS != w.SMS || v.Subject != w.Subject || v.Body != w.Body ||
			v.Source != w.Source || v.Version != w.Version {
			return false
		}
	}
	return true
}

func wantStateError(t *testing.T, err error) {
	t.Helper()
	var se *StateError
	if !errors.As(err, &se) {
		t.Errorf("want a StateError (409) after the approval, got %v", err)
	}
}

func variantCountsIsNull(t *testing.T, pool *pgxpool.Pool, id int64) bool {
	t.Helper()
	var null bool
	if err := pool.QueryRow(context.Background(), `SELECT variant_counts IS NULL FROM campaigns WHERE id=$1`, id).Scan(&null); err != nil {
		t.Fatal(err)
	}
	return null
}

// duringApproval runs write while an approval is half done: its transaction has
// changed the status but not committed yet. It waits until write either blocks
// on the campaign row or finishes, then commits the approval and returns
// write's error. A write that does not wait for the approval decides on the
// status from before it.
func duringApproval(t *testing.T, pool *pgxpool.Pool, id int64, write func() error) error {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET status='approved', approved_at=now(), updated_at=now() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- write() }()

	deadline := time.Now().Add(15 * time.Second)
	for {
		select {
		case err := <-done:
			if cerr := tx.Commit(ctx); cerr != nil {
				t.Fatal(cerr)
			}
			return err
		default:
		}
		var waiting bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the write neither finished nor waited for the approval")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return <-done
}

// "Yeniden üret" is running (the model can take minutes) when another tab
// approves the campaign: the late result must not replace the approved text.
func TestAGenerationThatEndsAfterTheApprovalStoresNothing(t *testing.T) {
	t.Parallel()
	g := newGatedLLM()
	s, pool := raceService(t, g)
	id := raceCampaign(t, s)
	ctx := context.Background()
	before := variantRows(t, pool, id, VariantA)

	g.armed.Store(true)
	done := make(chan error, 1)
	go func() { done <- s.GenerateMessages(ctx, id, Events{}) }()
	<-g.entered
	if _, err := s.Approve(ctx, id); err != nil {
		t.Fatalf("approve while generating: %v", err)
	}
	close(g.release)

	wantStateError(t, <-done)
	if after := variantRows(t, pool, id, VariantA); !sameRows(before, after) {
		t.Fatalf("the approved text changed after the approval:\nbefore %+v\nafter  %+v", before, after)
	}
}

// Variant B is being written when the campaign is approved with A alone: B must
// not appear in the approved campaign, and its counts must not be stored.
func TestVariantBFinishedAfterTheApprovalIsNotAdded(t *testing.T) {
	t.Parallel()
	g := newGatedLLM()
	s, pool := raceService(t, g)
	id := raceCampaign(t, s)
	ctx := context.Background()

	g.armed.Store(true)
	done := make(chan error, 1)
	go func() { done <- s.GenerateVariant(ctx, id, VariantB, Events{}) }()
	<-g.entered
	if _, err := s.Approve(ctx, id); err != nil {
		t.Fatalf("approve while writing B: %v", err)
	}
	close(g.release)

	wantStateError(t, <-done)
	if b := variantRows(t, pool, id, VariantB); len(b) != 0 {
		t.Fatalf("variant B was added to an approved campaign: %+v", b)
	}
	if !variantCountsIsNull(t, pool, id) {
		t.Fatal("variant counts were stored for an approved campaign")
	}
}

// An edit saved while the approval is committing either lands before it (and
// the approval sees it) or is refused; it never lands after.
func TestAnEditRacingTheApprovalIsRefused(t *testing.T) {
	t.Parallel()
	s, pool := raceService(t, mock.New(0))
	id := raceCampaign(t, s)
	ctx := context.Background()
	before := variantRows(t, pool, id, VariantA)

	err := duringApproval(t, pool, id, func() error {
		_, err := s.SaveContent(ctx, id, content.Content{SMSBody: "Merhaba {{first_name}}!", EmailSubject: "Konu", EmailBody: "Gövde"})
		return err
	})
	wantStateError(t, err)
	if after := variantRows(t, pool, id, VariantA); !sameRows(before, after) {
		t.Fatalf("an edit changed the approved text:\nbefore %+v\nafter  %+v", before, after)
	}
}

// The variant counts are not rewritten once the approval has fixed them.
func TestVariantCountsAreNotStoredAfterTheApproval(t *testing.T) {
	t.Parallel()
	s, pool := raceService(t, mock.New(0))
	id := raceCampaign(t, s)
	ctx := context.Background()
	row, err := s.loadRow(ctx, s.pool, id)
	if err != nil {
		t.Fatal(err)
	}

	wantStateError(t, duringApproval(t, pool, id, func() error { return s.storeVariantCounts(ctx, id, row) }))
	if !variantCountsIsNull(t, pool, id) {
		t.Fatal("variant counts were stored for an approved campaign")
	}
}

// A translation finishing while the approval commits is not stored after it.
func TestATranslationRacingTheApprovalIsNotStored(t *testing.T) {
	t.Parallel()
	s, pool := raceService(t, mock.New(0))
	id := raceCampaign(t, s)
	ctx := context.Background()
	ls, err := s.loadLocales(ctx)
	if err != nil {
		t.Fatal(err)
	}
	src := ls.Source().Code
	var target string
	if err := pool.QueryRow(ctx, `SELECT code FROM locales WHERE code <> $1 ORDER BY code LIMIT 1`, src).Scan(&target); err != nil {
		t.Fatal(err)
	}
	version := variantRows(t, pool, id, VariantA)[src].Version
	before := variantRows(t, pool, id, VariantA)

	err = duringApproval(t, pool, id, func() error {
		return s.writeTranslation(ctx, id, VariantA, src, target, content.Content{SMSBody: "x", EmailSubject: "y", EmailBody: "z"}, version)
	})
	wantStateError(t, err)
	if after := variantRows(t, pool, id, VariantA); !sameRows(before, after) {
		t.Fatalf("a translation was stored after the approval:\nbefore %+v\nafter  %+v", before, after)
	}
}

// Translations of one campaign run in parallel and must not wait for each
// other: two open translation writes of the same variant both get through.
func TestParallelTranslationsDoNotWaitForEachOther(t *testing.T) {
	t.Parallel()
	s, pool := raceService(t, mock.New(0))
	id := raceCampaign(t, s)
	ctx := context.Background()
	ls, err := s.loadLocales(ctx)
	if err != nil {
		t.Fatal(err)
	}
	src := ls.Source().Code
	version := variantRows(t, pool, id, VariantA)[src].Version

	// A translation transaction left open holds whatever writeTranslation locks.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM campaigns WHERE id=$1 FOR SHARE`, id); err != nil {
		t.Fatal(err)
	}
	var target string
	if err := pool.QueryRow(ctx, `SELECT code FROM locales WHERE code <> $1 ORDER BY code LIMIT 1`, src).Scan(&target); err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.writeTranslation(wctx, id, VariantA, src, target, content.Content{SMSBody: "x", EmailSubject: "y", EmailBody: "z"}, version); err != nil {
		t.Fatalf("a translation waited for another one: %v", err)
	}
}
