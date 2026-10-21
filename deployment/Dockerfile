FROM golang:1.26 AS builder

WORKDIR /src
COPY agent-server/go.mod agent-server/go.sum ./
RUN go mod download
COPY agent-server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /agent-server ./cmd

FROM ubuntu:24.04 AS ollama-runtime

ENV DEBIAN_FRONTEND=noninteractive \
    MODEL_SERVER_URL=http://127.0.0.1:11434/v1 \
    OLLAMA_HOST=127.0.0.1:11434 \
    OLLAMA_MODELS=/models \
    OLLAMA_CONTEXT_LENGTH=2048 \
    OLLAMA_NUM_PARALLEL=1 \
    OLLAMA_MAX_LOADED_MODELS=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl zstd \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fsSL https://ollama.com/install.sh | sh

COPY --from=builder /agent-server /usr/local/bin/agent-server
COPY deployment/entrypoint.sh /usr/local/bin/home-agent-entrypoint
RUN chmod 0755 /usr/local/bin/home-agent-entrypoint

FROM ollama-runtime AS desktop
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=30m --retries=3 \
    CMD curl -fsS --max-time 4 http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/home-agent-entrypoint"]

FROM ollama-runtime AS raspberry-pi
ENV EMBEDDED_POSTGRES=true \
    PGDATA=/var/lib/postgresql/data \
    POSTGRES_HOST=127.0.0.1 \
    POSTGRES_PORT=5432 \
    POSTGRES_USER=agent \
    POSTGRES_DB=agent
RUN apt-get update \
    && apt-get install -y --no-install-recommends postgresql \
    && rm -rf /var/lib/apt/lists/*
EXPOSE 8080
VOLUME ["/var/lib/postgresql/data", "/models"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=30m --retries=3 \
    CMD curl -fsS --max-time 4 http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/home-agent-entrypoint"]
