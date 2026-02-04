# AI Gateway

A lightweight, OpenAI-compatible API gateway written in Go that routes requests sequentially through configured providers until a successful response is received.

## Why AI Gateway?

**Project Origins**  
This project was born out of frustration when Cloudflare AI Gateway unexpectedly started disconnecting users without explanation. As an alternative that puts you in control, AI Gateway provides transparent, self-hosted routing with no vendor lock-in or sudden service interruptions.

**Lightweight & Efficient**
- ~10MB binary size with minimal memory footprint
- Fast startup with no JVM or interpreter overhead
- Efficient Go runtime optimized for low resource consumption

**Fast & Scalable**
- Compiled Go language delivers high performance
- Automatic goroutine-based concurrency for handling multiple requests
- Single binary deployment with no external runtime dependencies

**Simple & Reliable**
- Statically compiled with minimal dependencies (only YAML parser)
- Cross-platform compilation support (Linux, macOS, Windows, ARM)
- Type-safe with compile-time error checking

## Features

- **OpenAI-Compatible API**: Works with any OpenAI-compatible client
- **Sequential Provider Fallback**: Automatically tries providers in order until one succeeds
- **Flexible Configuration**: YAML-based configuration with environment variable support
- **Security**: API key authentication, TLS support, secure logging
- **Health Monitoring**: Built-in health check endpoint
- **Structured Logging**: JSON logging with API key redaction

## Installation

### Prerequisites

- Go 1.21 or later
- Linux system with systemd (for service installation)

### Build

```bash
./install.sh build
```

### Install Binary

```bash
./install.sh install
```

### Install as Systemd Service (Local)

```bash
./install.sh install-service
```

This will:
- Create a `ai-gateway` system user
- Install the binary to `/usr/local/bin`
- Create configuration directory at `/etc/ai-gateway`
- Install and enable the systemd service

### Deploy to Remote Server

```bash
SSH_HOST=example.com SSH_USER=deploy ./install.sh deploy
```

Or with SSH key:

```bash
SSH_HOST=example.com SSH_USER=deploy SSH_KEY=~/.ssh/id_rsa ./install.sh deploy
```

The deploy command will:
1. Build the binary locally
2. Connect to the remote server via SSH
3. Stop and remove the old service version
4. Copy the new binary, service file, and config to the remote server
5. Install and start the new service

**Environment Variables:**
- `SSH_HOST` - Remote server hostname or IP (required)
- `SSH_USER` - SSH user (default: root)
- `SSH_KEY` - Path to SSH private key (optional, uses default SSH key)
- `SSH_PORT` - SSH port (default: 22)

## Configuration

Create a `config.yaml` file (or copy the example):

```yaml
api_key: ${GATEWAY_API_KEY}  # Required: Gateway API key
port: 8080  # Optional, defaults to PORT env var or 8080
default_timeout: 300s  # Optional, used when route steps don't specify timeout

providers:
  - name: cerebras
    api_key: ${CEREBRAS_API_KEY}
    base_url: https://api.cerebras.ai/v1

  - name: openrouter
    api_key: ${OPENROUTER_API_KEY}
    base_url: https://openrouter.ai/api/v1

routes:
  - name: dynamic/n8n  # Must match request model exactly
    steps:
      - provider: cerebras
        model: gpt-oss-120b
        timeout: 300s  # Optional, uses default_timeout if omitted
        conflict_resolution: tools  # Optional: "tools" or "format"
      - provider: openrouter
        model: nvidia/nemotron-3-nano-30b-a3b:free
        timeout: 300s
        conflict_resolution: format
```

### Configuration Locations

The gateway looks for `config.yaml` in:
1. Current directory (`./config.yaml`)
2. `/etc/ai-gateway/config.yaml`

### Environment Variables

All API keys can be provided via environment variables using `${VAR_NAME}` syntax in the YAML file.

**Required:**
- `GATEWAY_API_KEY`: API key for gateway authentication

**Per Provider:**
- `PROVIDER1_API_KEY`, `PROVIDER2_API_KEY`, etc.: API keys for each provider

### Configuration Security

- Configuration file should have permissions `600` (owner read/write only)
- Prefer environment variables over hardcoded keys in YAML
- Service runs as non-root user `ai-gateway`

## API Endpoints

### Health Check

```bash
GET /health
```

Returns: `{"status": "healthy"}`

No authentication required.

### List Models

```bash
GET /v1/models
Headers:
  X-Api-Key: <gateway-api-key>
  # OR
  Authorization: Bearer <gateway-api-key>
```

Returns:
```json
{
  "object": "list",
  "data": [
    {
      "id": "dynamic/model",
      "object": "model",
      "created": 1677610602,
      "owned_by": "ai-gateway"
    }
  ]
}
```

### Chat Completions

```bash
POST /v1/chat/completions
Headers:
  X-Api-Key: <gateway-api-key>
  Content-Type: application/json
Body:
{
  "model": "dynamic/model",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ]
}
```

The gateway will:
1. Look up a route that matches the request model exactly
2. If no route found, return 404 error
3. For the matching route, try each step sequentially:
   - Use the step's provider and model configuration
   - Apply conflict resolution if specified (remove tools or response_format)
4. Return the first successful response

## Authentication

The gateway supports two authentication methods:

1. **X-Api-Key header**: `X-Api-Key: <gateway-api-key>`
2. **Authorization header**: `Authorization: Bearer <gateway-api-key>`

Both methods are checked against the configured `api_key` in `config.yaml`.

## Service Management

```bash
# Start the service
sudo systemctl start ai-gateway

# Stop the service
sudo systemctl stop ai-gateway

# Enable auto-start on boot
sudo systemctl enable ai-gateway

# Check status
sudo systemctl status ai-gateway

# View logs
sudo journalctl -u ai-gateway -f
```

## Development

### Run Tests

```bash
./install.sh test
```

### Run Tests with Coverage

```bash
./install.sh test-coverage
```

### Manual Build and Run

```bash
go build -o ai-gateway .
./ai-gateway
```

## Security Considerations

- **TLS/HTTPS**: Gateway requires TLS connections (configure via systemd or environment)
- **API Key Redaction**: All API keys are automatically redacted from logs
- **Input Validation**: Moderate validation of request structure
- **Non-root Execution**: Service runs as dedicated `ai-gateway` user
- **File Permissions**: Configuration files use restrictive permissions (600)

## Logging

The gateway uses structured JSON logging:

```json
{
  "level": "INFO",
  "message": "Trying provider",
  "fields": {
    "provider": "provider1"
  }
}
```

- API keys are automatically redacted
- Request/response content is logged as summaries only
- Full error details are logged server-side for debugging

## Error Handling

- Any error (network, HTTP error, timeout) triggers fallback to the next provider
- If all providers fail, returns `502 Bad Gateway` with error details
- Detailed error messages include provider name and error information

## License

MIT