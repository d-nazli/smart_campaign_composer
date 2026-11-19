package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/campaign"
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

	poolCfg, err := pgxpool.ParseConfig(config.Env("DATABASE_URL", "postgres://campaign:campaign@localhost:5432/campaign"))
	if err != nil {
		log.Error("invalid DATABASE_URL", "error", err)
		os.Exit(1)
	}
	// Every send writes its result (two small statements). With the default pool
	// (a few connections) those writes queue up behind each other and cap the
	// send rate, so size it for the senders of both channels.
	if want := int32(cfg.Concurrency/2 + 4); poolCfg.MaxConns < want {
		poolCfg.MaxConns = want
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Error("cannot create database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	providerURL := config.Env("PROVIDER_URL", "http://localhost:8090")
	w := worker.New(queue.NewPG(pool), worker.NewHTTPSender(providerURL, cfg.RequestTimeout), policy, clock.Real{}, log, cfg)
	log.Info("worker started", "provider_url", providerURL, "rate_per_second", cfg.RatePerSecond,
		"concurrency", cfg.Concurrency, "max_attempts", cfg.Policy.MaxAttempts,
		"budget_first_name", policy.Budget("first_name"), "budget_city", policy.Budget("city"))

	// Both loops are waited for at shutdown, before the pool closes.
	var running sync.WaitGroup

	// One worker sends at a time (the rate limit lives in this process's pacer);
	// another one waits here and takes over when the sending worker stops.
	running.Add(1)
	go func() {
		defer running.Done()
		if err := w.RunExclusive(ctx, pool, 5*time.Second); err != nil && ctx.Err() == nil {
			log.Error("worker stopped", "error", err)
			stop()
		}
	}()

	// Starts the campaigns scheduled for now: builds their audience and queues
	// the deliveries. Several workers can run this at once; each campaign is
	// taken by exactly one of them.
	scheduler := campaign.NewScheduler(pool, policy, clock.Real{}, log)
	running.Add(1)
	go func() {
		defer running.Done()
		if err := scheduler.Run(ctx); err != nil && ctx.Err() == nil {
			log.Error("scheduler stopped", "error", err)
			stop()
		}
	}()

	mux := http.NewServeMux()
	// Includes the database: a worker that cannot reach it cannot send anything.
	mux.HandleFunc("GET /healthz", httpx.HealthzWith(pool.Ping))
	srv := &http.Server{
		Addr:              config.Env("WORKER_HTTP_ADDR", ":9091"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := httpx.Serve(ctx, srv, log); err != nil {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}

	// Shutting down: nothing new is claimed; messages already with the provider
	// get cfg.ShutdownGrace to finish and be recorded (Run enforces it). The pool
	// closes (deferred) only after that, or after a hard limit.
	stopped := make(chan struct{})
	go func() { running.Wait(); close(stopped) }()
	select {
	case <-stopped:
		log.Info("worker stopped cleanly")
	case <-time.After(cfg.ShutdownGrace + 2*time.Second):
		log.Warn("worker did not stop within the shutdown grace; closing anyway")
	}
}
