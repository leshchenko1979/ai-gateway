# AI Gateway

[![Build and deploy](https://github.com/leshchenko1979/ai-gateway/actions/workflows/deploy.yml/badge.svg)](https://github.com/leshchenko1979/ai-gateway/actions/workflows/deploy.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/leshchenko1979/ai-gateway?filename=app%2Fgo.mod&logo=go&label=go)](app/go.mod)

AI Gateway is a lightweight, OpenAI-compatible gateway you can self-host. You define one route per model name your app uses, and the gateway tries providers in order until one succeeds. This is useful when you want fewer failures from provider rate limits or temporary outages.

## Connect Your App

Use this contract in any OpenAI-compatible client (n8n, scripts, apps):

- Base URL: `https://YOUR_DOMAIN/v1` (or `http://localhost:8080/v1` locally)
- Model: a route `name` from `config.yaml` (not the upstream provider model id)
- Auth: `X-Api-Key: <GATEWAY_API_KEY>` or `Authorization: Bearer <GATEWAY_API_KEY>`

For n8n, set:
- Base URL: `https://YOUR_DOMAIN/v1`
- Model: a route name from `config.yaml`
- API key: your `GATEWAY_API_KEY`

## Quick Start (Docker)

1. Copy the config template:

```bash
cp config.yaml.example config.yaml
```

2. Fill in your env vars (`GATEWAY_API_KEY`, provider keys such as `CEREBRAS_API_KEY`, `OPENROUTER_API_KEY`) and keep `${VAR}` references in `config.yaml`.

3. If needed, copy and fill deployment env:

```bash
cp .env.example .env
```

4. Deploy:

```bash
./ops.sh deploy-docker
```

5. Verify:

```bash
curl http://localhost:8080/health
```

If you deploy behind Traefik, set `DOMAIN` in `.env` and use `https://YOUR_DOMAIN/v1` as your client base URL.

## Configure Providers and Routes

The gateway reads YAML with env var substitution. Start from [`config.yaml.example`](config.yaml.example).

```yaml
api_key: ${GATEWAY_API_KEY}
port: 8080
default_step_timeout: 30s
default_route_timeout: 300s

providers:
  - name: cerebras
    api_key: ${CEREBRAS_API_KEY}
    base_url: https://api.cerebras.ai/v1
  - name: openrouter
    api_key: ${OPENROUTER_API_KEY}
    base_url: https://openrouter.ai/api/v1

routes:
  - name: n8n-heavy
    route_timeout: 5m
    steps:
      - provider: cerebras
        model: gpt-oss-120b
        step_timeout: 5m
        conflict_resolution: tools
      - provider: openrouter
        model: nvidia/nemotron-3-nano-30b-a3b:free
        step_timeout: 5m
```

Timeouts accept Go durations like `1s`, `30s`, `5m`, `2h` and must be greater than `0`.

Config lookup order:
1. `./config.yaml`
2. `/etc/ai-gateway/config.yaml`

Missing `${VAR}` values fail startup with a clear list, and unknown YAML fields also fail startup.

## API Cheat Sheet

Unauthenticated:
- `GET /health`
- `GET /v1/diagnostics/upstream-models`

Authenticated (`X-Api-Key` or `Authorization: Bearer`):
- `GET /v1/models` returns route names
- `POST /v1/chat/completions` uses the route name in `model`

Example chat endpoint:

```bash
curl -X POST "http://localhost:8080/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: YOUR_GATEWAY_API_KEY" \
  -d '{
    "model": "n8n-heavy",
    "messages": [{"role":"user","content":"Hello"}]
  }'
```

Responses include `routing_summary` so you can see which providers and steps were tried.

Diagnostics warning: `/v1/diagnostics/upstream-models` is unauthenticated and performs outbound requests. Do not expose it publicly without network restrictions (allowlists, VPN, or private ingress).

## Other Ways To Run

Use [`ops.sh`](ops.sh):

- Local binary build: `./ops.sh build`
- Local systemd install: `./ops.sh install-service`
- Remote systemd deploy over SSH: `./ops.sh deploy`
- Binary-only install: `./ops.sh install`

For systemd deployments, terminate TLS at a reverse proxy such as `nginx` or `traefik`.

## Security and Telemetry

- Security defaults include API key redaction, non-root execution, and restrictive file permissions.
- Run behind TLS in production.
- OpenTelemetry over OTLP/HTTP is supported via `OTLP_ENDPOINT`, `OTLP_API_KEY`, optional `OTEL_SERVICE_NAME` or `OTLP_SERVICE_NAME`, optional `OTEL_RESOURCE_ATTRIBUTES` or `OTLP_RESOURCE_ATTRIBUTES`, and optional `OTLP_HEADERS`.

## Maintainer Notes

CI/CD, GHCR, VDS deployment workflow, deploy secrets, and sync details are documented in [`CONTRIBUTING.md`](CONTRIBUTING.md).

## Contributing

Bug reports and pull requests are welcome. See [`CONTRIBUTING.md`](CONTRIBUTING.md). For security-sensitive issues, follow [`SECURITY.md`](SECURITY.md).

## License

[MIT](LICENSE)