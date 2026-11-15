package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"smartcampaign/internal/config"
	"smartcampaign/internal/httpx"
)

func main() {
	log := httpx.NewLogger("api")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	llm, err := config.LoadLLM()
	if err != nil {
		log.Error("invalid LLM configuration", "error", err)
		os.Exit(1)
	}
	// The key itself is never logged.
	log.Info("LLM mode active", "mode", llm.Mode, "model", llm.Model, "reason", llm.Reason)

	pool, err := pgxpool.New(ctx, config.Env("DATABASE_URL", "postgres://campaign:campaign@localhost:5432/campaign"))
	if err != nil {
		log.Error("invalid DATABASE_URL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Healthz)
	mux.HandleFunc("GET /readyz", httpx.Readyz(pool.Ping))
	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, _ *http.Request) {
		info := map[string]string{"llm_mode": llm.Mode}
		if llm.Mode == config.LLMModeClaude {
			info["model"] = llm.Model
		}
		httpx.WriteJSON(w, http.StatusOK, info)
	})

	srv := &http.Server{
		Addr:              config.Env("HTTP_ADDR", ":8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		// No WriteTimeout: SSE responses are long-lived.
	}
	if err := httpx.Serve(ctx, srv, log); err != nil {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}
