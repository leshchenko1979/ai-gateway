# Contributing

## Issues and security

- **General bugs and features:** open a [GitHub issue](https://github.com/alexeyleshchenko/ai-gateway/issues) with what you expected, what happened, and how to reproduce when relevant.
- **Security vulnerabilities:** do **not** file a public issue. Follow [SECURITY.md](SECURITY.md).

## Development setup

1. Install **Go 1.25+** (match [`app/go.mod`](app/go.mod)).
2. Copy [`config.yaml.example`](config.yaml.example) to `config.yaml` and set the environment variables referenced in the file (see [README.md](README.md)).
3. Run tests from the `app/` directory (same as CI):

   ```bash
   cd app && go test ./... -count=1
   ```

4. Build the binary from the repository root:

   ```bash
   ./ops.sh build
   ```

5. Format Go code before submitting:

   ```bash
   cd app && gofmt -w .
   ```

### Observability (when changing logging or routes)

- Pass **request context** into `logger.Info` / `logger.Error` so OTLP exports log lines as span events on the active trace (e.g. `r.Context()` in handlers, `stepCtx` in `providers/manager.go`).
- Route steps use span name `step/{model}`; call `stepSpan.End()` before each `continue` in the route loop — do not `defer stepSpan.End()` inside that `for` loop.
- See [README.md — OpenTelemetry](README.md#opentelemetry-otlphttp) for env vars and collector examples.

## Pull requests

- Branch from `main`, keep commits focused, and reference an issue when one exists.
- Run `go test ./... -count=1` in `app/` and ensure tests pass.
- Match existing style: `gofmt` formatting, package layout, and logging patterns in touched files.
- Update user-facing docs ([README.md](README.md)) if your change affects behavior, configuration, or deployment.

## Maintainer: CI/CD and VDS deploy

Pushes to `main` run `go test ./... -count=1` in `app/`. If tests pass, GitHub Actions builds and pushes images to GHCR (`:main` and `:sha-<short>`), then deploys over SSH by running `docker compose pull && docker compose up -d`.

### Repository secrets

| Secret | Purpose |
|--------|---------|
| `SSH_HOST` | VDS hostname or IP |
| `SSH_USER` | SSH user |
| `SSH_PRIVATE_KEY` | Private key (PEM) for that user |
| `SSH_PORT` | Optional; defaults to `22` if omitted |
| `GHCR_PULL_USER` | Optional; GitHub username for `docker login` on the VDS when the package is private |
| `GHCR_PULL_TOKEN` | Optional; PAT with `read:packages` for that login |

### Runtime on VDS

- Keep a gitignored `config.yaml` and `.env` beside `docker-compose.yml`.
- CI pushes `ghcr.io/<github.repository_owner>/ai-gateway`. Compose defaults to `ghcr.io/alexeyleshchenko/ai-gateway` (`IMAGE_TAG` default `main`). Override with `GHCR_IMAGE` / `IMAGE_TAG` on the VDS.
- Set `GHCR_IMAGE` and `IMAGE_TAG` in VDS `.env` if you use a fork or pinned tag.

### Sync config and env from local machine

Use either:
- `./scripts/sync-config-to-vds.sh`
- `./ops.sh deploy-docker`

Both upload `docker-compose.yml`, `config.yaml`, and a filtered `.env` (SSH and `GHCR_PULL_*` lines are stripped before upload), then run `docker compose pull && docker compose up -d`.

In Cursor/VS Code, the non-default build task **Sync config and env to VDS** runs the same sync script.
