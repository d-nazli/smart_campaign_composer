// Package store owns the database schema.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies all pending migrations. It is safe to run concurrently from
// several processes: goose holds a Postgres session advisory lock while
// migrating, so the second caller waits and then finds nothing to do.
func Migrate(ctx context.Context, dsn string) error {
	return withProvider(dsn, func(p *goose.Provider) error {
		_, err := p.Up(ctx)
		return err
	})
}

// MigrateTo moves the schema up or down to exactly version (tests of a
// migration's data handling use it).
func MigrateTo(ctx context.Context, dsn string, version int64) error {
	return withProvider(dsn, func(p *goose.Provider) error {
		current, err := p.GetDBVersion(ctx)
		if err != nil {
			return err
		}
		if version >= current {
			_, err = p.UpTo(ctx, version)
		} else {
			_, err = p.DownTo(ctx, version)
		}
		return err
	})
}

func withProvider(dsn string, f func(*goose.Provider) error) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create session locker: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, sub, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if err := f(provider); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
