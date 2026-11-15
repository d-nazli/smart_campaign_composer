package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smartcampaign/internal/config"
	"smartcampaign/internal/httpx"
)

func main() {
	log := httpx.NewLogger("worker")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Phase 0: the worker only exposes its health endpoint.
	// The queue loop is added in phase 4.
	log.Info("worker started (queue loop not implemented yet)", "provider_url", config.Env("PROVIDER_URL", "http://localhost:8090"))

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
