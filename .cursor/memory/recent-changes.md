# Recent Changes

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