# AI Gateway

[![Build and deploy](https://github.com/leshchenko1979/ai-gateway/actions/workflows/deploy.yml/badge.svg)](https://github.com/leshchenko1979/ai-gateway/actions/workflows/deploy.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/leshchenko1979/ai-gateway?filename=app%2Fgo.mod&logo=go&label=go)](app/go.mod)

A lightweight, OpenAI-compatible API gateway written in Go that routes requests sequentially through configured providers until a successful response is received.

**Build requirements:** Go **1.25+** (see [`app/go.mod`](app/go.mod)).

## Why Choose AI Gateway?

**Born from Frustration**: Created when Cloudflare AI Gateway unexpectedly started disconnecting users without explanation. This self-hosted alternative gives you full control with no vendor lock-in.

**Daily Use Case**: Connects to multiple AI providers with free tiers, automatically cycling between them when rate limits are hit - ensuring continuous service.

### Key Benefits
- **Lightweight**: Small stripped binary (~13MB typical local build) with minimal memory footprint
- **Fast**: Compiled Go with efficient runtime, no JVM overhead
- **Reliable**: Sequential provider fallback, automatic retry logic
- **Simple**: Single binary deployment, YAML configuration
- **Secure**: API key redaction, non-root execution, restrictive permissions
- **Open-AI Compatible**: Drop-in replacement for OpenAI API in tools like n8n. Just change the API base and make sure the requested model name matches one of your configured routes.

## Installation

### Quick Setup

1. **Configure the gateway:**
```bash
cp config.yaml.example config.yaml
# Edit config.yaml with your API keys
# See Configuration below
```

2. **Deploy locally:**
```bash
./ops.sh build                    # Build binary
./ops.sh install-service         # Install as systemd service
sudo systemctl start ai-gateway      # Start service
```

3. **Or deploy remotely:**
```bash
cp .env.example .env                  # Configure SSH credentials
# Edit .env with your server details or put them into the command string
SSH_HOST=your-server.com ./ops.sh deploy
```

### Deployment Options

Commands below use [`ops.sh`](ops.sh) at the repo root (formerly `install.sh`).

- **Local**: `./ops.sh build` → `./ops.sh install-service` for development/production on same machine
- **Remote (systemd)**: `./ops.sh deploy` — SSH upload, remote installation, and systemd service setup
- **Remote (Docker, CLI)**: `./ops.sh deploy-docker` — same as `./scripts/sync-config-to-vds.sh` (upload `docker-compose.yml`, `config.yaml`, filtered `.env`; `docker compose pull && up -d` with GHCR image and bind-mounted config)
- **Binary-only**: `./ops.sh install` — copy binary only, no systemd service

For systemd deployments, use a reverse proxy like `nginx` or `traefik` to set up TLS termination and secure the traffic to your gateway.

### Docker Installation

Deploy as a container behind Traefik (or any reverse proxy) for HTTPS termination:

1. **Prerequisites**: Docker on the remote server; Traefik with `traefik-public` network
2. **Configure**: Ensure `config.yaml` and `.env` exist with your API keys (GATEWAY_API_KEY, provider keys)
3. **Deploy**:
   ```bash
   cp .env.example .env   # if needed
   # Edit .env with SSH_HOST, SSH_USER, DOMAIN, and runtime vars (GATEWAY_API_KEY, etc.)
   ./ops.sh deploy-docker
   ```
4. **Domain**: Set `DOMAIN` in `.env` (e.g. `DOMAIN=ai-gateway.example.com`). docker-compose uses it for the Traefik Host rule.
5. **n8n integration**: Set Base URL to `https://YOUR_DOMAIN/v1` (your Traefik host), Model to a route name from `config.yaml`, API Key to your `GATEWAY_API_KEY` value.

### CI/CD (GitHub Actions + GHCR)

Pushes to `main` run **`go test ./... -count=1`** in [`app/`](app/) first; if tests pass, the workflow builds the image on GitHub, pushes images to GHCR (`:main` and `:sha-<short>`), then SSHs into the VDS and runs `docker compose pull && docker compose up -d` in `/root/services/ai-gateway`.

**Repository secrets (CI / deploy job)**

| Secret | Purpose |
|--------|---------|
| `SSH_HOST` | VDS hostname or IP |
| `SSH_USER` | SSH user |
| `SSH_PRIVATE_KEY` | Private key (PEM) for that user |
| `SSH_PORT` | Optional; defaults to `22` if omitted |
| `GHCR_PULL_USER` | Optional; GitHub username for `docker login` on the VDS when the package is **private** |
| `GHCR_PULL_TOKEN` | Optional; PAT with `read:packages` for that login |

**Runtime on the VDS** (not stored in GitHub): a gitignored `config.yaml` and `.env` beside `docker-compose.yml`. The compose file pulls `ghcr.io/leshchenko1979/ai-gateway` tagged by `IMAGE_TAG` (default `main`). Set `GHCR_IMAGE` and `IMAGE_TAG` in the VDS `.env` if you use a fork or pin a digest tag.

**Syncing config from your machine** without committing secrets: copy `config.yaml` and `.env` from the examples, fill them in, then run `./scripts/sync-config-to-vds.sh`. That uploads `docker-compose.yml`, `config.yaml`, and a filtered `.env` (SSH and `GHCR_PULL_*` lines are stripped so they are not left on the server), optionally runs `docker login` on the VDS when `GHCR_PULL_USER` and `GHCR_PULL_TOKEN` are set locally, then `docker compose pull && up -d`. In Cursor/VS Code, use **Tasks → Run Build Task** and choose **Sync config and env to VDS** (non-default build task).

**Local Docker build** instead of pulling GHCR: add a gitignored `docker-compose.override.yml` that sets `build: { context: ., dockerfile: Dockerfile }` on `ai-gateway` and drop `image:` for local use.

**CLI alternative to the VS Code sync task**: `./ops.sh deploy-docker` runs the same steps as `./scripts/sync-config-to-vds.sh`, then waits for the container. Use either the script or `ops.sh`; both match the current [`docker-compose.yml`](docker-compose.yml) (pulled image + `./config.yaml` mount).

## Configuration

The gateway uses YAML configuration with environment variable substitution:

```yaml
api_key: ${GATEWAY_API_KEY}  # Gateway authentication key
port: 8080                   # Optional, defaults to 8080
default_timeout: 300s        # Example; omit for built-in default 30s

providers:
  - name: cerebras
    api_key: ${CEREBRAS_API_KEY}
    base_url: https://api.cerebras.ai/v1
  - name: openrouter
    api_key: ${OPENROUTER_API_KEY}
    base_url: https://openrouter.ai/api/v1

routes:
  - name: dynamic/n8n  # Exact model name match required
    steps:
      - provider: cerebras
        model: gpt-oss-120b
        conflict_resolution: tools  # Remove response_format if tools present
      - provider: openrouter
        model: nvidia/nemotron-3-nano-30b-a3b:free
```

You can put your API keys into `config.yaml` directly, but for security purposes it's better to store them in env vars and use them in `config.yaml`.

**Configuration Locations:**
1. `./config.yaml` (current directory)
2. `/etc/ai-gateway/config.yaml` (system location)

**Environment Variables:**
- `GATEWAY_API_KEY`: Required for authentication
- Provider API keys: `${PROVIDER_NAME}_API_KEY`
 - Missing `${VAR}` values cause startup errors with a clear list of missing vars


## API Endpoints

### Authentication
These endpoints do **not** require authentication:

- `GET /health`
- `GET /v1/diagnostics/upstream-models` — see [Diagnostics](#diagnostics)

For all other routes (for example `GET /v1/models` and `POST /v1/chat/completions`), use `X-Api-Key` or `Authorization: Bearer <token>` with your configured gateway API key.

### Health Check
```bash
GET /health
```
Returns `{"status": "healthy"}` - no authentication required.

### Diagnostics
```bash
GET /v1/diagnostics/upstream-models
```
No authentication. Calls each configured provider’s OpenAI-style `GET {base_url}/models` in parallel and returns JSON with an overall `ok` flag and per-provider results. Responds with **503** if any provider check fails, **200** if all succeed.

**Security:** This route is unauthenticated and triggers outbound requests to provider APIs. Do not expose it on the public internet without network restrictions (for example reverse-proxy allowlists, VPN, or private ingress).

### List Models
```bash
GET /v1/models
Headers: X-Api-Key: <gateway-api-key> OR Authorization: Bearer <token>
```
Returns available route names from the configuration, which serve as the model names for requests.

### Chat Completions
```bash
POST /v1/chat/completions
Headers: X-Api-Key: <gateway-api-key> OR Authorization: Bearer <token>
```
Routes requests to providers. Set model to the desired route name.

**Response with routing summary:**
```json
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "model": "dynamic/n8n",
  "choices": [...],
  "routing_summary": {
    "route_name": "dynamic/n8n",
    "steps": [
      {"step_index": 0, "provider": "cerebras", "model": "gpt-oss-120b", "success": true, "duration_ms": 1234},
      {"step_index": 1, "provider": "openrouter", "model": "nvidia/nemotron-3-nano-30b-a3b:free", "success": false, "duration_ms": 500, "error": "rate limit exceeded"}
    ]
  }
}
```

The `routing_summary` field shows which route steps were attempted, which succeeded, and the timing in milliseconds for each step. Failed steps include the error message. Included in both successful responses and error responses when all steps fail.

## Service Management
```bash
sudo systemctl start ai-gateway     # Start service
sudo systemctl stop ai-gateway      # Stop service
sudo systemctl enable ai-gateway    # Enable auto-start
sudo systemctl status ai-gateway    # Check status
sudo journalctl -u ai-gateway -f    # View logs
```

## Security & Logging

- **Security**: API key redaction, non-root execution, restrictive file permissions (600), TLS recommended
- **Logging**: Structured JSON logs with request/response summaries, automatic key redaction
- **Error Handling**: Sequential provider fallback on any error, detailed error messages with provider info

## Telemetry

The gateway can send OpenTelemetry traces and logger events directly to any OTLP/HTTP-compliant collector. Configure the following environment variables to point at your observability backend (Grafana Cloud, Alloy, Tempo, or other OTLP destination):

- `OTLP_ENDPOINT`: Full URL to the OTLP HTTP endpoint. Supports both `host:port` and full URLs like `https://otlp-gateway.example.com/otlp`.
- `OTLP_API_KEY`: API key or token.
    - For **Grafana Cloud**: You can use a standard `glc_` Access Policy Token. The gateway automatically extracts the Instance ID from the token and handles the required Basic authentication (`instanceID:apiKey`).
    - For other collectors: It uses the provided key for Basic authentication (`apiKey:`).
- `OTEL_SERVICE_NAME` (or `OTLP_SERVICE_NAME`): Optional. The service name (`ai-gateway`) used to group spans/logs.
- `OTEL_RESOURCE_ATTRIBUTES` (or `OTLP_RESOURCE_ATTRIBUTES`): Optional. Comma-separated `key=value` pairs added to each resource (e.g., `deployment.environment=production`).
- `OTLP_HEADERS` (optional): Optional. Extra headers in `Key=Value` CSV format.

### How it works
The gateway uses the **OTLP HTTP exporter** for maximum compatibility (bypassing gRPC/ALPN issues). It automatically handles the `/v1/traces` signal path, ensuring that if you provide a base URL (like Grafana's `/otlp`), it still reaches the correct endpoint.

## Contributing

Bug reports and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, tests, and PR expectations. For **security-sensitive** issues, use the process in [SECURITY.md](SECURITY.md) instead of a public issue.

## Reporting vulnerabilities

See [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)