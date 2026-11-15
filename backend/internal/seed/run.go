package seed

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockKey is the Postgres advisory lock that serialises seeding.
const lockKey int64 = 0x5eed_c0de

// Result describes what Run did.
type Result struct {
	Skipped    bool
	Users      int
	CartEvents int
	Orders     int
}

// Run loads the dataset unless users already exist. It is idempotent and safe
// to call concurrently: callers queue on an advisory lock, and everyone after
// the first sees a non-empty users table and skips.
//
// Everything is inserted in a single transaction, so a crash never leaves a
// half-seeded database that the "users not empty" check would then accept.
func Run(ctx context.Context, pool *pgxpool.Pool, seed uint64, now time.Time, n int, log *slog.Logger) (Result, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return Result{}, fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		// Background context: unlock even if ctx was cancelled.
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
	}()

	var existing int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&existing); err != nil {
		return Result{}, fmt.Errorf("count users: %w", err)
	}
	if existing > 0 {
		log.Info("seed skipped: users table is not empty", "users", existing)
		return Result{Skipped: true, Users: existing}, nil
	}

	ds, err := Generate(seed, now, n)
	if err != nil {
		return Result{}, err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := copyUsers(ctx, tx, ds.Users); err != nil {
		return Result{}, err
	}
	if err := copyCarts(ctx, tx, ds.CartEvents); err != nil {
		return Result{}, err
	}
	if err := copyOrders(ctx, tx, ds.Orders); err != nil {
		return Result{}, err
	}
	// We inserted explicit ids, so move the identity sequences past them.
	for _, table := range []string{"users", "cart_events", "orders"} {
		q := fmt.Sprintf("SELECT setval(pg_get_serial_sequence('%s', 'id'), (SELECT max(id) FROM %s))", table, table)
		if _, err := tx.Exec(ctx, q); err != nil {
			return Result{}, fmt.Errorf("advance %s sequence: %w", table, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit seed: %w", err)
	}

	res := Result{Users: len(ds.Users), CartEvents: len(ds.CartEvents), Orders: len(ds.Orders)}
	log.Info("seed loaded",
		"seed", seed, "users", res.Users, "cart_events", res.CartEvents, "orders", res.Orders,
		"brief_cohort", ds.CohortCount(now))
	return res, nil
}

func copyUsers(ctx context.Context, tx pgx.Tx, users []User) error {
	cols := []string{"id", "first_name", "last_name", "email", "phone", "city", "sms_consent", "email_consent",
		"last_cart_add_at", "last_order_at", "order_count", "total_spent", "created_at"}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"users"}, cols, pgx.CopyFromSlice(len(users), func(i int) ([]any, error) {
		u := users[i]
		return []any{u.ID, u.FirstName, u.LastName, nullableString(u.Email), nullableString(u.Phone), u.City,
			u.SMSConsent, u.EmailConsent, nullableTime(u.LastCartAddAt), nullableTime(u.LastOrderAt),
			int32(u.OrderCount), cents(u.TotalSpentCents), u.CreatedAt}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy users: %w", err)
	}
	return nil
}

func copyCarts(ctx context.Context, tx pgx.Tx, events []CartEvent) error {
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"cart_events"}, []string{"id", "user_id", "product_name", "added_at"},
		pgx.CopyFromSlice(len(events), func(i int) ([]any, error) {
			e := events[i]
			return []any{e.ID, e.UserID, e.Product, e.AddedAt}, nil
		}))
	if err != nil {
		return fmt.Errorf("copy cart_events: %w", err)
	}
	return nil
}

func copyOrders(ctx context.Context, tx pgx.Tx, orders []Order) error {
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"orders"}, []string{"id", "user_id", "total", "created_at"},
		pgx.CopyFromSlice(len(orders), func(i int) ([]any, error) {
			o := orders[i]
			return []any{o.ID, o.UserID, cents(o.TotalCents), o.CreatedAt}, nil
		}))
	if err != nil {
		return fmt.Errorf("copy orders: %w", err)
	}
	return nil
}

// cents converts an integer amount of cents to an exact NUMERIC value.
func cents(c int64) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(c), Exp: -2, Valid: true}
}

func nullableString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
