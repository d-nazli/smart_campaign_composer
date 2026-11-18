package store_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"smartcampaign/internal/store"
	"smartcampaign/internal/testdb"
)

// 00009 moves users.language (tr | en | de) to users.locale and every stored
// text, audit row and audience count with it; 00010 turns a translation a
// person had edited into a stale translation (translations are read-only,
// D97). Going back restores the old columns for the locales that existed before.
func TestTheLocalesMigrationMovesTheOldLanguages(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.NewEmpty(t)
	if err := store.MigrateTo(ctx, dsn, 8); err != nil {
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
	exec(`INSERT INTO users (id, first_name, last_name, city, sms_consent, email_consent, created_at, language) VALUES
		(1, 'A', 'B', 'İstanbul', true, true, now(), 'tr'),
		(2, 'C', 'D', 'İzmir', true, true, now(), 'en'),
		(3, 'E', 'F', 'Ankara', true, true, now(), 'de')`)
	exec(`INSERT INTO campaigns (id, brief, status, language_counts) VALUES (1, 'b', 'content_ready', '{"tr": 1, "en": 1, "de": 1}')`)
	exec(`INSERT INTO campaign_contents (campaign_id, language, sms_body, email_subject, email_body, source) VALUES
		(1, 'tr', 'tr sms', 'tr konu', 'tr metin', 'llm'),
		(1, 'en', 'en sms', 'en subject', 'en body', 'llm'),
		(1, 'de', 'de sms', 'de Betreff', 'de Text', 'edited')`)
	exec(`INSERT INTO llm_generations (campaign_id, kind, attempt, mode, model, valid, problems, raw_output, language)
		VALUES (1, 'messages', 1, 'mock', 'mock', true, '[]', '{}', 'en')`)

	if err := store.Migrate(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	locales := map[int]string{}
	rows, _ := db.Query(ctx, `SELECT id, locale FROM users ORDER BY id`)
	for rows.Next() {
		var id int
		var code string
		_ = rows.Scan(&id, &code)
		locales[id] = code
	}
	rows.Close()
	if locales[1] != "tr-TR" || locales[2] != "en-GB" || locales[3] != "de-DE" {
		t.Fatalf("users: %v", locales)
	}
	type row struct {
		source        string
		version       int
		sourceVersion *int
	}
	got := map[string]row{}
	rows, _ = db.Query(ctx, `SELECT locale, source, version, source_version FROM campaign_contents WHERE campaign_id=1`)
	for rows.Next() {
		var code string
		var r row
		_ = rows.Scan(&code, &r.source, &r.version, &r.sourceVersion)
		got[code] = r
	}
	rows.Close()
	if len(got) != 3 || got["tr-TR"].source != "llm" || got["tr-TR"].sourceVersion != nil ||
		got["en-GB"].source != "translated" || *got["en-GB"].sourceVersion != 1 ||
		got["de-DE"].source != "translated" || *got["de-DE"].sourceVersion != 0 {
		t.Fatalf("contents: %+v", got)
	}
	var counts, auditLocale string
	_ = db.QueryRow(ctx, `SELECT locale_counts::text FROM campaigns WHERE id=1`).Scan(&counts)
	_ = db.QueryRow(ctx, `SELECT locale FROM llm_generations`).Scan(&auditLocale)
	if counts != `{"de-DE": 1, "en-GB": 1, "tr-TR": 1}` || auditLocale != "en-GB" {
		t.Fatalf("counts %s, audit %s", counts, auditLocale)
	}
	var left int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE column_name IN ('language', 'language_counts') AND table_schema='public'`).Scan(&left)
	if left != 0 {
		t.Fatalf("%d old language columns left", left)
	}

	// And back.
	if err := store.MigrateTo(ctx, dsn, 8); err != nil {
		t.Fatal(err)
	}
	var lang string
	_ = db.QueryRow(ctx, `SELECT language FROM users WHERE id=2`).Scan(&lang)
	_ = db.QueryRow(ctx, `SELECT language_counts::text FROM campaigns WHERE id=1`).Scan(&counts)
	if lang != "en" || counts != `{"de": 1, "en": 1, "tr": 1}` {
		t.Fatalf("down: language %q counts %s", lang, counts)
	}
}
