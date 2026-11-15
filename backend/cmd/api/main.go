package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/campaign"
	"smartcampaign/internal/config"
	"smartcampaign/internal/content"
	"smartcampaign/internal/httpapi"
	"smartcampaign/internal/httpx"
	"smartcampaign/internal/llm"
	"smartcampaign/internal/llm/anthropic"
	"smartcampaign/internal/llm/mock"
)

func main() {
	log := httpx.NewLogger("api")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadLLM()
	if err != nil {
		log.Error("invalid LLM configuration", "error", err)
		os.Exit(1)
	}
	policy, err := content.PolicyFromEnv(os.Getenv)
	if err != nil {
		log.Error("invalid message variable budgets", "error", err)
		os.Exit(1)
	}

	var client llm.Client
	switch cfg.Mode {
	case config.LLMModeClaude:
		client = anthropic.New(cfg.APIKey, cfg.Model)
	default:
		delay, err := strconv.Atoi(config.Env("MOCK_LLM_DELAY_MS", "15"))
		if err != nil || delay < 0 {
			log.Error("MOCK_LLM_DELAY_MS must be a non-negative integer")
			os.Exit(1)
		}
		client = mock.New(time.Duration(delay) * time.Millisecond)
	}
	// The key itself is never logged.
	log.Info("LLM mode active", "mode", client.Mode(), "model", cfg.Model, "reason", cfg.Reason)

	pool, err := pgxpool.New(ctx, config.Env("DATABASE_URL", "postgres://campaign:campaign@localhost:5432/campaign"))
	if err != nil {
		log.Error("invalid DATABASE_URL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	svc := campaign.NewService(pool, client, policy, log)
	handler := httpapi.New(svc, httpapi.Info{LLMMode: cfg.Mode, Model: cfg.Model}, pool.Ping, log)

	srv := &http.Server{
		Addr:              config.Env("HTTP_ADDR", ":8080"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		// No WriteTimeout: SSE responses are long-lived.
	}
	if err := httpx.Serve(ctx, srv, log); err != nil {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}
