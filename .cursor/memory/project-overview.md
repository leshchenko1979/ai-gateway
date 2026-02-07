# AI Gateway - Route-Based Provider Configuration

## Project Description
A lightweight, OpenAI-compatible API gateway written in Go that routes requests to AI providers based on exact model name matching. Features route-based provider selection with sequential fallback, conflict resolution for tools/response_format incompatibility, and full support for multimodal messages (text + images).

## Tech Stack
- **Language**: Go 1.21+
- **Architecture**: REST API gateway with route-based provider selection
- **Configuration**: YAML-based with environment variable substitution and fail-fast missing-var checks
- **Deployment**: Systemd service with SSH-based deployment scripts
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
- `telemetry/`: Configures the OTLP exporter that streams traces and logger events to whichever collector is pointed to by `OTLP_ENDPOINT`, `OTLP_API_KEY`, and related env vars.

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
- **SSH Deployment**: Remote deployment via install.sh script
- **Observability**: Traces and structured logs flow through the env-driven OTLP exporter (see `telemetry/`), making them available in whichever backend `OTLP_ENDPOINT` targets.