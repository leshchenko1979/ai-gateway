# Recent Changes

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
- Monitor for any edge cases in production deployment
- Consider adding route pattern matching (prefix/wildcard) if needed
- Evaluate adding route metrics and monitoring

## Blockers
- None identified - refactoring completed successfully
- All tests passing
- Configuration migration completed

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