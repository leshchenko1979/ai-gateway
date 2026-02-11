# AI Gateway

A lightweight, OpenAI-compatible API gateway written in Go that routes requests sequentially through configured providers until a successful response is received.

## Why Choose AI Gateway?

**Born from Frustration**: Created when Cloudflare AI Gateway unexpectedly started disconnecting users without explanation. This self-hosted alternative gives you full control with no vendor lock-in.

**Daily Use Case**: Connects to multiple AI providers with free tiers, automatically cycling between them when rate limits are hit - ensuring continuous service.

### Key Benefits
- **Lightweight**: ~20MB binary with minimal memory footprint
- **Fast**: Compiled Go with efficient runtime, no JVM overhead
- **Reliable**: Sequential provider fallback, automatic retry logic
- **Simple**: Single binary deployment, YAML configuration
- **Secure**: API key redaction, non-root execution, restrictive permissions
- **Compatible**: Drop-in replacement for OpenAI API in tools like n8n

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
./install.sh build                    # Build binary
./install.sh install-service         # Install as systemd service
sudo systemctl start ai-gateway      # Start service
```

3. **Or deploy remotely:**
```bash
cp .env.example .env                  # Configure SSH credentials
# Edit .env with your server details or put them into the command string
SSH_HOST=your-server.com ./install.sh deploy
```

Then use a reverse proxy like `nginx` or `traefik` to set up TLS termination and secure the traffic to your newly installed gateway.

## Configuration

The gateway uses YAML configuration with environment variable substitution:

```yaml
api_key: ${GATEWAY_API_KEY}  # Gateway authentication key
port: 8080                   # Optional, defaults to 8080
default_timeout: 300s        # Default timeout for requests

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
All endpoints except for `/health` require authentication.

Use `X-Api-Key` header or `Authorization: Bearer <token>` against configured gateway API key.

### Health Check
```bash
GET /health
```
Returns `{"status": "healthy"}` - no authentication required.

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

When you install via `./install.sh install-service`, the script copies `.env` into `/etc/ai-gateway/.env` and the generated systemd unit loads it via `EnvironmentFile`. After editing, run `sudo systemctl restart ai-gateway`.

## Development

```bash
go test -v ./...           # Run tests
go test -cover ./...       # Tests with coverage
go build -o ai-gateway .   # Build manually
./ai-gateway               # Run locally
```

## License

MIT