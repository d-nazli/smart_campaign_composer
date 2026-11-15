// Command tool runs one-shot maintenance steps: `tool migrate seed`.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/config"
	"smartcampaign/internal/httpx"
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

	pool, err := pgxpool.New(ctx, config.Env("DATABASE_URL", "postgres://campaign:campaign@localhost:5432/campaign"))
	if err != nil {
		log.Error("invalid DATABASE_URL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		log.Error("database unreachable", "error", err)
		os.Exit(1)
	}

	for _, step := range steps {
		switch step {
		case "migrate":
			log.Info("migrate: no migrations yet (added in phase 1)")
		case "seed":
			log.Info("seed: nothing to seed yet (added in phase 1)")
		default:
			log.Error("unknown step", "step", step)
			os.Exit(2)
		}
	}
	log.Info("done")
}
