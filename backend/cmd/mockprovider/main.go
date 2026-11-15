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
	log := httpx.NewLogger("mock-provider")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Phase 0: health only. SMS/email endpoints with chaos are added in phase 1.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Healthz)

	srv := &http.Server{
		Addr:              config.Env("HTTP_ADDR", ":8090"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := httpx.Serve(ctx, srv, log); err != nil {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}
