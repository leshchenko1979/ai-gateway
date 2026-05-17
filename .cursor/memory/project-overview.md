# AI Gateway - Route-Based Provider Configuration

## Project Description
A lightweight, OpenAI-compatible API gateway written in Go that routes requests to AI providers based on exact model name matching. Features route-based provider selection with sequential fallback, conflict resolution for tools/response_format incompatibility, and full support for multimodal messages (text + images).

## Tech Stack
- **Language**: Go 1.25+ (see `app/go.mod`)
- **Architecture**: REST API gateway with route-based provider selection
- **Configuration**: YAML-based with environment variable substitution and fail-fast missing-var checks
- **Deployment**: Systemd service or Docker; SSH-based [`ops.sh`](../../ops.sh) supports `deploy` (systemd) and `deploy-docker` (delegates to `scripts/sync-config-to-vds.sh`, same as CI: GHCR pull + bind-mounted config). **CI:** push to `main` builds/pushes GHCR image and SSH deploys `compose pull && up -d`.
- **Logging**: Structured JSON logging with API key redaction
- **Security**: API key authentication, TLS support

## Architecture Overview

### Route-Based Configuration System
- **Providers**: Store only connection details (name, api_key, base_url)
- **Routes**: Match incoming request models exactly, contain sequences of provider/model steps
- **Route Steps**: Define provider, model, timeout, and optional conflict resolution
- **Manager**: Looks up routes by model name, executes steps sequentially until success

### Key Components
- `main.go`: Entry point, loads config and starts server
- `config/`: Configuration loading and validation
- `providers/`: Provider management and client implementation
- `server/`: HTTP handlers and request processing
- `types/`: Data structures for requests/responses

### Observability
- `telemetry/`: OTLP/HTTP traces + log correlation via span events (see [key-components.md](#key-components.md)).
- **Production backend:** [Logfire](https://logfire.pydantic.dev) US (`OTLP_ENDPOINT=https://logfire-us.pydantic.dev`); use `OTLP_HEADERS=Authorization=<write-token>` (plain token, not Grafana Basic auth). Exporter still supports Grafana `glc_` tokens when configured.
- **Trace hierarchy:** `http.{path}` → `route/{name}` → `step/{model}`; structured logs attach as events on the active span when callers pass request context (see [decisions.md](#decisions.md)).

### Data Flow
1. Client sends request with specific model name
2. Server extracts model and looks up matching route
3. Manager executes route steps sequentially:
   - Creates provider client with route step configuration
   - Applies conflict resolution if specified
   - Calls provider API
   - Returns first successful response

### Conflict Resolution
Resolves "tools is incompatible with response_format" errors:
- `conflict_resolution: tools` → removes `response_format` field
- `conflict_resolution: format` → removes `tools` field

## Integration Points
- **OpenAI-Compatible APIs**: Works with any provider supporting OpenAI API format
- **Environment Variables**: All sensitive data via `${VAR_NAME}` syntax; missing vars fail fast
- **Systemd Service**: Managed deployment with automatic restarts
- **SSH Deployment**: Remote deployment via `ops.sh`
- **Observability**: Env-driven OTLP/HTTP to Logfire (or any collector); logs correlated as span events on the request/step trace tree.