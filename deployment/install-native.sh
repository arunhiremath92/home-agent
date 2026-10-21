#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "$EUID" -ne 0 ]]; then
    echo "Run this installer with sudo: sudo bash install-native.sh /path/to/agent-server-linux-arm64" >&2
    exit 1
fi

binary_path="${1:-}"
token_binary_path="${2:-}"
if [[ -z "$binary_path" || ! -f "$binary_path" || -z "$token_binary_path" || ! -f "$token_binary_path" ]]; then
    echo "Usage: sudo bash install-native.sh /path/to/agent-server-linux-arm64 /path/to/agent-token-linux-arm64" >&2
    exit 2
fi

if [[ "$(uname -m)" != "aarch64" ]]; then
    echo "This installer targets 64-bit ARM Linux (aarch64)." >&2
    exit 2
fi

prompt_secret() {
    local variable_name="$1"
    local prompt="$2"
    local value="${!variable_name:-}"
    if [[ -z "$value" ]]; then
        read -r -s -p "$prompt: " value
        printf '\n'
    fi
    if [[ -z "$value" ]]; then
        echo "$variable_name must not be empty" >&2
        exit 2
    fi
    printf -v "$variable_name" '%s' "$value"
}

validate_env_value() {
    local variable_name="$1"
    local value="${!variable_name}"
    if [[ ! "$value" =~ ^[A-Za-z0-9._~!@#%^+=:,/-]+$ ]]; then
        echo "$variable_name may contain only letters, numbers, and . _ ~ ! @ # % ^ + = : , / -" >&2
        exit 2
    fi
}

POSTGRES_USER="${POSTGRES_USER:-agent}"
POSTGRES_DB="${POSTGRES_DB:-agent}"
MODEL_NAME="${MODEL_NAME:-qwen3.5:0.8b}"
JWT_SECRET="${JWT_SECRET:-$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')}"
AGENT_API_AUDIENCE="${AGENT_API_AUDIENCE:-home-agent-api}"
AGENT_HOUSEHOLD_ID="${AGENT_HOUSEHOLD_ID:-home}"
AGENT_PROVIDER="${AGENT_PROVIDER:-}"
prompt_secret POSTGRES_PASSWORD "PostgreSQL password"
prompt_secret GOOGLE_MAPS_API_KEY "Google Maps API key"
if [[ -z "$AGENT_PROVIDER" ]]; then
    read -r -p "Agent provider [ollama/chatgpt/claude] (default ollama): " AGENT_PROVIDER
    AGENT_PROVIDER="${AGENT_PROVIDER:-ollama}"
fi
OPENAI_API_KEY="${OPENAI_API_KEY:-${OPEN_API_KEY:-}}"
ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:-}"
case "$AGENT_PROVIDER" in
    ollama) ;;
    chatgpt) prompt_secret OPENAI_API_KEY "OpenAI API key" ;;
    claude) prompt_secret ANTHROPIC_API_KEY "Anthropic API key" ;;
    *) echo "AGENT_PROVIDER must be ollama, chatgpt, or claude" >&2; exit 2 ;;
esac
validate_env_value POSTGRES_USER
validate_env_value POSTGRES_DB
validate_env_value POSTGRES_PASSWORD
validate_env_value JWT_SECRET
validate_env_value AGENT_API_AUDIENCE
validate_env_value AGENT_HOUSEHOLD_ID
validate_env_value GOOGLE_MAPS_API_KEY
if [[ -n "$OPENAI_API_KEY" ]]; then
    validate_env_value OPENAI_API_KEY
fi
if [[ -n "$ANTHROPIC_API_KEY" ]]; then
    validate_env_value ANTHROPIC_API_KEY
fi
validate_env_value MODEL_NAME

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends ca-certificates curl openssl postgresql

mapfile -t postgres_versions < <(pg_lsclusters --no-header | awk '$2 == "main" { print $1 }')
if [[ "${#postgres_versions[@]}" -eq 0 ]]; then
    echo "No default PostgreSQL cluster was created" >&2
    exit 1
fi
for postgres_version in "${postgres_versions[@]}"; do
    postgres_config="/etc/postgresql/$postgres_version/main/conf.d"
    install -d -o postgres -g postgres -m 0755 "$postgres_config"
    cat > "$postgres_config/home-agent.conf" <<'EOF'
max_connections = 20
shared_buffers = 128MB
work_mem = 4MB
EOF
done

if [[ "$AGENT_PROVIDER" == "ollama" ]]; then
    if ! command -v ollama >/dev/null 2>&1; then
        curl -fsSL https://ollama.com/install.sh | sh
    fi
    if ! id ollama >/dev/null 2>&1; then
        echo "Ollama installer did not create the ollama service user" >&2
        exit 1
    fi
    install -d -o ollama -g ollama -m 0750 /var/lib/ollama/models
    install -d -o root -g root -m 0755 /etc/systemd/system/ollama.service.d
    cat > /etc/systemd/system/ollama.service.d/home-agent.conf <<'EOF'
[Service]
Environment="OLLAMA_HOST=127.0.0.1:11434"
Environment="OLLAMA_MODELS=/var/lib/ollama/models"
Environment="OLLAMA_CONTEXT_LENGTH=2048"
Environment="OLLAMA_NUM_PARALLEL=1"
Environment="OLLAMA_MAX_LOADED_MODELS=1"
EOF
    systemctl daemon-reload
fi
systemctl enable --now postgresql.service
if [[ "$AGENT_PROVIDER" == "ollama" ]]; then
    systemctl enable --now ollama.service
fi

runuser -u postgres -- env \
    POSTGRES_USER="$POSTGRES_USER" \
    POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
    POSTGRES_DB="$POSTGRES_DB" \
    psql --no-psqlrc --set=ON_ERROR_STOP=1 --dbname=postgres <<'SQL'
\getenv app_user POSTGRES_USER
\getenv app_password POSTGRES_PASSWORD
\getenv app_db POSTGRES_DB
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'app_user', :'app_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'app_user') \gexec
SELECT format('ALTER ROLE %I WITH LOGIN PASSWORD %L', :'app_user', :'app_password') \gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'app_db', :'app_user')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'app_db') \gexec
SQL

if [[ "$AGENT_PROVIDER" == "ollama" ]]; then
    printf 'Waiting for the Ollama API'
    for _ in $(seq 1 60); do
        if curl -fsS --max-time 3 http://127.0.0.1:11434/api/tags >/dev/null; then
            printf '\n'
            break
        fi
        printf '.'
        sleep 2
    done
    if ! curl -fsS --max-time 3 http://127.0.0.1:11434/api/tags >/dev/null; then
        printf '\nOllama did not become ready; check: systemctl status ollama\n' >&2
        exit 1
    fi
    if ! ollama show "$MODEL_NAME" >/dev/null 2>&1; then
        ollama pull "$MODEL_NAME"
    fi
fi

if ! id home-agent >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/home-agent --shell /usr/sbin/nologin home-agent
fi
install -d -o home-agent -g home-agent -m 0750 /var/lib/home-agent
install -o root -g root -m 0755 "$binary_path" /usr/local/bin/agent-server
install -o root -g root -m 0755 "$token_binary_path" /usr/local/bin/agent-token

install -d -o root -g root -m 0750 /etc/home-agent
{
    printf 'POSTGRES_USER=%s\n' "$POSTGRES_USER"
    printf 'POSTGRES_PASSWORD=%s\n' "$POSTGRES_PASSWORD"
    printf 'POSTGRES_HOST=127.0.0.1\n'
    printf 'POSTGRES_PORT=5432\n'
    printf 'POSTGRES_DB=%s\n' "$POSTGRES_DB"
    printf 'JWT_SECRET=%s\n' "$JWT_SECRET"
    printf 'AGENT_API_AUDIENCE=%s\n' "$AGENT_API_AUDIENCE"
    printf 'AGENT_HOUSEHOLD_ID=%s\n' "$AGENT_HOUSEHOLD_ID"
    printf 'AGENT_PROVIDER=%s\n' "$AGENT_PROVIDER"
    printf 'AGENT_SERVER_ADDRESS=127.0.0.1:8080\n'
    printf 'GOOGLE_MAPS_API_KEY=%s\n' "$GOOGLE_MAPS_API_KEY"
    printf 'OPENAI_API_KEY=%s\n' "$OPENAI_API_KEY"
    printf 'ANTHROPIC_API_KEY=%s\n' "$ANTHROPIC_API_KEY"
    printf 'MODEL_NAME=%s\n' "$MODEL_NAME"
    printf 'MODEL_SERVER_URL=http://127.0.0.1:11434/v1\n'
} > /etc/home-agent/agent.env
chmod 0600 /etc/home-agent/agent.env
chown root:root /etc/home-agent/agent.env

install -o root -g root -m 0644 "$(dirname "$0")/agent-server.service" \
    /etc/systemd/system/home-agent.service
systemctl daemon-reload
systemctl enable home-agent.service
systemctl restart home-agent.service

echo "Home Agent installed. Check status with: systemctl status home-agent"
echo "API health check: curl http://127.0.0.1:8080/healthz"
