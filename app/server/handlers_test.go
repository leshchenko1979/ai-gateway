package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/providers"
	"ai-gateway/types"
)

func mustNewProviderManager(t *testing.T, cfg *config.Config, log *logger.Logger) *providers.Manager {
	t.Helper()
	manager, err := providers.NewManager(cfg, log)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func mustNewServer(t *testing.T, cfg *config.Config, log *logger.Logger, manager *providers.Manager) *Server {
	t.Helper()
	srv, err := NewServer(cfg, log, manager)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return srv
}

func TestHandleHealth(t *testing.T) {
	cfg := &config.Config{APIKey: "test-key", Port: 8080}
	log := logger.NewLogger()
	manager := mustNewProviderManager(t, &config.Config{Providers: []config.Provider{}, Routes: []config.Route{}}, log)
	srv := mustNewServer(t, cfg, log, manager)

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	srv.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var response map[string]string
	json.NewDecoder(rr.Body).Decode(&response)
	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", response["status"])
	}
}

func TestHandleUpstreamModelsCheck(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"x","object":"model"}]}`))
	}))
	defer ts.Close()

	providersList := []config.Provider{
		{Name: "p1", APIKey: "secret", BaseURL: ts.URL + "/v1"},
	}
	cfg := &config.Config{APIKey: "test-key", Port: 8080, Providers: providersList}
	log := logger.NewLogger()
	manager := mustNewProviderManager(t, &config.Config{Providers: providersList, Routes: []config.Route{}}, log)
	srv := mustNewServer(t, cfg, log, manager)

	req := httptest.NewRequest(http.MethodGet, "/v1/diagnostics/upstream-models", nil)
	rr := httptest.NewRecorder()
	srv.handleUpstreamModelsCheck(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var body struct {
		OK        bool `json:"ok"`
		Providers []struct {
			Provider       string `json:"provider"`
			OK             bool   `json:"ok"`
			ModelCount     int    `json:"model_count"`
			ResponseTimeMs int64  `json:"response_time_ms"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || len(body.Providers) != 1 || !body.Providers[0].OK || body.Providers[0].ModelCount != 1 {
		t.Fatalf("%+v", body)
	}
	if body.Providers[0].ResponseTimeMs < 0 {
		t.Fatalf("response_time_ms: %d", body.Providers[0].ResponseTimeMs)
	}
}

func TestHandleModels(t *testing.T) {
	routes := []config.Route{
		{Name: "test-route-1"},
		{Name: "test-route-2"},
	}
	cfg := &config.Config{APIKey: "test-key", Port: 8080, Routes: routes}
	log := logger.NewLogger()
	manager := mustNewProviderManager(t, &config.Config{Providers: []config.Provider{}, Routes: routes}, log)
	srv := mustNewServer(t, cfg, log, manager)

	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("X-Api-Key", "test-key")
	rr := httptest.NewRecorder()
	srv.handleModels(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var response types.ModelsResponse
	json.NewDecoder(rr.Body).Decode(&response)
	if len(response.Data) != 2 {
		t.Errorf("Expected 2 models, got %d", len(response.Data))
	}
	expectedModels := []string{"test-route-1", "test-route-2"}
	for i, model := range response.Data {
		if model.ID != expectedModels[i] {
			t.Errorf("Expected model ID '%s', got '%s'", expectedModels[i], model.ID)
		}
	}
}

func TestHandleChatCompletions_AllStepsFail(t *testing.T) {
	// Create mock server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	providersList := []config.Provider{
		{Name: "provider1", APIKey: "key1", BaseURL: server.URL},
	}
	routes := []config.Route{
		{
			Name: "test-model",
			Steps: []config.RouteStep{
				{Provider: "provider1", Model: "gpt-4"},
			},
		},
	}

	cfg := &config.Config{APIKey: "test-key", Port: 8080}
	log := logger.NewLogger()
	manager := mustNewProviderManager(t, &config.Config{Providers: providersList, Routes: routes}, log)
	srv := mustNewServer(t, cfg, log, manager)

	requestBody := `{"model":"test-model","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "test-key")
	rr := httptest.NewRecorder()

	srv.handleChatCompletions(rr, req)

	// Should return 502 Bad Gateway
	if rr.Code != http.StatusBadGateway {
		t.Errorf("Expected status 502, got %d", rr.Code)
	}

	// Should return unified JSON error response
	var errorResp types.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errorResp); err != nil {
		t.Errorf("Expected JSON ErrorResponse, got error: %v", err)
		return
	}

	// Verify error response structure
	if errorResp.Error.Type != "execution_error" {
		t.Errorf("Expected error type 'execution_error', got '%s'", errorResp.Error.Type)
	}
	if errorResp.Error.Code != "ROUTE_EXECUTION_FAILED" {
		t.Errorf("Expected error code 'ROUTE_EXECUTION_FAILED', got '%s'", errorResp.Error.Code)
	}
	if errorResp.Error.Message != "All route steps failed" {
		t.Errorf("Expected error message 'All route steps failed', got '%s'", errorResp.Error.Message)
	}

	// Verify details contain route error information (JSON unmarshals to map)
	details, ok := errorResp.Error.Details.(map[string]interface{})
	if !ok {
		t.Errorf("Expected details to be a map, got %T", errorResp.Error.Details)
		return
	}

	// Verify route information
	routeInfo, ok := details["route"].(map[string]interface{})
	if !ok {
		t.Error("Expected route information in details")
		return
	}

	if routeInfo["Name"] != "test-model" {
		t.Errorf("Expected route name 'test-model', got '%s'", routeInfo["Name"])
	}

	// Verify errors array
	errors, ok := details["errors"].([]interface{})
	if !ok {
		t.Error("Expected errors array in details")
		return
	}

	if len(errors) != 1 {
		t.Errorf("Expected 1 step error, got %d", len(errors))
		return
	}

	// Verify step error details
	stepErr, ok := errors[0].(map[string]interface{})
	if !ok {
		t.Error("Expected step error to be a map")
		return
	}

	if stepErr["step_index"].(float64) != 0 {
		t.Errorf("Expected step index 0, got %v", stepErr["step_index"])
	}
	if stepErr["provider"] != "provider1" {
		t.Errorf("Expected provider 'provider1', got '%s'", stepErr["provider"])
	}
	if stepErr["model"] != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", stepErr["model"])
	}
	if stepErr["error"] == "" {
		t.Error("Expected non-empty error message")
	}
}

func TestHandleChatCompletions_RouteTimeout(t *testing.T) {
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer slowServer.Close()

	providersList := []config.Provider{
		{Name: "provider1", APIKey: "key1", BaseURL: slowServer.URL},
	}
	routes := []config.Route{
		{
			Name:         "timeout-model",
			RouteTimeout: "50ms",
			Steps: []config.RouteStep{
				{Provider: "provider1", Model: "gpt-4", StepTimeout: "2s"},
			},
		},
	}

	cfg := &config.Config{
		APIKey:    "test-key",
		Port:      8080,
		Providers: providersList,
		Routes:    routes,
	}
	log := logger.NewLogger()
	manager := mustNewProviderManager(t, cfg, log)
	srv := mustNewServer(t, cfg, log, manager)

	requestBody := `{"model":"timeout-model","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "test-key")
	rr := httptest.NewRecorder()

	srv.handleChatCompletions(rr, req)

	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("Expected status 504, got %d body=%s", rr.Code, rr.Body.String())
	}

	var errorResp types.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Expected JSON ErrorResponse, got error: %v", err)
	}
	if errorResp.Error.Code != "ROUTE_TIMEOUT" {
		t.Fatalf("Expected error code ROUTE_TIMEOUT, got %s", errorResp.Error.Code)
	}
}

func TestHandleChatCompletions_RouteNotFound(t *testing.T) {
	cfg := &config.Config{
		APIKey: "test-key",
		Port:   8080,
		Providers: []config.Provider{
			{Name: "provider1", APIKey: "key1", BaseURL: "http://example.com"},
		},
		Routes: []config.Route{
			{
				Name: "configured-model",
				Steps: []config.RouteStep{
					{Provider: "provider1", Model: "gpt-4"},
				},
			},
		},
	}
	log := logger.NewLogger()
	manager := mustNewProviderManager(t, cfg, log)
	srv := mustNewServer(t, cfg, log, manager)

	requestBody := `{"model":"missing-model","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "test-key")
	rr := httptest.NewRecorder()

	srv.handleChatCompletions(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	var errorResp types.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Expected JSON ErrorResponse, got error: %v", err)
	}
	if errorResp.Error.Code != "ROUTE_NOT_FOUND" {
		t.Fatalf("Expected error code ROUTE_NOT_FOUND, got %s", errorResp.Error.Code)
	}
}

func TestHandleChatCompletions_ClientCanceled(t *testing.T) {
	cfg := &config.Config{
		APIKey: "test-key",
		Port:   8080,
		Providers: []config.Provider{
			{Name: "provider1", APIKey: "key1", BaseURL: "http://example.com"},
		},
		Routes: []config.Route{
			{
				Name: "cancel-model",
				Steps: []config.RouteStep{
					{Provider: "provider1", Model: "gpt-4"},
				},
			},
		},
	}
	log := logger.NewLogger()
	manager := mustNewProviderManager(t, cfg, log)
	srv := mustNewServer(t, cfg, log, manager)

	requestBody := `{"model":"cancel-model","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "test-key")
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.handleChatCompletions(rr, req)

	if rr.Code != http.StatusRequestTimeout {
		t.Fatalf("Expected status 408, got %d body=%s", rr.Code, rr.Body.String())
	}
	var errorResp types.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Expected JSON ErrorResponse, got error: %v", err)
	}
	if errorResp.Error.Code != "CLIENT_CANCELED" {
		t.Fatalf("Expected error code CLIENT_CANCELED, got %s", errorResp.Error.Code)
	}
}
