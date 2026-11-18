# Everything runs inside Docker: no Go or Node needed on the host.
# On Windows install make with `choco install make` (or use Git Bash / WSL).
# Recipes are single docker commands so they work under cmd, PowerShell and sh.

COMPOSE     := docker compose
COMPOSE_DEV := docker compose -f docker-compose.yml -f docker-compose.dev.yml

.PHONY: up down build dev dev-down ps logs test test-backend test-frontend test-race test-live bench clean

## Full stack, production-like images (http://localhost:8080)
up:
	$(COMPOSE) up --build

down:
	$(COMPOSE) down

build:
	$(COMPOSE) build

## Dev stack: air + Vite HMR, source synced into containers (no rebuilds)
dev:
	$(COMPOSE_DEV) up --build --watch

dev-down:
	$(COMPOSE_DEV) down

ps:
	$(COMPOSE) ps

logs:
	$(COMPOSE) logs -f --tail=100

## All tests. Always mock LLM: no API key and no internet needed,
## even if .env contains a real key.
test: test-backend test-frontend

test-backend:
	$(COMPOSE) --profile test run --rm --build backend-test

test-frontend:
	$(COMPOSE) --profile test run --rm --build frontend-test

## Backend tests under the Go race detector (Docker, CGO). Not part of `make test` (slower).
test-race:
	$(COMPOSE) --profile test run --rm --build backend-test-race

## Opt-in smoke test against real Claude. Requires ANTHROPIC_API_KEY in .env.
## Not part of `make test`.
test-live:
	$(COMPOSE) --profile live run --rm --build backend-test-live

## Send-rate benchmark: worker + mock provider, both channels, real clock (~30 s).
## Prints calls/s and accepted/s per channel; fails if any 1 s window exceeds 100 calls.
## Not part of `make test`. Options: BENCH_RATES=95,97,100 BENCH_MESSAGES=800
bench:
	$(COMPOSE) --profile bench run --rm --build -e BENCH_RATES=$(BENCH_RATES) -e BENCH_MESSAGES=$(BENCH_MESSAGES) backend-bench

## Stop everything and delete the database volume (re-seeds on next up)
clean:
	$(COMPOSE) down -v
