#!/usr/bin/env bash
set -Eeuo pipefail

: "${POSTGRES_PASSWORD:?Set POSTGRES_PASSWORD}"
: "${GOOGLE_MAPS_API_KEY:?Set GOOGLE_MAPS_API_KEY}"
: "${JWT_SECRET:?Set JWT_SECRET to a random secret of at least 32 characters}"
: "${AGENT_API_AUDIENCE:=home-agent-api}"
: "${AGENT_HOUSEHOLD_ID:?Set AGENT_HOUSEHOLD_ID for this household deployment}"
export AGENT_API_AUDIENCE


EMBEDDED_POSTGRES="${EMBEDDED_POSTGRES:-false}"
POSTGRES_BIN=/usr/lib/postgresql/16/bin
OLLAMA_PID=
AGENT_PID=

shutdown() {
    trap - EXIT TERM INT

    if [[ -n "$AGENT_PID" ]]; then
        kill "$AGENT_PID" 2>/dev/null || true
        wait "$AGENT_PID" 2>/dev/null || true
    fi
    if [[ -n "$OLLAMA_PID" ]]; then
        kill "$OLLAMA_PID" 2>/dev/null || true
        wait "$OLLAMA_PID" 2>/dev/null || true
    fi
    if [[ "$EMBEDDED_POSTGRES" == "true" ]]; then
        if ! runuser -u postgres -- "$POSTGRES_BIN/pg_ctl" -D "$PGDATA" -m fast -w stop; then
            echo "PostgreSQL did not shut down cleanly" >&2
        fi
    fi
}
trap shutdown EXIT
trap 'exit 0' TERM INT

if [[ "$EMBEDDED_POSTGRES" == "true" ]]; then
    if [[ ! -s "$PGDATA/PG_VERSION" ]]; then
        install -d -o postgres -g postgres "$PGDATA"
        runuser -u postgres -- "$POSTGRES_BIN/initdb" -D "$PGDATA" \
            --auth-local=peer --auth-host=scram-sha-256 \
            --locale=C.utf8 --encoding=UTF8 --no-instructions
    fi
    runuser -u postgres -- "$POSTGRES_BIN/pg_ctl" -D "$PGDATA" \
        -l "$PGDATA/server.log" \
        -o "-h 127.0.0.1 -p $POSTGRES_PORT -c shared_buffers=64MB -c work_mem=2MB -c max_connections=20" \
        -w start

    runuser -u postgres -- psql --no-psqlrc --set=ON_ERROR_STOP=1 \
        --port="$POSTGRES_PORT" \
        --set=app_user="$POSTGRES_USER" \
        --set=app_password="$POSTGRES_PASSWORD" \
        --set=app_db="$POSTGRES_DB" \
        --dbname=postgres <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'app_user', :'app_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'app_user') \gexec
SELECT format('ALTER ROLE %I WITH LOGIN PASSWORD %L', :'app_user', :'app_password') \gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'app_db', :'app_user')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'app_db') \gexec
SQL
fi

if [[ -z "${AGENT_PROVIDER:-}" ]]; then
    if [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
        AGENT_PROVIDER=claude
    elif [[ -n "${OPENAI_API_KEY:-${OPEN_API_KEY:-}}" ]]; then
        AGENT_PROVIDER=chatgpt
    else
        AGENT_PROVIDER=ollama
    fi
fi
export AGENT_PROVIDER

if [[ "$AGENT_PROVIDER" == "ollama" ]]; then
    MODEL_NAME="${MODEL_NAME:-qwen3.5:0.8b}"
    export MODEL_NAME
    ollama serve &
    OLLAMA_PID=$!
    until curl -fsS --max-time 3 http://127.0.0.1:11434/api/tags >/dev/null; do
        if ! kill -0 "$OLLAMA_PID" 2>/dev/null; then
            echo "Ollama exited before its API became ready" >&2
            exit 1
        fi
        sleep 2
    done

    if ! ollama show "$MODEL_NAME" >/dev/null 2>&1; then
        ollama pull "$MODEL_NAME"
    fi
elif [[ "$AGENT_PROVIDER" == "chatgpt" ]]; then
    : "${OPENAI_API_KEY:=${OPEN_API_KEY:-}}"
    : "${OPENAI_API_KEY:?Set OPENAI_API_KEY for the ChatGPT provider}"
elif [[ "$AGENT_PROVIDER" == "claude" ]]; then
    : "${ANTHROPIC_API_KEY:?Set ANTHROPIC_API_KEY for the Claude provider}"
elif [[ "$AGENT_PROVIDER" == "local" ]]; then
    : "${MODEL_SERVER_URL:?Set MODEL_SERVER_URL for the local provider}"
else
    echo "Unsupported AGENT_PROVIDER: $AGENT_PROVIDER" >&2
    exit 1
fi

/usr/local/bin/agent-server &
AGENT_PID=$!
set +e
if [[ -n "$OLLAMA_PID" ]]; then
    wait -n "$AGENT_PID" "$OLLAMA_PID"
else
    wait "$AGENT_PID"
fi
process_status=$?
set -e
exit "$process_status"
