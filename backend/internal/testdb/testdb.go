// Package testdb gives integration tests a private, migrated Postgres database.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/store"
)

// New creates a fresh database named test_<random> on the server given by
// TEST_DATABASE_URL, applies all migrations and returns a pool for it. The
// database is dropped when the test ends. Without TEST_DATABASE_URL the test
// is skipped (docker compose's backend-test service always sets it).
func New(t *testing.T) (pool *pgxpool.Pool, dsn string) {
	t.Helper()
	adminDSN := os.Getenv("TEST_DATABASE_URL")
	if adminDSN == "" {
		t.Skip("TEST_DATABASE_URL not set; run integration tests via `make test`")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect to test server: %v", err)
	}
	defer admin.Close(ctx)

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}

	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	dsn = u.String()

	if err := store.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		c, err := pgx.Connect(cctx, adminDSN)
		if err != nil {
			return
		}
		defer c.Close(cctx)
		_, _ = c.Exec(cctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return pool, dsn
}
