package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/telemetry"
	"ai-gateway/types"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var ErrRouteNotFound = errors.New("route not found")

// Manager handles route-based execution of providers
type Manager struct {
	providers map[string]config.Provider // provider name -> provider config
	routes    []config.Route
	config    *config.Config
	logger    *logger.Logger
	tracer    trace.Tracer
}

// NewManager creates a new provider manager.
func NewManager(cfg *config.Config, logger *logger.Logger) (*Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is nil")
	}

	if err := config.ValidateTimeoutSettings(cfg); err != nil {
		return nil, fmt.Errorf("invalid timeout settings: %w", err)
	}

	// Build provider map for quick lookup
	providerMap := make(map[string]config.Provider)
	for _, provider := range cfg.Providers {
		providerMap[provider.Name] = provider
	}

	return &Manager{
		providers: providerMap,
		routes:    cfg.Routes,
		config:    cfg,
		logger:    logger,
		tracer:    telemetry.Tracer("ai-gateway.providers"),
	}, nil
}

// GetRoute finds a route by exact model name match
func (m *Manager) GetRoute(model string) (*config.Route, error) {
	for _, route := range m.routes {
		if route.Name == model {
			return &route, nil
		}
	}
	return nil, fmt.Errorf("%w for model '%s'", ErrRouteNotFound, model)
}

// Execute runs the request through the route for the model until one succeeds
func (m *Manager) Execute(request types.ChatRequest) (*types.ChatResponse, error) {
	return m.ExecuteWithTracing(context.Background(), request, "")
}

// ExecuteWithTracing runs the request through the route for the model until one succeeds with request tracing
func (m *Manager) ExecuteWithTracing(ctx context.Context, request types.ChatRequest, requestID string) (*types.ChatResponse, error) {
	// Find the route for this model
	route, err := m.GetRoute(request.Model)
	if err != nil {
		return nil, fmt.Errorf("route lookup failed: %w", err)
	}

	rootCtx, routeSpan := m.tracer.Start(ctx, fmt.Sprintf("route/%s", route.Name),
		trace.WithAttributes(
			attribute.String("route.name", route.Name),
			attribute.String("route.model", request.Model),
		),
	)
	if requestID != "" {
		routeSpan.SetAttributes(attribute.String("request.id", requestID))
	}
	defer routeSpan.End()

	routeTimeout := m.config.GetRouteTimeout(*route)
	routeStart := time.Now()
	routeCtx, cancel := context.WithTimeout(rootCtx, routeTimeout)
	defer cancel()

	var stepErrors []types.RouteStepError
	var stepResults []types.RouteStepResult

	// Try each step in the route
	for stepIndex, step := range route.Steps {
		if routeErr := routeCtx.Err(); routeErr != nil {
			if isRouteDeadlineExceeded(routeCtx) {
				return nil, m.newRouteTimeoutError(*route, routeTimeout, time.Since(routeStart), stepErrors, stepResults)
			}
			return nil, routeErr
		}

		// Get provider config
		providerCfg, exists := m.providers[step.Provider]
		if !exists {
			err := fmt.Errorf("route '%s' step %d: provider '%s' not found", route.Name, stepIndex, step.Provider)
			routeSpan.RecordError(err)
			routeSpan.SetStatus(codes.Error, err.Error())
			return nil, err
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

		_, stepSpan := m.tracer.Start(routeCtx, fmt.Sprintf("route.%s.step.%d", route.Name, stepIndex),
			trace.WithAttributes(
				attribute.String("step.provider", step.Provider),
				attribute.String("step.model", step.Model),
				attribute.Int("step.index", stepIndex),
			),
			trace.WithSpanKind(trace.SpanKindClient),
		)

		start := time.Now()
		// Create provider client on-demand with route step configuration
		provider := NewClientWithRouteStep(providerCfg, step, m.config.DefaultStepTimeout, m.logger)
		response, err := provider.Call(routeCtx, request)
		duration := time.Since(start)

		stepSpan.SetAttributes(attribute.Int64("step.duration_ms", duration.Milliseconds()))

		if err != nil {
			errorFields := map[string]interface{}{
				"provider":    step.Provider,
				"model":       step.Model,
				"route":       route.Name,
				"step":        stepIndex,
				"duration_ms": duration.Milliseconds(),
			}
			if requestID != "" {
				errorFields["request_id"] = requestID
			}

			m.logger.Error("Route step failed", err, errorFields)
			stepSpan.RecordError(err)
			stepSpan.SetStatus(codes.Error, err.Error())
			routeSpan.RecordError(err)
			routeSpan.AddEvent("step.failed", trace.WithAttributes(
				attribute.String("step.error", err.Error()),
				attribute.String("step.provider", step.Provider),
			))
			stepErrors = append(stepErrors, types.RouteStepError{
				StepIndex: stepIndex,
				Provider:  step.Provider,
				Model:     step.Model,
				Error:     err.Error(),
			})
			stepResults = append(stepResults, types.RouteStepResult{
				StepIndex:  stepIndex,
				Provider:   step.Provider,
				Model:      step.Model,
				Success:    false,
				DurationMs: duration.Milliseconds(),
				Error:      err.Error(),
			})
			stepSpan.End()
			if isRouteDeadlineExceeded(routeCtx) {
				return nil, m.newRouteTimeoutError(*route, routeTimeout, time.Since(routeStart), stepErrors, stepResults)
			}
			if isRouteCanceled(err, routeCtx) {
				return nil, context.Canceled
			}
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
		stepSpan.SetAttributes(attribute.String("step.response", string(responseJSON)))
		stepSpan.SetStatus(codes.Ok, "success")
		stepSpan.End()

		// Record successful step
		stepResults = append(stepResults, types.RouteStepResult{
			StepIndex:  stepIndex,
			Provider:   step.Provider,
			Model:      step.Model,
			Success:    true,
			DurationMs: duration.Milliseconds(),
		})

		// Attach routing summary to response
		response.RoutingSummary = &types.RoutingSummary{
			RouteName: route.Name,
			Steps:     stepResults,
		}
		return response, nil
	}

	// All route steps failed
	routeSpan.SetStatus(codes.Error, "all steps failed")
	routeSpan.AddEvent("route.failed", trace.WithAttributes(attribute.Int("route.step.failures", len(route.Steps))))

	// Build routing summary from stepResults
	routingSummary := &types.RoutingSummary{
		RouteName: route.Name,
		Steps:     stepResults,
	}

	routeError := types.RouteError{
		Route:          *route,
		Errors:         stepErrors,
		RoutingSummary: routingSummary,
	}
	return nil, routeError
}

func (m *Manager) newRouteTimeoutError(route config.Route, routeTimeout, elapsed time.Duration, stepErrors []types.RouteStepError, stepResults []types.RouteStepResult) types.RouteTimeoutError {
	routingSummary := &types.RoutingSummary{
		RouteName: route.Name,
		Steps:     stepResults,
	}
	return types.RouteTimeoutError{
		Route:          route,
		TimeoutMs:      routeTimeout.Milliseconds(),
		ElapsedMs:      elapsed.Milliseconds(),
		Errors:         stepErrors,
		RoutingSummary: routingSummary,
	}
}

func isRouteDeadlineExceeded(routeCtx context.Context) bool {
	return errors.Is(routeCtx.Err(), context.DeadlineExceeded)
}

func isRouteCanceled(err error, routeCtx context.Context) bool {
	return errors.Is(err, context.Canceled) || errors.Is(routeCtx.Err(), context.Canceled)
}
