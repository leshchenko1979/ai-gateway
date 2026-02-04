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
Implement hierarchical timeout resolution: step timeout → default timeout → 30s fallback.

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