package providers

import (
	"encoding/json"
	"fmt"
	"time"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/types"
)

// Manager handles route-based execution of providers
type Manager struct {
	providers map[string]config.Provider // provider name -> provider config
	routes    []config.Route
	logger    *logger.Logger
}

// NewManager creates a new provider manager
func NewManager(providers []config.Provider, routes []config.Route, logger *logger.Logger) *Manager {
	// Build provider map for quick lookup
	providerMap := make(map[string]config.Provider)
	for _, provider := range providers {
		providerMap[provider.Name] = provider
	}

	return &Manager{
		providers: providerMap,
		routes:    routes,
		logger:    logger,
	}
}

// GetRoute finds a route by exact model name match
func (m *Manager) GetRoute(model string) (*config.Route, error) {
	for _, route := range m.routes {
		if route.Name == model {
			return &route, nil
		}
	}
	return nil, fmt.Errorf("no route found for model '%s'", model)
}

// Execute runs the request through the route for the model until one succeeds
func (m *Manager) Execute(request types.ChatRequest) (*types.ChatResponse, error) {
	return m.ExecuteWithTracing(request, "")
}

// ExecuteWithTracing runs the request through the route for the model until one succeeds with request tracing
func (m *Manager) ExecuteWithTracing(request types.ChatRequest, requestID string) (*types.ChatResponse, error) {
	// Find the route for this model
	route, err := m.GetRoute(request.Model)
	if err != nil {
		return nil, fmt.Errorf("route lookup failed: %w", err)
	}

	var stepErrors []types.RouteStepError

	// Try each step in the route
	for stepIndex, step := range route.Steps {
		// Get provider config
		providerCfg, exists := m.providers[step.Provider]
		if !exists {
			return nil, fmt.Errorf("route '%s' step %d: provider '%s' not found", route.Name, stepIndex, step.Provider)
		}

		fields := map[string]interface{}{
			"provider": step.Provider,
			"model":    step.Model,
			"route":    route.Name,
		}
		if requestID != "" {
			fields["request_id"] = requestID
		}

		m.logger.Info("Trying route step", fields)

		start := time.Now()
		// Create provider client on-demand with route step configuration
		provider := NewClientWithRouteStep(providerCfg, step, m.logger)
		response, err := provider.Call(request)
		duration := time.Since(start)

		if err != nil {
			errorFields := map[string]interface{}{
				"provider":     step.Provider,
				"model":        step.Model,
				"route":        route.Name,
				"step":         stepIndex,
				"duration_ms":  duration.Milliseconds(),
			}
			if requestID != "" {
				errorFields["request_id"] = requestID
			}

			m.logger.Error("Route step failed", err, errorFields)

			// Collect error from this step
			stepErrors = append(stepErrors, types.RouteStepError{
				StepIndex: stepIndex,
				Provider:  step.Provider,
				Model:     step.Model,
				Error:     err.Error(),
			})
			continue
		}

		// Convert response to JSON for logging (with truncated message contents)
		truncatedResp := response.TruncateResponseForLogging()
		responseJSON, _ := json.Marshal(truncatedResp)

		successFields := map[string]interface{}{
			"provider":      step.Provider,
			"model":         step.Model,
			"route":         route.Name,
			"step":          stepIndex,
			"response_json": string(responseJSON),
			"duration_ms":   duration.Milliseconds(),
		}
		if requestID != "" {
			successFields["request_id"] = requestID
		}

		m.logger.Info("Route step succeeded", successFields)
		return response, nil
	}

	// All route steps failed
	routeError := types.RouteError{
		Route:  *route,
		Errors: stepErrors,
	}
	return nil, routeError
}