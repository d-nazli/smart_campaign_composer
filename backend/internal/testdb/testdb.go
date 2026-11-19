// Package testdb gives integration tests a private, migrated Postgres database.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/store"
)

// New creates a fresh database named test_<random> on the server given by
// TEST_DATABASE_URL, applies all migrations and returns a pool for it. The
// database is dropped when the test ends. Without TEST_DATABASE_URL the test
// fails, so that `go test` on a machine without the database never looks
// green (docker compose's backend-test service always sets it); with
// SKIP_DB_TESTS=1 it is skipped instead.
func New(t *testing.T) (pool *pgxpool.Pool, dsn string) {
	t.Helper()
	dsn = NewEmpty(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, dsn
}

// NewEmpty creates a throwaway database without any migration and returns its
// address. It is dropped when the test ends.
func NewEmpty(t *testing.T) (dsn string) {
	t.Helper()
	return create(t, "")
}

// create makes a throwaway database, a copy of template when it is not "",
// and drops it when the test ends.
func create(t *testing.T, template string) (dsn string) {
	t.Helper()
	adminDSN := os.Getenv("TEST_DATABASE_URL")
	if adminDSN == "" {
		if os.Getenv("SKIP_DB_TESTS") == "1" {
			t.Skip("SKIP_DB_TESTS=1: veritabanı testleri atlandı")
		}
		t.Fatal("TEST_DATABASE_URL tanımlı değil; testleri 'make test' ile çalıştırın.")
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
	stmt := "CREATE DATABASE " + name
	if template != "" {
		stmt += " TEMPLATE " + template
	}
	if err := createDatabase(ctx, admin, stmt); err != nil {
		t.Fatalf("create database: %v", err)
	}

	dsn, err = withDatabase(adminDSN, name)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		c, err := pgx.Connect(cctx, adminDSN)
		if err != nil {
			return
		}
		defer c.Close(cctx)
		_, _ = c.Exec(cctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return dsn
}

// createDatabase runs CREATE DATABASE, again a few times if the template is
// briefly busy (SQLSTATE 55006: another session is copying it).
func createDatabase(ctx context.Context, admin *pgx.Conn, stmt string) error {
	var err error
	for i := 0; i < 20; i++ {
		if _, err = admin.Exec(ctx, stmt); err == nil {
			return nil
		}
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "55006" {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// withDatabase is the server address dsn pointing at database name.
func withDatabase(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}
