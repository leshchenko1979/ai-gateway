# Key Components & Data Flows

## Core Modules

### Configuration System (`config/`)
- **`types.go`**: Data structures for Config, Provider, Route, RouteStep (`default_route_timeout`, `route_timeout`, `step_timeout`)
- **`config.go`**: YAML loading with env var substitution and validation
- **Key Functions**:
  - `LoadConfig()`: Loads and validates configuration
  - `ValidateTimeoutSettings()`: Reusable timeout validation for both file-loaded and programmatic configs
  - Strict decode (`KnownFields(true)`): Unknown YAML keys fail fast during config load
  - `GetStepTimeout()`: Resolves per-step timeout (`step_timeout` -> 30s fallback)
  - `GetRouteTimeout()`: Resolves route budget (`route_timeout` -> `default_route_timeout` -> derived/fallback)
  - `MaxSequentialRouteDuration()`: Computes worst-case per-request route duration by summing step timeouts per route and taking max route sum
  - `EffectiveHTTPServerTimeouts()`: Derives server read/write timeout from effective route budget with a 30s floor (no upper clamp)
  - Route/provider validation with cross-reference checking
- **Ops**: Task **Verify provider model lists** runs `go run ./cmd/check-models` from `app/` (see `.vscode/tasks.json`) to confirm each provider’s upstream `/models` endpoint.

### Logger (`logger/`)
- **`Info(ctx, ...)` / `Error(ctx, ...)`**: Structured JSON to stdout + `telemetry.RecordLog(ctx, ...)` for OTLP correlation.
- Callers must pass request/step context (not `context.Background()`) so log lines appear as span events on the active trace.

### Provider Management (`providers/`)
- **`manager.go`**: Route-based provider execution with OpenTelemetry spans
- **`client.go`**: HTTP client with RequestAdapter pipeline (no nested HTTP spans yet)
- **Key Functions**:
  - `Manager.GetRoute()`: Exact model name matching
  - `Manager.ExecuteWithTracing()`: Route span + per-step `step/{model}` span; `stepCtx` for logs and `provider.Call`; explicit `stepSpan.End()` per iteration (not `defer` in `for`)
  - `NewManager(cfg, ...)`: Uses config-level timeout policy (`default_route_timeout`, `route_timeout`, `step_timeout`)
  - `Client.Call()` runs `RequestAdapter` pipeline before each provider call:
    - `adaptConflictResolution`: removes `response_format` when `tools` is present (always-on)
    - `adaptToolChoice`: downgrades `tool_choice: "required"` to `"auto"` for thinking models (deepseek v4 flash, deepseek-reasoner)

### Server Layer (`server/`)
- **`handlers.go`**: HTTP request/response handling
- **`server.go`**: Server setup and routing
- **Key Functions**:
  - `NewServer()`: Uses config-derived effective read/write timeout instead of fixed 30s
  - `handleChatCompletions()`: Main request processing
  - `handleUpstreamModelsCheck()`: `GET /v1/diagnostics/upstream-models` — unauthenticated; validates each provider’s upstream models API (same as CLI check)
  - Route lookup and error handling
  - Request/response logging with truncation

### Types (`types/`)
- **`types.go`**: OpenAI-compatible data structures
- **Key Features**:
  - Raw JSON preservation for passthrough
  - Selective field extraction for logging
  - Request/response truncation for security
  - **Multimodal content support**: Message.Content handles both string and array formats
  - Helper methods for content type detection and extraction

### Telemetry (`telemetry/`)
- **Purpose**: OTLP/HTTP trace export; log lines become **span events** on the active span (not separate log spans).
- **Key Components**:
  - **`newTraceExporter()`**: Grafana `glc_` Basic auth automation; **Logfire** needs `OTLP_HEADERS=Authorization=<write-token>` to override default Basic auth.
  - **`normalizeEndpoint()`**: Appends `/v1/traces` when a custom path is given.
  - **`RecordLog(ctx, ...)`**: `span.AddEvent` when `trace.SpanFromContext(ctx)` is valid; no-op otherwise (no orphan spans).
  - **Env vars**: `OTLP_ENDPOINT`, `OTLP_API_KEY` (both required to enable), `OTLP_SERVICE_NAME`, `OTLP_RESOURCE_ATTRIBUTES`, `OTLP_HEADERS`; also `OTEL_*` aliases.

### Trace span tree (chat requests)
```
http.{path}  (server.instrument)
└── route/{routeName}  (Manager)
    └── step/{model}  (Manager; step.index attribute if same model repeated)
        ├── span events: Trying route step, Route step succeeded|failed
        └── provider HTTP (context-bound; no child span yet)
```

## Data Flow: Request Processing

```mermaid
graph TD
    A[Client Request] --> B[server.handleChatCompletions]
    B --> C[Parse & Validate Request]
    C --> D[Extract Model Name]
    D --> E[providers.Manager.GetRoute]
    E --> F{Route Found?}
    F -->|No| G[Return 404]
    F -->|Yes| H[Execute Route Steps]
    H --> I[Create Provider Client]
    I --> J[Apply Request Adapters]
    J --> K[Call Provider API]
    K --> L{Success?}
    L -->|No| M[Try Next Step]
    L -->|Yes| N[Return Response]
    M --> O{More Steps?}
    O -->|Yes| H
    O -->|No| P[All Steps Failed - Return 502]
```

## Integration Points

### Configuration Loading
- File lookup: `./config.yaml` → `/etc/ai-gateway/config.yaml`
- Environment variable expansion: `${VAR_NAME}` syntax with missing-var fail fast
- Validation: Provider/route cross-references, strict known-field schema, timeout parsing with `> 0` enforcement

### Provider Communication
- HTTP POST to `/v1/chat/completions`
- Bearer token authentication
- Request field manipulation via adapter pipeline (`adaptConflictResolution`, `adaptToolChoice`)
- Response passthrough (unchanged)

### Error Handling
- Route not found: 404 with model name
- Provider errors: 502 with detailed error messages
- Configuration errors: Fail fast on startup
- Logging: Structured JSON with field redaction

## Security Considerations

### API Key Management
- Environment variables for all sensitive data
- File permissions: 600 (owner read/write only)
- Logging redaction: Automatic API key removal
- Service user: Non-root `ai-gateway` user

### Request Processing
- Input validation: Basic structure checking
- Content truncation: Message content limited in logs
- TLS requirement: External TLS termination expected
- Timeout enforcement: Per-route step configuration

## Deployment Integration

### Systemd Service
- Service file: `ai-gateway.service`
- User management: Dedicated service account
- Log management: journalctl integration
- Auto-restart: Service failure recovery

### SSH Deployment (systemd)
- Script: `ops.sh deploy`
- Remote operations: Binary copy, config deployment
- Service management: Stop/start/restart cycle

### Docker Deployment
- **CI/CD:** `.github/workflows/deploy.yml` — build/push `ghcr.io/leshchenko1979/ai-gateway`, SSH deploy `docker compose pull && up -d` on VDS (`/root/services/ai-gateway`). Secrets: `SSH_*`, optional `GHCR_PULL_*` for private registry.
- **Runtime:** Compose uses pulled `image:` + bind-mount `./config.yaml`; Dockerfile bakes `config.yaml.example` only.
- **Sync:** `scripts/sync-config-to-vds.sh` pushes `docker-compose.yml`, `config.yaml`, filtered `.env`; task **Sync config and env to VDS** in `.vscode/tasks.json`.
- **CLI deploy:** `ops.sh deploy-docker` delegates to `scripts/sync-config-to-vds.sh` (compose pull, bind-mounted `config.yaml`, same as CI)
- Files: `Dockerfile`, `docker-compose.yml`, `.dockerignore`
- Traefik: Host rule uses `DOMAIN` from `.env`