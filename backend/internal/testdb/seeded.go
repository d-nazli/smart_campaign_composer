package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/seed"
	"smartcampaign/internal/store"
)

// NewSeeded is New plus the standard 5000-user seed (SEED=42).
//
// Seeding 5000 users for every test was most of the suite's time. The seeded
// database is now built once per test binary (one package) and every test gets
// a copy of it (CREATE DATABASE ... TEMPLATE): the same data, private to the
// test, dropped when it ends.
func NewSeeded(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		NewEmpty(t) // fails (or skips) with the usual message
		return nil
	}
	template, err := seededTemplate(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("seeded template: %v", err)
	}
	dsn := create(t, template)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// templatePrefix names the seeded templates: tmpl_seeded_<unix time>_<random>.
const templatePrefix = "tmpl_seeded_"

// staleTemplate is the age after which a template left behind by an earlier
// test run (a test binary cannot drop its own at exit) is dropped.
const staleTemplate = 30 * time.Minute

var tmpl struct {
	once sync.Once
	name string
	err  error
}

func seededTemplate(adminDSN string) (string, error) {
	tmpl.once.Do(func() { tmpl.name, tmpl.err = buildTemplate(adminDSN) })
	return tmpl.name, tmpl.err
}

func buildTemplate(adminDSN string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return "", fmt.Errorf("connect to test server: %w", err)
	}
	defer admin.Close(ctx)
	dropStaleTemplates(ctx, admin)

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s%d_%s", templatePrefix, time.Now().Unix(), hex.EncodeToString(suffix))
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		return "", fmt.Errorf("create template: %w", err)
	}
	dsn, err := withDatabase(adminDSN, name)
	if err != nil {
		return "", err
	}
	if err := store.Migrate(ctx, dsn); err != nil {
		return "", fmt.Errorf("migrate template: %w", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return "", err
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err = seed.Run(ctx, pool, 42, time.Now().UTC().Truncate(time.Second), seed.DefaultUsers, log)
	pool.Close()
	if err != nil {
		return "", fmt.Errorf("seed template: %w", err)
	}
	// A database with a session open cannot be copied: nobody may connect to it.
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+name+" ALLOW_CONNECTIONS false"); err != nil {
		return "", fmt.Errorf("close template: %w", err)
	}
	return name, nil
}

// dropStaleTemplates drops the seeded templates of earlier runs.
func dropStaleTemplates(ctx context.Context, admin *pgx.Conn) {
	rows, err := admin.Query(ctx, `SELECT datname FROM pg_database WHERE datname LIKE 'tmpl\_seeded\_%'`)
	if err != nil {
		return
	}
	var stale []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(name, templatePrefix), "_")
		if sec, err := strconv.ParseInt(parts[0], 10, 64); err == nil && time.Since(time.Unix(sec, 0)) > staleTemplate {
			stale = append(stale, name)
		}
	}
	rows.Close()
	for _, name := range stale {
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	}
}
