# Request Adapters & Endpoint Rotation

This document describes what the gateway does to your request **before** it
hits an upstream provider, and how it recovers when a provider fails. If you
only configure routes, you should still read this — some providers reject
requests unless the gateway reshapes them, and endpoint rotation decides when
a failing step is skipped.

Two mechanisms, two layers:

- **Adapters** shape the request (per-provider, decided by config flags).
- **Rotation** reshapes the *provider list* (per-route-step, decided by errors).

---

## 1. Adapters — request shaping

Every chat request runs through a small pipeline of **request adapters** before
it is sent to a provider. An adapter is a pure function:

```go
type RequestAdapter func(request *types.ChatRequest) (bool, error)
```

It may mutate `request.Raw` (the raw JSON body) and reports whether it changed
anything. The gateway logs every adapter that fires:

```
Request adapter applied  adapter=tool-choice provider=opencode model=deepseek-v4-flash route=ai-antispam bytes_before=684 bytes_after=634
```

Adapter order is **fixed and semantic**, not cosmetic:

1. `conflict-resolution` — always
2. `tool-choice` — always (thinking-model logic below)
3. `strict-json-schema` — **only** when the provider sets `strict_json_schema: true`

If an adapter errors, the request **fails immediately** (no fallback to the
next step, no rotation mark) — the same malformed body would fail identically
on every provider, so trying the rest is wasted latency.

### 1.1 `conflict-resolution`

Some providers reject requests that carry **both** `tools` and
`response_format`. When both are present, the adapter **deletes
`response_format`** and keeps `tools` (the structured-output mechanism most
SDKs actually use).

- Fires when: `tools` AND `response_format` both present.
- Effect: `response_format` removed, tools kept.
- No-op otherwise (raw body untouched — no re-marshal, no key reorder).

### 1.2 `tool-choice` (thinking models)

**Thinking/reasoning models reject a forced `tool_choice`** ("Thinking mode does
not support this tool_choice"). The adapter relaxes forced choice to `auto`
for thinking models. Handles **both** wire shapes:

| Wire shape | Example | Result for thinking model |
|---|---|---|
| string | `"tool_choice": "required"` | → `"auto"` |
| object | `"tool_choice": {"type":"function","function":{"name":"final_result"}}` | → `"auto"` (explicit function name overridden) |
| anything else | `"none"`, `"auto"` | untouched |

Whether a model counts as "thinking" is the **tri-state `thinking`** flag on the
route step (see §3.2):

| `thinking` | Behavior |
|---|---|
| **unset** (`nil`) | fall back to the built-in hardcoded thinking-model list (`deepseek-v4-flash`, `deepseek/deepseek-v4-flash`, `deepseek-reasoner`, `deepseek/deepseek-r1`) |
| `true` | relax `tool_choice` **unconditionally** — this model is thinking |
| `false` | **explicit opt-out** — never relax, even if the model name is in the hardcoded list |

### 1.3 `strict-json-schema`

**groq and cerebras require `additionalProperties: false`** in every JSON
schema object node; opencode rejects it. So this adapter is **provider-scoped**
(via `strict_json_schema: true` on the provider), never global.

Recursively walks `response_format.json_schema.schema` (including nested
`properties`, and `oneOf`/`anyOf`/`allOf` branches) and sets
`additionalProperties: false` on every object node — whether the field is
absent **or** set to anything but exactly `false`. This makes strict-mode
providers accept schemas that OpenAI-style clients produce (e.g. pydantic-ai's
`final_result` tools shape).

- Fires when: provider has `strict_json_schema: true` AND the request carries
  `response_format.json_schema`.
- No-op otherwise (tools-only requests already carry `additionalProperties:false`
  from the SDK — no re-marshal needed).

---

## 2. Endpoint rotation — provider failover

Routes try steps in order until one succeeds. When a step **fails**, the
gateway may mark it for rotation: the step is **skipped on subsequent
requests** until its cooldown expires.

### 2.1 Cooldown

| Config key | Scope | Default |
|---|---|---|
| `step_cooldown` | per-route override | — |
| `default_step_cooldown` | global | `60s` |

Precedence: route `step_cooldown` → `default_step_cooldown` → `60s`.

Keyed by `routeName/provider/model`, so the same provider on two routes cools
down independently.

### 2.2 Lifecycle

1. **Failure** → `markStepFailed` records the cooldown expiry + a timestamp,
   and logs (as a **span event** on the trace, not a separate log):
   `Step marked for rotation (will be skipped)`.
2. **Skip** → while cooling down, the step is skipped on the next request
   (next healthy step is tried first).
3. **Success** → `clearStepFailure` removes the mark — **but only if the mark
   predates the successful request's start**. A mark set by a concurrent
   in-flight failure (after this request began) is *newer* evidence; clearing
   it would let a just-failed endpoint back in immediately.

### 2.3 What counts as a rotatable failure

This is the crux — **not every error is a provider health fault**:

| Error | Rotates? | Rationale |
|---|---|---|
| HTTP **4xx** (400, 401, 403, 404, 422…) | ❌ **never** | request-shape / permanent fault — rotation is futile and would mask the real cause |
| HTTP **429** Too Many Requests | ✅ yes | capacity signal — cooldown gives the endpoint room to recover |
| HTTP **5xx** (500, 502, 503…) | ✅ yes | provider-side health fault |
| Network error / timeout / context deadline | ✅ yes | endpoint unreachable or too slow |
| `ErrRequestAdapter` (adapter bug) | ❌ **never** | gateway-side, provider-independent — rotation is meaningless |
| `ErrClientSide` (marshal / request-creation failure) | ❌ **never** | gateway-side pre-network fault — the provider never saw the request |

The `status_code` is logged on every step failure, so you can see in logfire
whether a rotated endpoint went down with a 503 or just got a 400.

---

## 3. Config reference

### 3.1 Provider: `strict_json_schema`

```yaml
providers:
  - name: groq
    api_key: ${GROQ_API_KEY}
    base_url: https://api.groq.com/openai/v1/
    strict_json_schema: true   # groq/cerebras need additionalProperties:false
```

### 3.2 Route step: `thinking` (tri-state)

```yaml
routes:
  - name: antispam
    steps:
      - provider: opencode
        model: deepseek-v4-flash
        thinking: true    # true=relax tool_choice; false=opt-out; unset=hardcoded list
```

### 3.3 Route: `step_cooldown`

```yaml
routes:
  - name: antispam
    step_cooldown: 2m      # per-route rotation cooldown (default 60s)
    steps:
      - provider: groq
        model: openai/gpt-oss-120b
```

---

## 4. Worked example

```yaml
routes:
  - name: antispam
    step_cooldown: 1m
    steps:
      - provider: groq
        model: openai/gpt-oss-120b
        step_timeout: 15s
      - provider: opencode
        model: deepseek-v4-flash
        step_timeout: 30s
      - provider: openrouter
        model: meta-llama/llama-3.3-70b-instruct:free
```

What happens on each request:

1. **groq (step 1, `strict_json_schema: true`)** — if the client sent
   `response_format`, the strict-schema adapter injects
   `additionalProperties:false`; groq answers in ~0.5s. Done.
2. **groq 400s** (client sent an unshapable request) → **no rotation** — the
   400 is the client's fault; next request tries groq again. `isRotatableError`
   returns false.
3. **groq 503s** (provider down) → `markStepFailed` → groq is skipped for 60s;
   next request goes straight to **opencode (step 2)**. The tool-choice adapter
   relaxes `tool_choice: "required"` → `"auto"` (deepseek is a thinking model,
   either via the hardcoded list or `thinking: true`).
4. **opencode also fails** → openrouter (step 3) answers; opencode joins the
   cooldown. Both recover independently.
5. **A success on openrouter** clears *its own* mark only — groq's mark
   survives until its cooldown expires (fenced clear).

Result: the fast path stays groq (~0.5s), rotation absorbs outages without
burning cooldown on client errors, and thinking models never see a forced
`tool_choice`.
