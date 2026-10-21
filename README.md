# Home Agent

Home Agent is a Go API and browser dashboard for household notes, tasks,
location memories, and location-triggered recommendations. It uses PostgreSQL
for persistence and can use Ollama locally or an OpenAI/Anthropic-compatible
provider for model responses.

## Requirements

- Go **1.26.1 or newer** for local builds and tests.
- Docker Engine/Desktop with the Docker Compose v2 plugin for the container
  setup.
- Python **3.10 or newer** for the standard-library seed scripts and tests.
- A Google Maps API key with Geocoding API, Places API (New), and Routes API
  enabled.
- Git and [pre-commit](https://pre-commit.com/) to install the repository's
  commit checks. Gitleaks is installed automatically by pre-commit.

The first container startup downloads Ollama and the configured model, so
allow several gigabytes of free disk space and an internet connection. For a
Raspberry Pi, use a 64-bit Debian-based OS and an ARM64-capable Docker build
environment, or follow the native-install instructions in
[`agent-server/README.md`](agent-server/README.md).

## Configure and run

From the repository root:

```sh
cp .env.example .env
```

Edit `.env` and set a strong, unique `POSTGRES_PASSWORD`, a randomly generated
`JWT_SECRET` of at least 32 characters, your `AGENT_HOUSEHOLD_ID`, and your
`GOOGLE_MAPS_API_KEY`. Keep `.env` private; it is ignored by Git. Generate a
JWT secret with:

```sh
openssl rand -hex 32
```

Start the API, PostgreSQL, and local Ollama provider:

```sh
make run
```

The API and dashboard are available at `http://localhost:8080`. Check that the
server is healthy with `curl http://localhost:8080/healthz`. Stop the
containers with `make stop`.

By default, the app uses Ollama and downloads `qwen3.5:0.8b` on first startup.
To use a hosted provider, set `AGENT_PROVIDER=chatgpt` with `OPENAI_API_KEY`,
or `AGENT_PROVIDER=claude` with `ANTHROPIC_API_KEY`, in `.env`. For a local
OpenAI-compatible endpoint, set `AGENT_PROVIDER=local` and `MODEL_SERVER_URL`.

Set the home location in the authenticated API before using location-aware
routes. If you need the route-calculator fallback before a home location is
saved, configure `HOME_LATITUDE` and `HOME_LONGITUDE` in `.env`; do not commit
real home coordinates.

The API requires a bearer JWT for all endpoints except `/healthz`. See the
application guide for token creation, endpoints, native installation, and
additional configuration:
[`agent-server/README.md`](agent-server/README.md).

## Build and test

Run all Go and Python tests, formatting checks, and Go static analysis:

```sh
make check
```

Build the desktop or Raspberry Pi container images with:

```sh
make build-agent-server
make build-raspberry-pi
```

Run the Python seed-script tests independently with:

```sh
python3 -m unittest discover -s agent-server/scripts -p 'test_*.py' -v
```

## Commit-time checks

Install the Git hooks after cloning:

```sh
python3 -m pip install pre-commit
pre-commit install
```

Each commit checks staged files for secrets and private keys, rejects
unexpectedly large files and common formatting/merge errors, and runs
`make check`. Run every hook manually with:

```sh
pre-commit run --all-files
```

The local `locationdata.json` file and generated binaries/bytecode are ignored
to avoid publishing personal location data or build artifacts.
