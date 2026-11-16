# syntax=docker/dockerfile:1

# ---- base: toolchain + module cache -----------------------------------------
FROM golang:1.25-alpine AS base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# ---- dev: used by docker-compose.dev.yml (air) and by `make test` -----------
FROM base AS dev
RUN go install github.com/air-verse/air@v1.61.7
COPY . .
CMD ["go", "test", "./..."]

# ---- build: compiles every binary under cmd/ --------------------------------
FROM base AS build
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# ---- runtime images (alpine: healthchecks use busybox wget) -----------------
FROM alpine:3.21 AS runtime
RUN adduser -D -u 10001 app
USER app

FROM runtime AS api
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8080
ENTRYPOINT ["api"]

FROM runtime AS worker
COPY --from=build /out/worker /usr/local/bin/worker
EXPOSE 9091
ENTRYPOINT ["worker"]

FROM runtime AS mockprovider
COPY --from=build /out/mockprovider /usr/local/bin/mockprovider
EXPOSE 8090
ENTRYPOINT ["mockprovider"]

FROM runtime AS tool
COPY --from=build /out/tool /usr/local/bin/tool
ENTRYPOINT ["tool"]

# ---- race: `go test -race` needs cgo and a C toolchain, which the Alpine -----
# images above do not have; this one is Debian-based (used by `make test-race`).
FROM golang:1.25 AS race
WORKDIR /src
ENV CGO_ENABLED=1
COPY go.mod go.sum ./
RUN go mod download
COPY . .
CMD ["go", "test", "-race", "./..."]
