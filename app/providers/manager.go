package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
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

	// stepCooldowns tracks provider:model failures for endpoint rotation.
	// A step that failed recently is skipped on subsequent requests until the
	// cooldown expires, so a flaky endpoint stops burning the route budget.
	// The value records the expiry AND when the failure was marked, so
	// clearStepFailure can fence against the concurrent-success race.
	mu            sync.Mutex
	stepCooldowns map[string]stepCooldown // "route/provider/model" -> cooldown
}

// stepCooldown records a rotation mark: when the step may be tried again
// (until) and when the failure was recorded (markedAt). markedAt enables
// timestamp-fenced clearing — a success only clears a mark that predates the
// successful request's start.
type stepCooldown struct {
	until    time.Time
	markedAt time.Time
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
		providers:     providerMap,
		routes:        cfg.Routes,
		config:        cfg,
		logger:        logger,
		tracer:        telemetry.Tracer("ai-gateway.providers"),
		stepCooldowns: make(map[string]stepCooldown),
	}, nil
}

// cooldownKey builds a unique key for a route step.
func cooldownKey(routeName string, step config.RouteStep) string {
	return routeName + "/" + step.Provider + "/" + step.Model
}

// stepInCooldown reports whether the step is currently cooling down.
func (m *Manager) stepInCooldown(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cd, ok := m.stepCooldowns[key]
	return ok && time.Now().Before(cd.until)
}

// markStepFailed records a step failure and sets its cooldown expiry. The ctx
// is the step/route context so the rotation log attaches to the active trace
// span (decisions.md: log events must reach the exporter as span events).
func (m *Manager) markStepFailed(ctx context.Context, route config.Route, step config.RouteStep) {
	cooldown := m.config.GetStepCooldown(route)
	key := cooldownKey(route.Name, step)
	now := time.Now()
	m.mu.Lock()
	m.stepCooldowns[key] = stepCooldown{until: now.Add(cooldown), markedAt: now}
	m.mu.Unlock()
	m.logger.Info(ctx, "Step marked for rotation (will be skipped)",
		map[string]interface{}{
			"route":       route.Name,
			"provider":    step.Provider,
			"model":       step.Model,
			"cooldown":    cooldown.String(),
			"cooldownKey": key,
		})
}

// clearStepFailure removes a step's cooldown mark after a successful call —
// but only if the mark predates the successful request's start. A mark set by
// a concurrent in-flight failure (after this request began) is NEWER evidence
// than this success; clearing it would let a just-failed endpoint back into
// rotation immediately. requestStart fences exactly that.
func (m *Manager) clearStepFailure(routeName string, step config.RouteStep, requestStart time.Time) {
	key := cooldownKey(routeName, step)
	m.mu.Lock()
	cd, ok := m.stepCooldowns[key]
	if ok && cd.markedAt.Before(requestStart) {
		delete(m.stepCooldowns, key)
	}
	m.mu.Unlock()
}

// newStepClient constructs the provider client for a route step. Package-level
// var so tests can inject clients with custom adapters (e.g. a failing adapter
// to verify the adapter-error short-circuit path).
var newStepClient = NewClientWithRouteStep

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

		// Endpoint rotation: skip steps still in cooldown after a recent failure.
		key := cooldownKey(route.Name, step)
		if m.stepInCooldown(key) {
			skipFields := map[string]interface{}{
				"provider": step.Provider,
				"model":    step.Model,
				"route":    route.Name,
				"step":     stepIndex,
			}
			if requestID != "" {
				skipFields["request_id"] = requestID
			}
			m.logger.Info(routeCtx, "Skipping route step (in cooldown)", skipFields)
			stepResults = append(stepResults, types.RouteStepResult{
				StepIndex:  stepIndex,
				Provider:   step.Provider,
				Model:      step.Model,
				Success:    false,
				DurationMs: 0,
				Error:      "skipped (in cooldown)",
			})
			continue
		}

		stepCtx, stepSpan := m.tracer.Start(routeCtx, fmt.Sprintf("step/%s", step.Model),
			trace.WithAttributes(
				attribute.String("step.provider", step.Provider),
				attribute.String("step.model", step.Model),
				attribute.Int("step.index", stepIndex),
				attribute.String("route.name", route.Name),
			),
			trace.WithSpanKind(trace.SpanKindClient),
		)

		fields := map[string]interface{}{
			"provider": step.Provider,
			"model":    step.Model,
			"route":    route.Name,
		}
		if requestID != "" {
			fields["request_id"] = requestID
		}

		m.logger.Info(stepCtx, "Trying route step", fields)

		start := time.Now()
		provider := newStepClient(providerCfg, step, m.config.DefaultStepTimeout, m.logger, route.Name)
		response, err := provider.Call(stepCtx, request)
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
			var httpErr *HTTPStatusError
			if errors.As(err, &httpErr) {
				errorFields["status_code"] = httpErr.StatusCode
			}

			m.logger.Error(stepCtx, "Route step failed", err, errorFields)
			stepSpan.RecordError(err)
			stepSpan.SetStatus(codes.Error, err.Error())
			routeSpan.RecordError(err)
			routeSpan.AddEvent("step.failed", trace.WithAttributes(
				attribute.String("step.error", err.Error()),
				attribute.String("step.provider", step.Provider),
				attribute.String("step.model", step.Model),
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
			// Adapter failures are gateway-side bugs or unshapable requests —
			// the same Raw fails identically on every provider, so trying the
			// rest is wasted latency. Fail immediately, never mark rotation.
			if errors.Is(err, ErrRequestAdapter) {
				return nil, err
			}
			// Endpoint rotation: mark only real provider *health* faults
			// (5xx, 429, network, timeout). Never 4xx shape faults (400/422 —
			// permanent or client-fixable, rotation is futile and would mask
			// the real cause), never adapter bugs (provider-independent).
			if isRotatableError(err) {
				// routeCtx, not stepCtx: the step span is already ended at this
				// point (stepSpan.End() ran above), so its events are dropped.
				// The route span stays active until the function returns —
				// the rotation log attaches to it as a span event.
				m.markStepFailed(routeCtx, *route, step)
			}
			continue
		}

		truncatedResp := response.TruncateResponseForLogging()
		responseJSON, _ := json.Marshal(truncatedResp)
		responseJSONForLog := types.TruncateJSONForLogging(responseJSON)

		successFields := map[string]interface{}{
			"provider":      step.Provider,
			"model":         step.Model,
			"route":         route.Name,
			"step":          stepIndex,
			"response_json": responseJSONForLog,
			"duration_ms":   duration.Milliseconds(),
		}
		if requestID != "" {
			successFields["request_id"] = requestID
		}

		m.logger.Info(stepCtx, "Route step succeeded", successFields)
		stepSpan.SetAttributes(attribute.String("step.response", responseJSONForLog))
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

		// Endpoint rotation: a successful call clears the step's failure mark.
		m.clearStepFailure(route.Name, step, start)

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

// isRotatableError reports whether a step failure is a provider *health* fault.
// 4xx (except 429) are request-shape / permanent faults → do NOT rotate.
// 429, 5xx, network errors, timeouts = health fault → rotate.
// Adapter errors are gateway bugs, provider-independent → rotation is
// meaningless, never mark.
func isRotatableError(err error) bool {
	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 && httpErr.StatusCode != http.StatusTooManyRequests {
			return false
		}
		return true
	}
	if errors.Is(err, ErrRequestAdapter) {
		return false // gateway bug, provider-independent — rotation is meaningless
	}
	if errors.Is(err, ErrClientSide) {
		return false // gateway-side pre-network fault — provider never saw the request
	}
	return true // network / timeout / unknown
}
