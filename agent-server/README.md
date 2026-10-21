# Home Agent

## Deployment

Docker build files and Compose configuration are kept in [`deployment/`](../deployment/).

### Native Raspberry Pi install (recommended for 4 GB)

Install the Go API as a systemd service with PostgreSQL and Ollama installed
directly on Debian. This avoids running Docker on the Pi. Run the build and copy
commands from the repository root on the Mac:

```sh
(cd agent-server && \
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" \
  -o /tmp/agent-server-linux-arm64 ./cmd)
(cd agent-server && \
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" \
  -o /tmp/agent-token-linux-arm64 ./cmd/token)
tar -czf /tmp/home-agent-source.tar.gz \
  --exclude=agent-server/main \
  --exclude='agent-server/scripts/__pycache__' \
  agent-server deployment

ssh pi-user@pi-address 'mkdir -p "$HOME/home-agent-deploy"'
scp /tmp/agent-server-linux-arm64 \
  /tmp/agent-token-linux-arm64 \
  /tmp/home-agent-source.tar.gz \
  deployment/install-native.sh deployment/agent-server.service \
  pi-user@pi-address:home-agent-deploy/
```

On the Pi, unpack the complete source and deployment configuration for
inspection/updates:

```sh
mkdir -p ~/home-agent-source
tar -xzf ~/home-agent-deploy/home-agent-source.tar.gz -C ~/home-agent-source
```

Then open a terminal on the Pi and run:

```sh
sudo bash ~/home-agent-deploy/install-native.sh \
  ~/home-agent-deploy/agent-server-linux-arm64 \
  ~/home-agent-deploy/agent-token-linux-arm64
```

The installer sets up PostgreSQL and the selected agent provider, creates the
application database, and enables the API at boot. It prompts privately for
the database password and Google Maps API key. Ollama downloads
`qwen3.5:0.8b` (about 1.2 GB) for local inference. Secrets are saved root-only
in `/etc/home-agent/agent.env`. The API listens on localhost port 8080. Check
it with `curl http://127.0.0.1:8080/healthz`; inspect logs with
`sudo journalctl -u home-agent -f`.

The installer requires a 64-bit (`aarch64`) Debian-based Pi with working
internet access and sudo. It does not remove existing Docker containers or
volumes. If the installer reports an error, check
`sudo systemctl status postgresql ollama home-agent`.

The installer asks which agent provider to use: `ollama` (default),
`chatgpt`, or `claude`. Ollama runs locally and needs no API credential.
OpenAI or Anthropic API keys are read without echo and stored in the root-only
`/etc/home-agent/agent.env` file (`0600`); do not place them in source, shell
history, or the phone browser. Existing deployments can edit that file with
`sudoedit /etc/home-agent/agent.env`, then restart `home-agent`.

For local access without exposing port 8080 on the LAN, use an SSH tunnel
(`ssh -L 8080:127.0.0.1:8080 pi-user@pi-address`) and open
`http://127.0.0.1:8080/`. Keep the API on a trusted network; do not expose it
directly to the public internet.

### Apple Silicon / desktop

This mode runs the Go API and Ollama together in one container, with PostgreSQL
in a separate container:

```sh
export POSTGRES_PASSWORD='choose-a-local-password'
export JWT_SECRET="$(openssl rand -hex 32)"
export AGENT_HOUSEHOLD_ID='my-household'
export GOOGLE_MAPS_API_KEY='your-google-maps-api-key'
docker compose -f deployment/compose.yaml up --build
```

The API is available at `http://localhost:8080`. The `ollama-models` and
`postgres-data` volumes persist models and database data across container
recreations. Set `MODEL_NAME` to select the Ollama model; it defaults to
`qwen3.5:0.8b` (about 1.2 GB in Ollama's catalog). The model is downloaded on
first startup and cached in the volume, so the first run needs internet access.
The 2048-token context default keeps memory usage modest.

For the desktop Compose deployment, set `OPENAI_API_KEY` in the environment to
use OpenAI (defaults to `gpt-4o-mini`); optionally set `OPENAI_MODEL` to choose
another OpenAI model. Without an OpenAI key, requests use the local Ollama
endpoint and `MODEL_NAME` (default `qwen3.5:0.8b`). The older variable name
`OPEN_API_KEY` is also accepted for compatibility.

### Agent providers

The agent implements one `Client.SendRequest` contract and selects a provider
with `AGENT_PROVIDER`:

| Provider | Configuration |
| --- | --- |
| `ollama` | `MODEL_SERVER_URL` and `MODEL_NAME`; the deployment starts Ollama |
| `chatgpt` | `OPENAI_API_KEY`, optional `OPENAI_MODEL` |
| `claude` | `ANTHROPIC_API_KEY`, optional `ANTHROPIC_MODEL` |
| `local` | `MODEL_SERVER_URL` and `MODEL_NAME` for an OpenAI-compatible local endpoint |

When `AGENT_PROVIDER` is omitted, an Anthropic key selects Claude, an OpenAI
key selects ChatGPT, and otherwise Ollama is selected. The notes handler uses a
repository interface; Postgres is the current implementation, so future notes
connectors can be added without changing the HTTP API.

### API authentication

All endpoints except `GET /healthz` require a bearer JWT signed with HS256.
Configure `JWT_SECRET` with at least 32 characters and `AGENT_API_AUDIENCE`
(defaults to `home-agent-api`) and set `AGENT_HOUSEHOLD_ID` to the household
this deployment serves. Tokens must include `aud` matching that audience, a
client identifier in `sub`, a `household_id` matching the configured household,
and an `exp` claim.
The API validates tokens; the root-only `agent-token` command issues short-lived
tokens locally on the Pi. Keep the signing secret out of client apps.

This release supports one household per API/database deployment. Run a
separate deployment and database for each household; the middleware rejects
tokens issued for a different household. Sharing one Postgres database between
households is not supported until tenant-keyed tables and repository queries
are added.

### Browser dashboard

Open `/` on the SSH-forwarded local API from a phone or desktop.
Paste a client token created on the Pi:

```sh
sudo /usr/local/bin/agent-token -config /etc/home-agent/agent.env \
  -client phone-browser -household home -ttl 24h
```

Copy only the printed token into the dashboard. Tokens are valid for 24 hours
by default (up to 30 days) and are kept in the current tab's `sessionStorage`;
clear the tab session to remove them. The dashboard supports notes, tasks,
shortcuts, and location-memory summaries. For place memories, **Use my current
location** asks the browser for permission and fills the coordinates in the
form; you must still submit the form to save them. Browser location access
requires HTTPS or localhost. Coordinates are sent to Home Agent only when you
submit the memory. The dashboard never receives the JWT signing secret or
model-provider API keys.

### Raspberry Pi all-in-one

Build and run one ARM64 image containing the Go API, Ollama, and PostgreSQL.
Commands are run from the repository root:

```sh
docker buildx build --platform linux/arm64 --load --target raspberry-pi \
  -f deployment/Dockerfile -t home-agent:raspberry-pi .
docker run -d --name home-agent \
  -p 8080:8080 \
  -e POSTGRES_PASSWORD='choose-a-local-password' \
  -e GOOGLE_MAPS_API_KEY='your-google-maps-api-key' \
  -e MODEL_NAME=qwen3.5:0.8b \
  -v home-agent-postgres:/var/lib/postgresql/data \
  -v home-agent-models:/models \
  home-agent:raspberry-pi
```

The ARM64 image also runs on Apple Silicon. For an M1, the desktop Compose
layout is usually preferable: it avoids running PostgreSQL inside the app
container, although Ollama inside Docker uses CPU inference rather than macOS
Metal. `MODEL_NAME` and `MODEL_SERVER_URL` can be overridden in either mode.

The default `qwen3.5:0.8b` model is about 1.2 GB in Ollama's catalog and is
selected as a small starting point for a 4 GB Raspberry Pi. PostgreSQL is
configured with modest memory limits, and Ollama runs one request at a time
with a 2048-token context. Ollama saves models under `/models`; preserve the
model volume between runs. A 4 GB device should be able to run this
configuration, but CPU inference will be slow and other running services reduce
available memory. Larger models require proportionally more memory, storage,
and time.

## REST API

All successful resource responses use `{"data": ...}`. Errors use
`{"error":{"code":"...","message":"..."}}`. All endpoints are under `/api/v1`;
unversioned event and agent-management routes have been removed.

An importable Postman collection is available at
[`deployment/postman/Home-Agent.postman_collection.json`](../deployment/postman/Home-Agent.postman_collection.json).
Import it into Postman, then set its `baseUrl` collection variable to the API
address (for example, `http://localhost:8080` or `http://<raspberry-pi-ip>:8080`).
Location planning requires `GOOGLE_MAPS_API_KEY` with Google Geocoding API,
Places API (New), and Routes API enabled for the associated Google Cloud
project. The location update calls the model to classify pending errands,
reverse-geocodes coordinates when location details are absent, searches nearby
for relevant task categories, and returns a structured response. Home chore
estimates come from a mock adapter; no device or Apple Shortcut is executed by
the API. Register shortcuts so the response can recommend their exact names.

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | Liveness check |
| `POST` | `/api/v1/notes` | Create a standalone note: `{"content":"..."}` |
| `GET` | `/api/v1/notes` | List notes, newest first |
| `DELETE` | `/api/v1/notes/{id}` | Delete a note |
| `POST` | `/api/v1/tasks` | Create a task: `{"title":"Buy milk","description":"2%"}` |
| `GET` | `/api/v1/tasks?status=pending` | List tasks; optional status filter |
| `POST` | `/api/v1/location-memories` | Add a place memory |
| `GET` | `/api/v1/location-memories?store_name=Cafe&store_type=restaurant` | List memories for a place |
| `GET` | `/api/v1/location-memories/summary?store_name=Market&store_type=grocery` | Summarize place memories; grocery summaries include pending tasks |
| `POST` | `/api/v1/shortcuts` | Register/update a Shortcut name and description |
| `GET` | `/api/v1/shortcuts` | List registered Shortcuts |
| `DELETE` | `/api/v1/shortcuts/{name}` | Unregister a Shortcut |
| `PUT` | `/api/v1/settings/home-location` | Set home coordinates and optional geofence radius |
| `GET` | `/api/v1/settings/home-location` | Read configured home coordinates |
| `POST` | `/api/v1/location-updates` | Plan errands/actions for coordinates and return Shortcut-compatible JSON |
| `GET` | `/api/v1/location-updates/history?limit=50` | List recent location updates (maximum 100 per request) |
| `DELETE` | `/api/v1/location-updates/history/{id}` | Delete one stored location history entry |

Location memory creation requires `store_type`, `store_name`, `message`,
`latitude`, and `longitude`. Location updates require `latitude` and
`longitude`; optional `location_name` and `location_type` skip reverse
geocoding when both are provided. The result includes `data.suggested_shortcuts`,
whose `shortcutName` values are restricted to registered Shortcut names.
Location history, including exact coordinates and recommendations, is stored
indefinitely until deleted. All endpoints except `GET /healthz` require a
bearer JWT. Keep the API on a trusted network and do not expose it directly to
the public internet.

Example:

```sh
curl -X POST http://localhost:8080/api/v1/notes \
  -H 'Content-Type: application/json' \
  -d '{"content":"Pick up the dry cleaning"}'

curl -X POST http://localhost:8080/api/v1/location-memories \
  -H 'Content-Type: application/json' \
  -d '{"store_type":"restaurant","store_name":"Sample Cafe","latitude":40.7580,"longitude":-73.9855,"message":"The croissants are excellent."}'

# Configure the home geofence; radius_meters defaults to 150.
curl -X PUT http://localhost:8080/api/v1/settings/home-location \
  -H 'Content-Type: application/json' \
  -d '{"latitude":40.7580,"longitude":-73.9855,"radius_meters":150}'

# Register the exact Apple Shortcut name and explain when it should run.
curl -X POST http://localhost:8080/api/v1/shortcuts \
  -H 'Content-Type: application/json' \
  -d '{"name":"Start Roomba","description":"Start the robot vacuum when I am home and have time for cleaning."}'

# Location response contains data.suggested_shortcuts[].shortcutName for Shortcuts to parse.
curl -X POST http://localhost:8080/api/v1/location-updates \
  -H 'Content-Type: application/json' \
  -d '{"latitude":40.7580,"longitude":-73.9855}'

curl 'http://localhost:8080/api/v1/location-updates/history?limit=50'
```

The location response is shaped for Shortcuts to consume:

```json
{
  "data": {
    "at_home": true,
    "pending_task_actions": [
      {
        "task_id": 7,
        "task_title": "Run Roomba",
        "action_type": "start_robot_vacuum",
        "estimated_minutes": 45,
        "source": "mock"
      }
    ],
    "suggested_shortcuts": [
      {
        "shortcutName": "Start Roomba",
        "reason": "You are home and have a pending cleaning task.",
        "input": {
          "task_id": 7
        }
      }
    ]
  }
}
```

Read `data.suggested_shortcuts` in the iOS Shortcut and use
`shortcutName` to select the locally installed action. Suggested actions are
recommendations only; the caller decides whether to run them.

## Seed scripts and tests

```sh
python3 scripts/seed_location_info.py \
  --store-name "Juniper & Crumb Cafe" \
  --store-type restaurant \
  --message "I usually order a cafe latte." \
  --message "The croissants are really good."

python3 scripts/seed_task.py --title "Buy milk" --description "2% milk"

go test ./...
python3 -m unittest discover -s scripts -p 'test_*.py' -v
```
