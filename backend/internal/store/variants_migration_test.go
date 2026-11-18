package store_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"smartcampaign/internal/store"
	"smartcampaign/internal/testdb"
)

// 00012 makes every stored text and delivery Variant A; going back drops
// Variant B and restores the old key.
func TestTheVariantsMigrationKeepsOldCampaignsAsVariantA(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.NewEmpty(t)
	if err := store.MigrateTo(ctx, dsn, 11); err != nil {
		t.Fatal(err)
	}
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO users (id, first_name, last_name, city, sms_consent, email_consent, created_at) VALUES (1, 'Ali', 'Kaya', 'İstanbul', true, true, now())`)
	exec(`INSERT INTO campaigns (id, brief, status) VALUES (1, 'b', 'content_ready')`)
	exec(`INSERT INTO campaign_contents (campaign_id, locale, sms_body, email_subject, email_body, source) VALUES (1, 'tr-TR', 's', 'k', 'g', 'llm')`)
	exec(`INSERT INTO deliveries (campaign_id, user_id, channel) VALUES (1, 1, 'sms')`)

	if err := store.MigrateTo(ctx, dsn, 12); err != nil {
		t.Fatal(err)
	}
	var cv, dv string
	if err := db.QueryRow(ctx, `SELECT (SELECT variant FROM campaign_contents), (SELECT variant FROM deliveries)`).Scan(&cv, &dv); err != nil {
		t.Fatal(err)
	}
	if cv != "A" || dv != "A" {
		t.Fatalf("content %s, delivery %s", cv, dv)
	}
	// B is a row of its own; a third variant is refused.
	exec(`INSERT INTO campaign_contents (campaign_id, variant, locale, sms_body, email_subject, email_body, source) VALUES (1, 'B', 'tr-TR', 's2', 'k2', 'g2', 'llm')`)
	if _, err := db.Exec(ctx, `INSERT INTO campaign_contents (campaign_id, variant, locale, sms_body, email_subject, email_body, source) VALUES (1, 'C', 'tr-TR', 's', 'k', 'g', 'llm')`); err == nil {
		t.Fatal("variant C was accepted")
	}

	if err := store.MigrateTo(ctx, dsn, 11); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM campaign_contents`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("after down: %d texts", n)
	}
}
