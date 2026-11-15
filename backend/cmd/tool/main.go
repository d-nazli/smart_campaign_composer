// Command tool runs one-shot maintenance steps in order: `tool migrate seed`.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/config"
	"smartcampaign/internal/httpx"
	"smartcampaign/internal/seed"
	"smartcampaign/internal/store"
)

func main() {
	log := httpx.NewLogger("tool")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	steps := os.Args[1:]
	if len(steps) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tool <migrate|seed>...")
		os.Exit(2)
	}
	dsn := config.Env("DATABASE_URL", "postgres://campaign:campaign@localhost:5432/campaign")

	for _, step := range steps {
		switch step {
		case "migrate":
			if err := store.Migrate(ctx, dsn); err != nil {
				log.Error("migrate failed", "error", err)
				os.Exit(1)
			}
			log.Info("migrations applied")

		case "seed":
			seedValue, err := strconv.ParseUint(config.Env("SEED", "42"), 10, 64)
			if err != nil {
				log.Error("SEED must be an unsigned integer", "error", err)
				os.Exit(1)
			}
			users, err := strconv.Atoi(config.Env("SEED_USERS", strconv.Itoa(seed.DefaultUsers)))
			if err != nil {
				log.Error("SEED_USERS must be an integer", "error", err)
				os.Exit(1)
			}
			pool, err := pgxpool.New(ctx, dsn)
			if err != nil {
				log.Error("invalid DATABASE_URL", "error", err)
				os.Exit(1)
			}
			runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			_, err = seed.Run(runCtx, pool, seedValue, time.Now(), users, log)
			cancel()
			pool.Close()
			if err != nil {
				log.Error("seed failed", "error", err)
				os.Exit(1)
			}

		default:
			log.Error("unknown step", "step", step)
			os.Exit(2)
		}
	}
	log.Info("done")
}
