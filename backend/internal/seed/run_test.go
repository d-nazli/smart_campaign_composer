package seed_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"smartcampaign/internal/seed"
	"smartcampaign/internal/store"
	"smartcampaign/internal/testdb"
)

var (
	now = time.Now().UTC().Truncate(time.Second)
	log = slog.New(slog.NewTextHandler(io.Discard, nil))
)

func TestRunSeedsOnceAndIsIdempotent(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()

	res, err := seed.Run(ctx, pool, 42, now, seed.DefaultUsers, log)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped || res.Users != 5000 {
		t.Fatalf("first run: %+v", res)
	}

	again, err := seed.Run(ctx, pool, 42, now, seed.DefaultUsers, log)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Skipped {
		t.Fatalf("second run must be skipped: %+v", again)
	}

	var users int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 5000 {
		t.Fatalf("users = %d, want 5000", users)
	}
}

func TestConcurrentRunsSeedExactlyOnce(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	results := make([]seed.Result, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = seed.Run(ctx, pool, 42, now, seed.DefaultUsers, log)
		}()
	}
	wg.Wait()

	seeded := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if !results[i].Skipped {
			seeded++
		}
	}
	if seeded != 1 {
		t.Fatalf("%d runs inserted data, want exactly 1", seeded)
	}
	var users int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&users)
	if users != 5000 {
		t.Fatalf("users = %d, want 5000", users)
	}
}

func TestDerivedColumnsMatchRawTablesInDB(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, 42, now, seed.DefaultUsers, log); err != nil {
		t.Fatal(err)
	}

	var mismatches int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM users u WHERE
		     u.order_count <> (SELECT count(*) FROM orders o WHERE o.user_id = u.id)
		  OR u.total_spent <> COALESCE((SELECT sum(total) FROM orders o WHERE o.user_id = u.id), 0)
		  OR u.last_order_at    IS DISTINCT FROM (SELECT max(created_at) FROM orders o WHERE o.user_id = u.id)
		  OR u.last_cart_add_at IS DISTINCT FROM (SELECT max(added_at) FROM cart_events c WHERE c.user_id = u.id)
	`).Scan(&mismatches)
	if err != nil {
		t.Fatal(err)
	}
	if mismatches != 0 {
		t.Fatalf("%d users have derived columns that disagree with orders/cart_events", mismatches)
	}
}

func TestBriefCohortQueryReturnsKnownCount(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, 42, now, seed.DefaultUsers, log); err != nil {
		t.Fatal(err)
	}

	var cohort int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM users
		WHERE city = $1
		  AND last_cart_add_at >= $2::timestamptz - make_interval(days => $3)
		  AND (last_order_at IS NULL OR last_order_at < last_cart_add_at)
	`, seed.BriefCity, now, seed.BriefWindowDays).Scan(&cohort)
	if err != nil {
		t.Fatal(err)
	}
	if cohort != 300 {
		t.Fatalf("brief cohort = %d, want 300", cohort)
	}
}

func TestSequencesAdvancedPastSeededIDs(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	if _, err := seed.Run(ctx, pool, 42, now, seed.DefaultUsers, log); err != nil {
		t.Fatal(err)
	}
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO users (first_name, last_name, city, sms_consent, email_consent, created_at)
		VALUES ('Test', 'User', 'Ankara', false, false, now()) RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("insert after seed: %v", err)
	}
	if id != 5001 {
		t.Fatalf("next id = %d, want 5001", id)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	_, dsn := testdb.New(t) // testdb already migrated once
	if err := store.Migrate(context.Background(), dsn); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
