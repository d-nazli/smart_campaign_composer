package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/clock"
	"smartcampaign/internal/config"
	"smartcampaign/internal/content"
	"smartcampaign/internal/httpx"
	"smartcampaign/internal/queue"
	"smartcampaign/internal/worker"
)

func main() {
	log := httpx.NewLogger("worker")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := worker.LoadConfig(os.Getenv)
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	policy, err := content.PolicyFromEnv(os.Getenv)
	if err != nil {
		log.Error("invalid message variable budgets", "error", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, config.Env("DATABASE_URL", "postgres://campaign:campaign@localhost:5432/campaign"))
	if err != nil {
		log.Error("invalid DATABASE_URL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	providerURL := config.Env("PROVIDER_URL", "http://localhost:8090")
	w := worker.New(queue.NewPG(pool), worker.NewHTTPSender(providerURL, cfg.RequestTimeout), policy, clock.Real{}, log, cfg)
	log.Info("worker started", "provider_url", providerURL, "rate_per_second", cfg.RatePerSecond,
		"concurrency", cfg.Concurrency, "max_attempts", cfg.Policy.MaxAttempts)

	go func() {
		if err := w.Run(ctx); err != nil && ctx.Err() == nil {
			log.Error("worker stopped", "error", err)
			stop()
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Healthz)
	srv := &http.Server{
		Addr:              config.Env("WORKER_HTTP_ADDR", ":9091"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := httpx.Serve(ctx, srv, log); err != nil {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}
