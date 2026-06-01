# Recent Changes

## RequestAdapter pipeline replaces conflict_resolution config (2026-06-01)

- **Problem:** `conflict_resolution: tools|format` was a manual config toggle. New model quirks (e.g. tool_choice incompatibility with thinking models) required adding config fields. Also, route authors shouldn't need to know about model-level quirks.
- **Solution:** `RequestAdapter` pipeline — always-on adapters that detect model capabilities and adapt the request automatically. No config toggle.
- **Config removed:** `conflict_resolution` field from `RouteStep` (types.go, config.go validation)
- **Client changes (`providers/client.go`):**
  - Added `type RequestAdapter func(request *types.ChatRequest) error`
  - `Client.conflictResolution string` → `Client.adapters []RequestAdapter`
  - `Call()` runs adapter pipeline before each provider request
  - `defaultAdapters()` returns both adapters:
    - `adaptConflictResolution`: removes `response_format` when `tools` is present (handles "tools" incompatibility)
    - `adaptToolChoice`: downgrades `tool_choice: "required"` → `"auto"` for thinking models (deepseek v4 flash, deepseek-reasoner)
  - `isThinkingModel()` helper detects thinking-capable models
- **`config.yaml.example`:** removed `conflict_resolution: tools` from route steps
- **Tests:** `manager_test.go`, `config_test.go`, `client_test.go` updated for always-on adapter behavior

## Logfire OTLP + trace/log correlation (2026-05-17)

- **Backend:** VDS `.env` switched from Grafana Cloud OTLP to **Logfire US** (`logfire-us.pydantic.dev`); sync via `scripts/sync-config-to-vds.sh`.
- **Logger:** `Info`/`Error` take `context.Context`; `telemetry.RecordLog` adds **span events** only when ctx carries a valid span (no orphan `log.record` spans).
- **Step spans:** Each route step is `step/{model}` under `route/{name}`; `stepCtx` used for logs and `provider.Call`. **Do not** `defer stepSpan.End()` inside the route `for` loop — call `stepSpan.End()` before `continue` and on success (see [decisions.md](#decisions.md)).
- **Server:** Handlers/middleware pass `r.Context()`; `writeErrorResponse` accepts ctx.

## README end-user refresh + maintainer docs move (2026-05-07)

- **README focus shift:** Rewrote `README.md` to be end-user first: clear app integration contract (base URL, route-name model mapping, auth), Docker-first quick start, shorter config/API sections, and simplified deploy options.
- **Maintainer detail relocation:** Moved CI/GHCR/VDS deployment specifics, secrets table, and sync script behavior into a new `CONTRIBUTING.md` maintainer section.
- **Result:** README is now more approachable for operators and client integrators, while operational depth remains preserved in contributor docs.

## Timeout hardening and schema strictness (2026-05-07)

- **Route vs step timeout correctness:** Fixed route-timeout classification so step-level request timeouts do not get mislabeled as `ROUTE_TIMEOUT`; fallback steps continue while route budget remains.
- **Positive-duration guard:** Timeout validation now rejects non-positive values (`<= 0`) for `default_route_timeout`, `default_step_timeout`, `route_timeout`, and `step_timeout`.
- **Strict config schema:** `LoadConfig()` now uses strict known-field decode (`KnownFields(true)`), so legacy/unknown keys (e.g. `default_timeout`, step `timeout`) fail startup instead of being silently ignored.
- **Server timeout derivation:** Removed upper clamp from effective HTTP server timeout derivation so configured route budgets above 24h are not preempted by server read/write timeout.
- **Tests:** Added regression coverage for step-timeout fallback behavior, unknown legacy timeout keys, non-positive timeout rejection, and large route budget preservation.

## Timeout validation consistency hardening (2026-05-07)

- Added shared `ValidateTimeoutSettings()` in `config` and reused it in full config validation.
- `providers.NewManager` and `server.NewServer` now fail fast on invalid timeout strings in programmatic configs, preventing silent runtime fallback differences.
- Added focused tests for timeout-only validation coverage.

## True route timeout + naming alignment (2026-05-07)

- **Config behavior:** Switched to user-facing timeout names: `default_route_timeout`, `route_timeout`, and `step_timeout` (breaking config change). Invalid route/step timeout strings fail config validation.
- **True route deadline:** Route execution now uses context deadline in manager logic, stopping retries immediately when route budget is exceeded.
- **Provider call cancellation:** Outbound provider requests now use context-bound HTTP requests so route deadline cancellation interrupts in-flight calls.
- **Error shape:** Added structured `ROUTE_TIMEOUT` responses with partial routing summary and step errors collected up to timeout.
- **Server timeout behavior:** HTTP server read/write timeouts remain derived from route step totals; route budget is now enforced primarily by manager route context.
- **Tests/docs:** Added coverage for route-timeout precedence and `ROUTE_TIMEOUT` response path; updated README and `config.yaml.example` to the new naming.

## README and public-repo docs (2026-03-28)

- **README:** CI test command (`-count=1`), GHCR tags, Go 1.25+ requirement, binary size (~13MB), `default_timeout` example vs default 30s, correct unauthenticated routes (`/health`, `/v1/diagnostics/upstream-models`) with diagnostics section and security note, fork-friendly n8n Base URL placeholder, workflow badge, links to contributing/security docs.
- **Badges (optional polish):** License (MIT) and Go version (from `app/go.mod`) alongside CI; License section links to `LICENSE`.
- **CONTRIBUTING.md:** Issues vs security, `gofmt`, PR checklist, doc update note.
- **SECURITY.md:** Scope (in/out), supported versions, link to contributing for non-security work.
- **New:** [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md).
- **Memory:** `project-overview.md` Go version aligned with `app/go.mod`.

## GitHub Actions + GHCR + VDS deploy (2026-03-28)

- **CI:** `.github/workflows/deploy.yml` — on push to `main`, `go test ./...` in `app/` then build/push image to `ghcr.io/leshchenko1979/ai-gateway` (`:main`, `:sha-*`) with GHA BuildKit cache; deploy job SSHs to VDS and runs `docker compose pull && up -d` in `/root/services/ai-gateway` (optional `GHCR_PULL_*` for private GHCR).
- **Compose:** `image:` + `GHCR_IMAGE` / `IMAGE_TAG`; bind-mount `./config.yaml:/app/config.yaml:ro`.
- **Dockerfile:** `COPY config.yaml.example` as `/app/config.yaml` (real config from mount at runtime).
- **Ops:** `scripts/sync-config-to-vds.sh` uploads `docker-compose.yml`, `config.yaml`, filtered `.env`; VS Code task **Sync config and env to VDS** (non-default build). `.env.example` documents image vars and optional pull credentials.
- **Rename:** `install.sh` → **`ops.sh`** (same commands; header/help point to GHCR + sync script for Docker). **`deploy-docker`** now calls `scripts/sync-config-to-vds.sh` (pull + bind mount), not remote `docker compose build`.

## Upstream models diagnostics HTTP endpoint (2026-03-24)

- `GET /v1/diagnostics/upstream-models` (no auth): parallel checks of each provider’s `GET {base_url}/models`; JSON `ok` + per-provider results; **503** if any provider fails, **200** if all succeed.
- Shared logic in `providers/model_check.go`; CLI `cmd/check-models` calls the same helper.
- Task **Query deployed upstream models check** runs `scripts/query-deployed-upstream-models.sh` (uses `DOMAIN` from `.env`, `curl` + `python3 -m json.tool`) so VS Code does not strip `$DOMAIN` from inline task commands.

## Provider upstream model list check (2026-03-24)

### Overview
Added `app/cmd/check-models` CLI: loads `config.yaml` (same env substitution as the gateway), `GET`s OpenAI-style `{base_url}/models` per provider with Bearer auth, prints OK/model count or errors, exits 1 on any failure. VS Code/Cursor task **Verify provider model lists** in `.vscode/tasks.json` runs it with `cwd` `app/` and `envFile` `.env`.

### Files
- `app/cmd/check-models/main.go`, `.vscode/tasks.json`

## Code Restructure: All Source in app/ (2026-03-17)

### Overview
Moved all Go source code into a single `app/` folder for simpler Docker builds and cleaner separation from config.

### Changes Made
1. **Layout**
   - `app/` now contains: go.mod, go.sum, main.go, config/, server/, providers/, logger/, telemetry/, types/, test/
   - `config.yaml` stays at project root (excluded from build; copied as last layer in Docker)
2. **Dockerfile**
   - Builder: `COPY app/ ./` — one copy for all code; config excluded
   - Final stage: `COPY config.yaml .` from context as last layer
3. **install.sh**
   - `build` and `build_linux`: `cd app && go build -o ../$BINARY_NAME .`

### Result
- Single `COPY app/` in Dockerfile
- Config-only changes: fast rebuild (builder cached)
- config.yaml remains at root

### Files Modified
- `Dockerfile`
- `install.sh`
- Created `app/test/config.yaml` for config tests

## Config-only Docker Deploy – Hybrid (2026-03-17)

### Overview
Reordered Dockerfile layers and removed `--no-cache` so config-only changes trigger a fast rebuild (seconds) instead of a full Go build.

### Changes Made
1. **Dockerfile**
   - Builder stage: Replaced `COPY . .` with explicit copy so `config.yaml` is excluded from the build (later simplified to `COPY app/` with code in app/)
   - Final stage: `COPY config.yaml .` from build context (not from builder) as the last layer
2. **install.sh**
   - Removed `--no-cache` from `deploy-docker` so Docker reuses layer cache when context is unchanged

### Result
- Config change: builder cached, rebuild in seconds
- Code change: full rebuild
- No change: fully cached, no rebuild

### Files Modified
- `Dockerfile`
- `install.sh`

## Docker Deployment & DOMAIN Configuration (2026-02-16)

### Overview
Added Docker-based deployment option and made Traefik domain configurable via `.env`.

### Changes Made
1. **Docker Setup**
   - Added `Dockerfile`, `docker-compose.yml`, `.dockerignore`
   - `deploy-docker` in `install.sh`: tars source, Dockerfile, config, filtered .env; SSHs to remote and runs `docker compose build && docker compose up -d`
   - VSCode task `deploy-docker` in `.vscode/tasks.json`

2. **DOMAIN from .env**
   - Traefik Host rule uses `${DOMAIN:-ai-gateway.redevest.ru}` in docker-compose
   - Added `DOMAIN` to `.env.example` and documented in README

### Files Modified
- `install.sh`: `deploy-docker` command
- `docker-compose.yml`: `Host(\`${DOMAIN:-...}\`)` for Traefik
- `.env.example`: `DOMAIN` variable
- `README.md`: Docker Installation section updated for DOMAIN
- `.vscode/tasks.json`: `deploy-docker` task

### Untracked (new)
- `Dockerfile`, `docker-compose.yml`, `.dockerignore`

### Git Status Note
Branch diverged from origin (1 local, 3 remote). Modified: .env.example, README.md, install.sh. Untracked: .dockerignore, Dockerfile, docker-compose.yml.

## Config Env Var Fail-Fast Validation (2026-02-07)

### Overview
Added fail-fast validation for missing `${VAR}` references in `config.yaml`, so startup errors clearly list missing environment variables (especially API keys).

### Changes Made
1. **Env Var Validation**
   - Added missing env var detection before YAML unmarshal
   - Error lists all missing variable names in sorted order
2. **Tests & Docs**
   - Added config test for missing env var failure
   - Documented fail-fast behavior in configuration docs

### Files Modified
- `config/config.go`: Missing env var detection during config load
- `config/config_test.go`: Added missing env var test
- `README.md`: Documented fail-fast behavior
- `config.yaml.example`: Added fail-fast note

### Impact
- Clear startup errors when API keys or other env vars are missing
- More predictable deployments with explicit configuration failures

## Multimodal Message Support: Enhanced Content Parsing (2026-02-04)

### Overview
Added support for OpenAI's multimodal message format where content can be either a string (text-only) or an array of content blocks (text + images). Enhanced error logging to include request details when validation fails.

### Changes Made
1. **Message Structure Update**
   - Changed `Message.Content` from `string` to `json.RawMessage` to support both formats
   - Added helper methods: `ContentAsString()`, `ContentAsArray()`, `IsContentString()`, `IsContentArray()`

2. **Enhanced Validation**
   - Updated `validateChatRequest()` to accept both string and array content formats
   - Added specific validation for multimodal content arrays (must have elements)
   - Improved error messages to distinguish between different validation failures

3. **Improved Error Logging**
   - Validation failures now include truncated request JSON in error logs
   - Helps debug parsing errors by showing exact request structure that caused issues

4. **Test Updates**
   - Fixed test comparisons to use new `ContentAsString()` method
   - All tests passing after changes

### Files Modified
- `types/types.go`: Message struct and helper methods, content truncation logic
- `server/validation.go`: Enhanced validation for multimodal content
- `server/handlers.go`: Added request details to validation error logs
- `providers/manager_test.go`: Updated test assertions for new content format

### Impact
- Gateway now supports multimodal requests (text + images) as per OpenAI API specification
- Better debugging capability when message parsing errors occur
- Backward compatible with existing string-only content requests
- No breaking changes to API contract

### Root Cause
The original error `"failed to parse messages: json: cannot unmarshal array into Go struct field Message.messages.content of type string"` occurred because OpenAI's API allows message content to be either:
- A string (simple text messages)
- An array of content blocks (multimodal messages with text + images)

## API Behavior Change: Models Endpoint Now Returns Route Names (2026-02-04)

### Overview
Modified `/v1/models` endpoint to return configured route names instead of hardcoded model list.

### Changes Made
1. **Handler Update**
   - Changed `handleModels()` to iterate through configured routes
   - Returns route names as available model IDs
   - Removed hardcoded "dynamic/model" response

2. **Test Updates**
   - Updated `TestHandleModels` to verify route name listing
   - Tests now expect actual route names from configuration

### Files Modified
- `server/handlers.go`: Updated models endpoint implementation
- `server/handlers_test.go`: Updated test expectations
- `README.md`: Updated API documentation

### Impact
- API consumers now see actual configured route names when listing models
- Route names serve as model identifiers for chat completion requests
- Behavior aligns with the documented "returns available models from configured routes"

## Major Refactoring: Route-Based Provider Configuration (2026-02-04)

### Overview
Complete architectural refactoring from provider-centric to route-based configuration system.

### Changes Made
1. **Configuration Structure Overhaul**
   - Renamed `providers.yaml` → `config.yaml`
   - Split provider configuration from routing logic
   - Added route-based model matching with exact name matching

2. **New Configuration Format**
   ```yaml
   providers:
     - name: provider1
       api_key: key
       base_url: url

   routes:
     - name: dynamic/n8n  # exact match required
       steps:
         - provider: provider1
           model: gpt-4
           conflict_resolution: tools
   ```

3. **Conflict Resolution Feature**
   - Added `conflict_resolution` field to route steps
   - `tools`: removes `response_format` field
   - `format`: removes `tools` field
   - Solves "tools is incompatible with response_format" errors

4. **Provider Manager Refactoring**
   - Changed from sequential provider fallback to route-based selection
   - Added exact model name matching for route lookup
   - On-demand provider client creation per route step

5. **Error Handling Updates**
   - Route not found returns 404 instead of trying all providers
   - Better error messages for route lookup failures
   - Updated logging to include route and step information

### Files Modified
- `config/types.go`: New Route/RouteStep types
- `config/config.go`: Updated validation and loading
- `providers/manager.go`: Route-based provider selection
- `providers/client.go`: Conflict resolution logic
- `main.go`: Config filename change
- `server/handlers.go`: Error handling updates
- All test files: Updated for new structure
- `README.md`: Documentation updates
- `install.sh`: Script updates

### Migration
- Existing `providers.yaml` migrated to new `config.yaml` format
- Backward compatibility maintained during transition
- Default timeout logic added (30s fallback)

### Testing
- Updated all unit tests for new configuration structure
- Added tests for conflict resolution functionality
- Added tests for route lookup and step execution

## Ongoing Work
- Deploy/commit step-span + logger-context changes if still only local
- Optional: child HTTP spans in `providers/client.go`; unit tests for `RecordLog` / step span end timing

## Blockers
- None

## Observability: Generic OTLP telemetry (2026-02-05)

### Overview
Added environment-driven telemetry so traces and structured logs stream to whichever OTLP backend operators configure, keeping the stack vendor-agnostic.

### Changes Made
1. **Telemetry Core**
   - Introduced `telemetry/` to initialize trace providers, build resource attributes, and expose a reusable `RecordLog` helper that replays logger entries into OTLP.
   - Logger now posts every `Info`/`Error` call through `telemetry.RecordLog`, aligning logs with trace data.
2. **Request Spans**
   - HTTP routes are wrapped with a lightweight tracer that annotates HTTP metadata and propagates the context into route execution.
   - `providers.Manager` now records route/step spans, durations, and errors before falling back between providers.
3. **Docs & Configuration**
   - Documented `OTLP_ENDPOINT`, `OTLP_API_KEY`, `OTLP_SERVICE_NAME`, `OTLP_RESOURCE_ATTRIBUTES`, and `OTLP_HEADERS` in `README.md` and `.env.example`.
   - Kept `config.yaml.example` OTLP-free while `.env.example` highlights the shared telemetry settings.

### Impact
- Observability is no longer tied to Grafana Cloud; any OTLP collector can receive telemetry.
- Routes, steps, and logger output now appear together, making distributed tracing easier to correlate with structured logs.

## OTLP Exporter Switch & Auth Fix (2026-02-05)

### Overview
Switched from gRPC to HTTP for OTLP exporting to improve compatibility and resolved authentication issues with Grafana Cloud.

### Changes Made
1. **HTTP Exporter Migration**
   - Replaced `otlptracegrpc` with `otlptracehttp`.
   - Bypasses common gRPC/ALPN handshake issues on restricted networks.
   - Improved endpoint normalization to handle both `host:port` and full URLs with paths.

2. **Grafana Cloud Auth Automation**
   - Added automatic detection and parsing of `glc_` prefixed Access Policy Tokens.
   - Extracts **Stack ID** from the token's base64-encoded JSON to use as the Basic auth username.
   - Fallback to standard Basic auth (`apiKey:`) for other token types.

3. **Path Normalization**
   - Enhanced `normalizeEndpoint` to ensure the `/v1/traces` signal path is correctly appended or handled when a custom path (like `/otlp`) is provided.

4. **Standard OTEL Env Var Support**
   - Added support for `OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES`.
   - Maintained backward compatibility with `OTLP_` prefixed variables.

### Impact
- Telemetry now works reliably on VDS environments where gRPC might be unstable or blocked.
- Zero-config authentication for Grafana Cloud users (just provide the `glc_` token).
- Better compliance with standard OpenTelemetry environment variables.
