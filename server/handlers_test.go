package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/providers"
	"ai-gateway/types"
)

func TestHandleHealth(t *testing.T) {
	cfg := &config.Config{APIKey: "test-key", Port: 8080}
	logger := logger.NewLogger()
	manager := providers.NewManager([]config.Provider{}, []config.Route{}, logger)
	srv := NewServer(cfg, logger, manager)

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

func TestHandleModels(t *testing.T) {
	routes := []config.Route{
		{Name: "test-route-1"},
		{Name: "test-route-2"},
	}
	cfg := &config.Config{APIKey: "test-key", Port: 8080, Routes: routes}
	logger := logger.NewLogger()
	manager := providers.NewManager([]config.Provider{}, routes, logger)
	srv := NewServer(cfg, logger, manager)

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
	logger := logger.NewLogger()
	manager := providers.NewManager(providersList, routes, logger)
	srv := NewServer(cfg, logger, manager)

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

	// Should return JSON with route error details
	var routeErr types.RouteError
	if err := json.NewDecoder(rr.Body).Decode(&routeErr); err != nil {
		t.Errorf("Expected JSON RouteError response, got error: %v", err)
		return
	}

	// Verify route information
	if routeErr.Route.Name != "test-model" {
		t.Errorf("Expected route name 'test-model', got '%s'", routeErr.Route.Name)
	}

	// Verify we have one error for the single step
	if len(routeErr.Errors) != 1 {
		t.Errorf("Expected 1 step error, got %d", len(routeErr.Errors))
		return
	}

	// Verify step error details
	stepErr := routeErr.Errors[0]
	if stepErr.StepIndex != 0 {
		t.Errorf("Expected step index 0, got %d", stepErr.StepIndex)
	}
	if stepErr.Provider != "provider1" {
		t.Errorf("Expected provider 'provider1', got '%s'", stepErr.Provider)
	}
	if stepErr.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", stepErr.Model)
	}
	if stepErr.Error == "" {
		t.Error("Expected non-empty error message")
	}
}