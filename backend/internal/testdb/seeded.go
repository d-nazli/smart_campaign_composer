package testdb

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/seed"
)

// NewSeeded is New plus the standard 5000-user seed (SEED=42).
func NewSeeded(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := New(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := seed.Run(context.Background(), pool, 42, time.Now().UTC().Truncate(time.Second), seed.DefaultUsers, log); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return pool
}
