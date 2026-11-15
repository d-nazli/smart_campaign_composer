package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smartcampaign/internal/clock"
	"smartcampaign/internal/config"
	"smartcampaign/internal/httpx"
	"smartcampaign/internal/provider"
)

func main() {
	log := httpx.NewLogger("mock-provider")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := provider.LoadConfig(os.Getenv)
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	log.Info("provider configuration",
		"latency_ms", cfg.Latency.Milliseconds(), "latency_jitter_ms", cfg.LatencyJitter.Milliseconds(),
		"error_rate", cfg.ErrorRate, "rate_429", cfg.Rate429, "timeout_rate", cfg.TimeoutRate,
		"timeout_ms", cfg.TimeoutDelay.Milliseconds(), "fail_after_accept_rate", cfg.FailAfterAccept,
		"retry_after_sec", [2]int{cfg.RetryAfterMin, cfg.RetryAfterMax},
		"rate_limit_per_sec", cfg.RateLimitPerSecond, "seed", cfg.Seed)

	p := provider.New(cfg, clock.Real{}, log)
	srv := &http.Server{
		Addr:              config.Env("HTTP_ADDR", ":8090"),
		Handler:           p.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := httpx.Serve(ctx, srv, log); err != nil {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}
