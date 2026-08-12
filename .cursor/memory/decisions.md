# Architectural Decisions

## Route-Based Configuration (2026-02-04)

### Decision
Implement route-based provider selection where routes match incoming request models exactly, rather than trying all providers sequentially.

### Context
- Original system tried all configured providers sequentially until one succeeded
- Client requests specify model names that should map to specific provider/model combinations
- Need to resolve "tools is incompatible with response_format" conflicts between providers
- Configuration was tightly coupled: each provider had fixed model and timeout

### Options Considered

#### Option 1: Keep Sequential Provider Fallback
- Pros: Simple, no breaking changes
- Cons: No model-specific routing, conflict resolution not possible

#### Option 2: Route-Based with Pattern Matching
- Pros: Flexible model matching (wildcards, prefixes)
- Cons: More complex, potential for ambiguous matches

#### Option 3: Route-Based with Exact Matching (Chosen)
- Pros: Explicit, predictable, solves conflict resolution need
- Cons: Requires exact model name knowledge in configuration

### Rationale
- **Exact matching**: Ensures predictable behavior and clear configuration
- **Conflict resolution**: Allows different providers to handle tools/format differently
- **Separation of concerns**: Providers define connections, routes define behavior
- **Flexibility**: Each route can have different fallback strategies

### Implementation Details
- Routes match request model exactly (case-sensitive)
- Route steps executed sequentially until success
- Conflict resolution applied per route step
- Default timeout with per-step overrides
- Provider configurations reusable across routes

## Conflict Resolution Strategy

> **SUPERSEDED (2026-06-01):** Replaced by always-on `RequestAdapter` pipeline in `providers/client.go`.  
> `adaptConflictResolution` (removes `response_format` when `tools` is present) runs automatically — no config needed.  
> The `conflict_resolution` field has been removed from `RouteStep` config. See `adapters []RequestAdapter` in `Client`.

### Decision
Add optional `conflict_resolution` field to route steps with "tools" or "format" values.

### Context
- Some providers reject requests with both `tools` and `response_format` fields
- Error: `"tools" is incompatible with "response_format"`
- Need to support both tools and structured output use cases

### Options Considered

#### Option 1: Fail on Conflict
- Pros: Simple implementation
- Cons: Breaks existing functionality

#### Option 2: Automatic Detection
- Pros: No configuration needed
- Cons: Unpredictable behavior, potential data loss

#### Option 3: Explicit Configuration (Chosen)
- Pros: Predictable, explicit control, preserves intent
- Cons: Requires configuration knowledge

### Rationale
- **Explicit control**: Configuration clearly states intent
- **Provider compatibility**: Allows different providers to handle conflicts differently
- **Backward compatibility**: Optional field, defaults to passthrough
- **Future extensibility**: Can add more resolution strategies

### Implementation
- `conflict_resolution: tools` → remove `response_format`
- `conflict_resolution: format` → remove `tools`
- Applied before sending request to provider

## Configuration File Naming

### Decision
Rename `providers.yaml` to `config.yaml` to reflect broader scope.

### Context
- Original file contained only provider configurations
- New file includes routes, timeouts, and gateway settings
- Installation scripts and documentation referenced old name

### Options Considered

#### Option 1: Keep `providers.yaml`
- Pros: Backward compatibility
- Cons: Misleading name, doesn't reflect route configuration

#### Option 2: New name `routes.yaml`
- Pros: Emphasizes routing aspect
- Cons: Still incomplete, includes provider and gateway config

#### Option 3: `config.yaml` (Chosen)
- Pros: Generic, accurate, follows convention
- Cons: Less specific about contents

### Rationale
- **Standard naming**: `config.yaml` is conventional for application configuration
- **Future-proof**: Accommodates additional configuration sections
- **Clear scope**: Not just providers or routes, but complete gateway configuration

## Timeout Handling

### Decision
Implement explicit multi-level timeout semantics:
- `step_timeout` for individual provider attempts,
- `route_timeout` for route-level execution budget,
- `default_route_timeout` as global route budget fallback.

### Context
- Different providers may need different timeouts
- Some operations require longer timeouts than others
- Need sensible defaults for reliability

### Options Considered

#### Option 1: Global timeout only
- Pros: Simple
- Cons: No per-provider flexibility

#### Option 2: Per-provider timeouts
- Pros: Provider-specific control
- Cons: Doesn't account for route-specific needs

#### Option 3: Hierarchical resolution (Chosen)
- Pros: Flexible, sensible defaults, backward compatible
- Cons: Slightly more complex logic

### Rationale
- **Flexibility**: Route steps can override defaults
- **Sensible defaults**: 30s fallback prevents infinite hangs
- **Configuration simplicity**: Most routes can use default timeout
- **Provider independence**: Timeouts not tied to provider definitions

### Follow-up Update (2026-05-07)
- Final naming: `default_route_timeout`, `route_timeout`, `step_timeout` (breaking migration for user clarity).
- Route timeout is now enforced in manager execution logic with `context.WithTimeout`, producing structured `ROUTE_TIMEOUT` errors with partial step summaries.
- Provider requests are context-bound (`http.NewRequestWithContext`) so route timeout cancels in-flight upstream calls.
- HTTP server read/write timeouts remain derived from effective route budgets with a 30s floor and no upper clamp, so large route budgets are not preempted by server timeout.
- Config loading now uses strict known-field decoding (`KnownFields(true)`), so legacy/unknown timeout keys fail fast instead of being silently ignored.
- Timeout values must be strictly positive (`> 0`) to prevent instant-cancel misconfiguration.

## Multimodal Message Support

### Decision
Support both string and array content formats in message structures to be fully compatible with OpenAI's multimodal API.

### Context
- OpenAI API allows message content to be either a string or an array of content blocks
- Array format used for multimodal inputs (text + images)
- Original implementation only supported string content
- Error: `"failed to parse messages: json: cannot unmarshal array into Go struct field"`

### Options Considered

#### Option 1: Reject Array Content
- Pros: Simple, maintains existing code structure
- Cons: Breaks multimodal functionality, not OpenAI-compatible

#### Option 2: Convert Arrays to Strings
- Pros: Maintains string interface
- Cons: Loses structured content information, potential data loss

#### Option 3: Support Both Formats (Chosen)
- Pros: Full OpenAI compatibility, preserves all data
- Cons: More complex type handling

### Rationale
- **API Compatibility**: Must support full OpenAI API specification
- **Future-proofing**: Multimodal is becoming standard for AI APIs
- **Data Preservation**: No loss of structured content information
- **Backward Compatibility**: Existing string-only requests continue to work

### Implementation Details
- `Message.Content` changed from `string` to `json.RawMessage`
- Added helper methods for type detection and extraction
- Validation updated to accept both formats
- Enhanced error logging for debugging parsing issues
- Content truncation logic updated for both string and array formats

## Error Response Strategy

### Decision
Return 404 for unmatched models, 502 for execution failures.

### Context
- Original system always tried providers, returned 502 on all failures
- Route-based system can distinguish between configuration and execution errors
- Clients need clear error semantics

### Options Considered

#### Option 1: Always 502 (like original)
- Pros: Simple, consistent
- Cons: Loses information about route configuration issues

#### Option 2: 404 for no route, 502 for failures (Chosen)
- Pros: Clear error semantics, helps with debugging
- Cons: Different from original behavior

### Rationale
- **HTTP semantics**: 404 correctly indicates "resource not found" (route)
- **Debugging**: Clear distinction between config and runtime issues
- **API design**: Follows REST conventions
- **Client handling**: Allows different retry strategies for different error types

## OTLP Observability

### Decision
Use an environment-driven OTLP exporter so traces and structured logger events flow through whichever collector the operator configures.

### Context
- Instrumentation already touches request handling and logging, so routing both through OTLP maximizes observability.
- The deployment workflow centers on `.env` and `ops.sh` (and CI/sync scripts for Docker), making environment variables the natural control point for OTLP settings.
- Locking to a vendor like Grafana Cloud would duplicate documentation and restrict operators who prefer another collector.

### Options Considered

#### Option 1: Vendor-specific telemetry (e.g., Grafana Cloud)
- Pros: Ready-made instructions and dashboards
- Cons: Tight coupling and duplicate documentation for env vars

#### Option 2: Env-driven OTLP exporter (Chosen)
- Pros: Works with any collector, keeps `config.yaml.example` focused on routes, makes logs+traces share one pipeline
- Cons: Requires manual instrumentation (span wrappers) and Basic auth header handling

#### Option 3: Skip telemetry
- Pros: No new dependencies
- Cons: Loses distributed tracing and log correlation

### Rationale
- **Flexibility**: Operators choose `OTLP_ENDPOINT`, `OTLP_API_KEY`, `OTLP_SERVICE_NAME`, and other env vars to match their collector.
- **Consistency**: Logger still outputs JSON but also emits OTLP events via `telemetry.RecordLog`, giving traces and logs the same timeline.
- **Minimal config impact**: Observability settings stay in `.env`/env vars so `config.yaml.example` remains about providers/routes.

## OTLP/HTTP for Compatibility (2026-02-05)

### Decision
Switch from OTLP/gRPC to OTLP/HTTP for exporting telemetry and structured logs.

### Context
- The initial gRPC implementation failed on some environments (VDS) with `Unavailable` errors and ALPN handshake failures.
- Grafana Cloud and other modern OTLP collectors fully support HTTP/JSON or HTTP/Protobuf.
- Authentication with `glc_` tokens in Grafana Cloud requires a specific `instanceID:apiKey` Basic auth format, which was tricky for users to configure manually.

### Options Considered

#### Option 1: Stick with gRPC and fix TLS/ALPN
- Pros: Slightly more efficient than HTTP.
- Cons: Complex to debug across different network environments and OS versions.

#### Option 2: Switch to HTTP (Chosen)
- Pros: Highly compatible, works through most firewalls/proxies, easier to debug with standard tools.
- Cons: Slightly higher overhead than gRPC (negotiation/headers).

### Rationale
- **Compatibility**: HTTP is a "fire and forget" solution for connectivity issues encountered with gRPC.
- **Ease of Use**: By using HTTP, we could implement a custom `normalizeEndpoint` and authentication logic that handles `glc_` tokens automatically, making the gateway "just work" with Grafana Cloud.
- **Standardization**: Most OTLP collectors (including Grafana Alloy and Grafana Cloud) recommend HTTP as a robust alternative to gRPC.

### Implementation Details
- Used `otlptracehttp` exporter.
- Added logic to parse `glc_` tokens and extract the Org/Stack ID to use as the Basic auth username.
- Implemented intelligent path handling: if the user provides `.../otlp`, the exporter correctly appends `/v1/traces`.
- Added support for `OTEL_` standard environment variables alongside `OTLP_` overrides.

## Logfire as production OTLP backend (2026-05-17)

### Decision
Point VDS `OTLP_ENDPOINT` at Logfire US; authenticate with write token via `OTLP_HEADERS`, not Grafana-style Basic auth alone.

### Rationale
- Logfire accepts standard OTLP/HTTP; ai-gateway already exports via `otlptracehttp`.
- Default exporter sets `Authorization: Basic base64(apiKey:)`, which Logfire rejects — `OTLP_HEADERS=Authorization=<token>` overrides it.
- Keep exporter vendor-agnostic; only `.env` changes per backend.

## Logger context for trace correlation (2026-05-17)

### Decision
`logger.Info` / `logger.Error` require `context.Context`; `RecordLog` only emits OTLP span events when ctx contains a valid span.

### Rationale
- Prior `RecordLog(context.Background(), ...)` created orphan `log.record` spans in Logfire.
- Span events nest under `http.*` / `route/*` / `step/*` when handlers and manager pass the right ctx.

### Pitfall avoided
Do not create fallback spans in `RecordLog` when ctx has no span — skip OTLP for startup logs instead.

## Step span as parent for step work (2026-05-17)

### Decision
Each route step gets span name `step/{model}`; all step logs and `provider.Call` use `stepCtx` from `tracer.Start(routeCtx, ...)`.

### Lifecycle rule
**Never `defer stepSpan.End()` inside the route `for` loop** — defers run at function return, so failed steps stay open across failover. Call `stepSpan.End()` before `continue` and before success `return`.

### Attributes
`step.provider`, `step.model`, `step.index`, `route.name` on the step span; duplicate model names distinguished by `step.index`.

## Error classification for rotation (2026-08-12)

### Decision
`markStepFailed` (endpoint rotation / circuit breaker) is now gated by `isRotatableError(err)` — only real provider *health* faults rotate. `RequestAdapter` signature changed to `func(*types.ChatRequest) (bool, error)` (bool = "did I change Raw") and adapter errors now short-circuit the request immediately.

### Classification rules
- **4xx ≠ 429 → NO rotation.** A 400/422 is a request-shape / permanent fault: the endpoint is healthy, the request is wrong. Rotation is futile and would mask the real cause (and burn a healthy endpoint for the cooldown).
- **429 → rotation.** Rate-limit is a capacity signal, not a permanent fault — the cooldown gives the endpoint room to recover.
- **5xx / network / timeout → rotation.** Genuine health faults.
- **Adapter errors (`ErrRequestAdapter`) → fail the request immediately, NEVER rotate.** The same Raw fails identically on every provider, so trying the rest of the route is wasted latency; and rotation is meaningless for a gateway-side bug. `isRouteCanceled` / `isRouteDeadlineExceeded` guards still run first (client cancel/timeout ≠ step failure).
- **Client-side marshal/create errors (`ErrClientSide`) → NEVER rotate** (added 2026-08-12, review finding). `Client.Call()` wraps pre-network failures (`json.Marshal` failure, `http.NewRequest` failure) with `ErrClientSide` — the provider never saw the request, so rotation would burn a healthy endpoint for a gateway-side fault. `isRotatableError` excludes it via `errors.Is`.

### Rationale
- Found during the groq/opencode investigation: an unshapable request (e.g. `max_tokens` capped low on groq) previously marked a *healthy* endpoint for rotation as if it were down, and an adapter bug would have rotated every endpoint.
- Classification lives in the manager (next to `markStepFailed`), not in `Call()` — the client stays a dumb HTTP+adapter layer; the manager owns rotation state.

### Implementation details
- `HTTPStatusError{StatusCode, Body}` typed error from `Client.Call()` on non-200; `errors.As` in the manager extracts the status.
- `ErrRequestAdapter` sentinel; adapter errors wrapped `%w ErrRequestAdapter: adapter[N] (name) failed: ...`.
- Error log for failed steps now includes `status_code` when the error is an `HTTPStatusError`.
- Test seam: `newStepClient` package var (defaults to `NewClientWithRouteStep`) so tests can inject clients with custom adapters.

## Adapter-fire logging + named adapters (2026-08-12)

### Decision
Adapters now report mutation and are named; every mutation is logged.

- `RequestAdapter func(*types.ChatRequest) (bool, error)` — bool = "did I change Raw". Explicit reporting is required because Go map marshaling is unordered: `bytes.Equal(before, after)` is NOT a reliable mutation detector.
- `namedAdapter{name, fn}` — the name exists only at list-construction (`defaultAdapters` / `adaptersForProvider`); `RequestAdapter` stays a bare function type so direct-call unit tests keep working.
- `Client.Call()` logs INFO `"Request adapter applied"` with `adapter`, `provider`, `model`, `route`, `bytes_before`, `bytes_after` only when an adapter reports a change. No-op adapters are silent.
- `Client` gained a `routeName` field; `NewClientWithRouteStep` gained a `routeName` param (the manager passes `route.Name`). Without it, adapter logs would silently carry `route: ""`.

### Adapter behavior notes
- `adaptConflictResolution` early-returns `(false, nil)` with Raw byte-identical when no deletion is needed (previously it re-marshaled unconditionally — CPU waste + map key reorder).
- `adaptStrictJSONSchema` returns `(true, nil)` only if it actually injected `additionalProperties:false` anywhere (recursive `enforceStrictSchema` now reports changes).
- `adaptToolChoice` became `adaptToolChoiceFor(thinking bool)` factory (see next decision).

## Config-driven thinking-model flag (2026-08-12)

### Decision
`RouteStep.Thinking *bool` (tri-state) replaces reliance on the hardcoded `isThinkingModel` name list for new thinking models.

- `thinking: true` on a step → the tool-choice adapter relaxes forced `tool_choice` unconditionally, whatever the model name.
- `thinking` absent (`nil`) → backward-compatible fallback to `isThinkingModel()` (deepseek-v4-flash, deepseek-reasoner, deepseek-r1).
- `thinking: false` → **explicit opt-out** (added 2026-08-12, review finding): this model is NOT thinking even if its name is in the hardcoded list — an escape hatch when a listed model stops requiring relaxation (or a model name collision). A plain `bool` cannot express this: with `bool`, `false` is indistinguishable from unset and would still hit the fallback list.
- Strict `KnownFields` YAML decoding means the field must exist in the struct or configs fail to load — adding the field IS the validation.
- New thinking models (e.g. `gpt-oss-120b`) need zero code changes: just set `thinking: true` on the step.

## adaptToolChoice object-form support (2026-08-12)

### Decision
The thinking-model branch of `adaptToolChoiceFor` handles both `tool_choice` shapes:

- string `"required"` → `"auto"` (existing behavior)
- object `{"type":"function","function":{"name":…}}` → `"auto"` (new)

Thinking models reject forced choice in any form, so an explicit function name is overridden — consistent with the existing string behavior. `"none"` / `"auto"` untouched; non-thinking untouched. pydantic-ai sends the string form today, so the ai-antispam bot is unaffected.

## Strict-schema coercion restored (2026-08-12)

### Decision
`enforceStrictSchema` coerces `additionalProperties` to `false` **unconditionally** — whenever it is absent **or** not exactly `false` — instead of only injecting when absent.

### Context
A sub-agent review found a regression introduced with the strict-schema adapter: the pre-change code forced `additionalProperties:false` unconditionally, but the new code only injected it when the key was absent. A client sending an explicit `additionalProperties: true` to groq/cerebras would pass through → provider 400 → and with the new 4xx-never-rotates classification, that's a **permanent, visible failure** where the gateway used to auto-correct.

### Rationale
- groq/cerebras strict mode rejects `additionalProperties: true` in the schema — coercing it is the whole point of the adapter.
- `changed` is still reported correctly: it returns `true` only when the walker actually modified the schema.

### Implementation
```go
if ap, exists := schema["additionalProperties"]; !exists || ap != false {
    schema["additionalProperties"] = false
    changed = true
}
```
Also recurses into `properties` and `oneOf`/`anyOf`/`allOf` branches.

## clearStepFailure race fence (2026-08-12)

### Decision
`clearStepFailure` is fenced against the concurrent-success race: a success only clears a cooldown mark that **predates** the successful request's own start.

### Context
Race found in review: request B passes the `stepInCooldown` check, is in-flight while request A fails and marks the step, then B succeeds and erases A's mark — the circuit breaker never engages for the next request. The cooldown map now stores `markedAt` alongside the expiry, and the success path passes its request `start` time.

### Implementation
- `stepCooldown{until, markedAt}` — `markStepFailed` records `now` as `markedAt`.
- `clearStepFailure(routeName, step, requestStart)` deletes only when `cd.markedAt.Before(requestStart)`.
- The manager's success path passes the request's `start` (captured at execution begin).
- Self-healing: the mark survives at most until its natural expiry, so a stale clear only weakens the breaker briefly.

## Rotation log attaches to trace span (2026-08-12)

### Decision
`markStepFailed` logs with the route context (`routeCtx`), not `context.Background()`, so the rotation log appears as a span event on the active trace.

### Context
Review finding: with `context.Background()`, `RecordLog` produced no span event (no valid span in ctx) — the rotation decision was invisible in Logfire traces, violating the project's own "Logger context for trace correlation" decision. The route span is the correct parent: the step span is already ended when `markStepFailed` runs (explicit `stepSpan.End()` before `continue`), so events on it would be dropped.
