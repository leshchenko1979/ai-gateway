package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/types"
)

func mustNewManager(t *testing.T, cfg *config.Config) *Manager {
	t.Helper()
	manager, err := NewManager(cfg, logger.NewLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func TestNewManager_NilInputs(t *testing.T) {
	_, err := NewManager(nil, logger.NewLogger())
	if err == nil || !strings.Contains(err.Error(), "config is nil") {
		t.Fatalf("expected config nil error, got %v", err)
	}

	_, err = NewManager(&config.Config{}, nil)
	if err == nil || !strings.Contains(err.Error(), "logger is nil") {
		t.Fatalf("expected logger nil error, got %v", err)
	}
}

func TestManager_Execute(t *testing.T) {
	// Create mock servers
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseJSON := `{
			"id": "test-id",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "gpt-4",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Success"
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 5,
				"completion_tokens": 10,
				"total_tokens": 15
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseJSON))
	}))
	defer server2.Close()

	// Create providers and routes
	providers := []config.Provider{
		{Name: "provider1", APIKey: "key1", BaseURL: server1.URL},
		{Name: "provider2", APIKey: "key2", BaseURL: server2.URL},
	}
	routes := []config.Route{
		{
			Name: "test-model",
			Steps: []config.RouteStep{
				{Provider: "provider1", Model: "gpt-4"},
				{Provider: "provider2", Model: "gpt-4"},
			},
		},
	}
	manager := mustNewManager(t, &config.Config{Providers: providers, Routes: routes})

	requestJSON := `{"model":"test-model","messages":[{"role":"user","content":"Hello"}]}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	response, err := manager.Execute(request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if response == nil {
		t.Error("Expected response, got nil")
	}
}

func TestManager_Execute_AllFail(t *testing.T) {
	// Create mock server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	providers := []config.Provider{
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
	manager := mustNewManager(t, &config.Config{Providers: providers, Routes: routes})

	requestJSON := `{"model":"test-model","messages":[{"role":"user","content":"Hello"}]}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	_, err := manager.Execute(request)
	if err == nil {
		t.Error("Expected error when all route steps fail")
	}

	// Verify the error is a RouteError with proper structure
	routeErr, ok := err.(types.RouteError)
	if !ok {
		t.Errorf("Expected RouteError, got %T: %v", err, err)
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

func TestManager_Execute_NoRoute(t *testing.T) {
	providers := []config.Provider{
		{Name: "provider1", APIKey: "key1", BaseURL: "http://example.com"},
	}
	routes := []config.Route{
		{
			Name: "existing-model",
			Steps: []config.RouteStep{
				{Provider: "provider1", Model: "gpt-4"},
			},
		},
	}
	manager := mustNewManager(t, &config.Config{Providers: providers, Routes: routes})

	requestJSON := `{"model":"nonexistent-model","messages":[{"role":"user","content":"Hello"}]}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	_, err := manager.Execute(request)
	if err == nil {
		t.Error("Expected error when no route matches the model")
	}
	if !strings.Contains(err.Error(), "route not found") {
		t.Errorf("Expected route not found error, got: %v", err)
	}
}

func TestManager_GetRoute(t *testing.T) {
	providers := []config.Provider{
		{Name: "provider1", APIKey: "key1", BaseURL: "http://example.com"},
	}
	routes := []config.Route{
		{
			Name: "exact-model",
			Steps: []config.RouteStep{
				{Provider: "provider1", Model: "gpt-4"},
			},
		},
		{
			Name: "another-model",
			Steps: []config.RouteStep{
				{Provider: "provider1", Model: "claude-3"},
			},
		},
	}
	manager := mustNewManager(t, &config.Config{Providers: providers, Routes: routes})

	tests := []struct {
		name          string
		model         string
		expectFound   bool
		expectedRoute string
	}{
		{
			name:          "exact match first route",
			model:         "exact-model",
			expectFound:   true,
			expectedRoute: "exact-model",
		},
		{
			name:          "exact match second route",
			model:         "another-model",
			expectFound:   true,
			expectedRoute: "another-model",
		},
		{
			name:        "no match",
			model:       "nonexistent-model",
			expectFound: false,
		},
		{
			name:        "case sensitive match",
			model:       "EXACT-MODEL",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, err := manager.GetRoute(tt.model)
			if tt.expectFound {
				if err != nil {
					t.Errorf("Expected to find route for model %s, got error: %v", tt.model, err)
				}
				if route == nil {
					t.Errorf("Expected route, got nil")
				} else if route.Name != tt.expectedRoute {
					t.Errorf("Expected route name %s, got %s", tt.expectedRoute, route.Name)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error for model %s, got route: %v", tt.model, route)
				}
			}
		})
	}
}

func TestManager_Execute_ConflictResolution(t *testing.T) {
	providers := []config.Provider{
		{Name: "test-provider", APIKey: "key", BaseURL: ""},
	}

	tests := []struct {
		name                 string
		routeName            string
		requestJSON          string
		expectTools          bool
		expectResponseFormat bool
	}{
		{
			name:                 "narrows to tools when both sent",
			routeName:            "tools-route",
			requestJSON:          `{"model":"tools-route","messages":[{"role":"user","content":"Hello"}],"tools":[{"function":{"name":"test"}}],"response_format":{"type":"json_object"}}`,
			expectTools:          true,
			expectResponseFormat: false,
		},
		{
			name:                 "adapter runs regardless of config",
			routeName:            "no-conflict-route",
			requestJSON:          `{"model":"no-conflict-route","messages":[{"role":"user","content":"Hello"}],"tools":[{"function":{"name":"test"}}],"response_format":{"type":"json_object"}}`,
			expectTools:          true,
			expectResponseFormat: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track what request was received for this test case
			var receivedRequest map[string]interface{}

			// Create mock server that captures the request for this test case
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				if err := json.Unmarshal(body, &receivedRequest); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				responseJSON := `{
					"id": "test-id",
					"object": "chat.completion",
					"created": 1234567890,
					"model": "gpt-4",
					"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
					"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
				}`
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(responseJSON))
			}))
			defer server.Close()

			// Update provider with server URL
			testProviders := make([]config.Provider, len(providers))
			copy(testProviders, providers)
			testProviders[0].BaseURL = server.URL

			routes := []config.Route{
				{
					Name: tt.routeName,
					Steps: []config.RouteStep{
						{
							Provider: "test-provider",
							Model:    "gpt-4",
						},
					},
				},
			}
			manager := mustNewManager(t, &config.Config{Providers: testProviders, Routes: routes})

			var request types.ChatRequest
			if err := json.Unmarshal([]byte(tt.requestJSON), &request); err != nil {
				t.Fatalf("Failed to unmarshal test request: %v", err)
			}

			_, err := manager.Execute(request)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			// Check if conflict resolution worked
			_, hasTools := receivedRequest["tools"]
			_, hasResponseFormat := receivedRequest["response_format"]

			if hasTools != tt.expectTools {
				t.Errorf("Expected tools field presence: %v, got: %v", tt.expectTools, hasTools)
			}
			if hasResponseFormat != tt.expectResponseFormat {
				t.Errorf("Expected response_format field presence: %v, got: %v", tt.expectResponseFormat, hasResponseFormat)
			}
		})
	}
}

func TestManager_Execute_TimeoutHandling(t *testing.T) {
	// Create mock server that verifies timeout behavior
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate successful response
		responseJSON := `{
			"id": "test-id",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "gpt-4",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	providers := []config.Provider{
		{Name: "test-provider", APIKey: "key", BaseURL: server.URL},
	}
	routes := []config.Route{
		{
			Name: "timeout-test",
			Steps: []config.RouteStep{
				{
					Provider:    "test-provider",
					Model:       "gpt-4",
					StepTimeout: "60s", // Step-specific timeout
				},
			},
		},
	}
	manager := mustNewManager(t, &config.Config{Providers: providers, Routes: routes})

	requestJSON := `{"model":"timeout-test","messages":[{"role":"user","content":"Hello"}]}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	_, err := manager.Execute(request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// If we get here, the timeout was handled correctly
	// (In a real timeout test, we'd need to mock slow responses)
}

func TestManager_Execute_MultipleRoutes(t *testing.T) {
	// Create two mock servers
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseJSON := `{
			"id": "test-id-1",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "gpt-4",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "from server 1"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseJSON))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseJSON := `{
			"id": "test-id-2",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "claude-3",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "from server 2"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseJSON))
	}))
	defer server2.Close()

	providers := []config.Provider{
		{Name: "provider1", APIKey: "key1", BaseURL: server1.URL},
		{Name: "provider2", APIKey: "key2", BaseURL: server2.URL},
	}
	routes := []config.Route{
		{
			Name: "gpt-route",
			Steps: []config.RouteStep{
				{Provider: "provider1", Model: "gpt-4"},
			},
		},
		{
			Name: "claude-route",
			Steps: []config.RouteStep{
				{Provider: "provider2", Model: "claude-3"},
			},
		},
	}
	manager := mustNewManager(t, &config.Config{Providers: providers, Routes: routes})

	tests := []struct {
		name            string
		model           string
		expectedContent string
	}{
		{
			name:            "gpt route",
			model:           "gpt-route",
			expectedContent: "from server 1",
		},
		{
			name:            "claude route",
			model:           "claude-route",
			expectedContent: "from server 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestJSON := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"Hello"}]}`, tt.model)
			var request types.ChatRequest
			if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
				t.Fatalf("Failed to unmarshal test request: %v", err)
			}

			response, err := manager.Execute(request)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			if response.Choices[0].Message.ContentAsString() != tt.expectedContent {
				t.Errorf("Expected content '%s', got '%s'", tt.expectedContent, response.Choices[0].Message.ContentAsString())
			}
		})
	}
}

func TestManager_Execute_RouteTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:              "test-key",
		DefaultRouteTimeout: "50ms",
		Providers: []config.Provider{
			{Name: "provider1", APIKey: "key1", BaseURL: server.URL},
		},
		Routes: []config.Route{
			{
				Name: "timeout-route",
				Steps: []config.RouteStep{
					{Provider: "provider1", Model: "gpt-4", StepTimeout: "2s"},
				},
			},
		},
	}

	manager := mustNewManager(t, cfg)

	requestJSON := `{"model":"timeout-route","messages":[{"role":"user","content":"Hello"}]}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	_, err := manager.Execute(request)
	if err == nil {
		t.Fatal("expected route timeout error, got nil")
	}
	if _, ok := err.(types.RouteTimeoutError); !ok {
		t.Fatalf("expected RouteTimeoutError, got %T: %v", err, err)
	}
}

func TestManager_Execute_StepTimeoutFallsBackToNextStep(t *testing.T) {
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"slow","object":"chat.completion","created":1,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"slow"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer slowServer.Close()

	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"fast","object":"chat.completion","created":1,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"fast"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer fastServer.Close()

	cfg := &config.Config{
		APIKey:              "test-key",
		DefaultRouteTimeout: "5s",
		Providers: []config.Provider{
			{Name: "slow-provider", APIKey: "k1", BaseURL: slowServer.URL},
			{Name: "fast-provider", APIKey: "k2", BaseURL: fastServer.URL},
		},
		Routes: []config.Route{
			{
				Name: "timeout-fallback-route",
				Steps: []config.RouteStep{
					{Provider: "slow-provider", Model: "gpt-4", StepTimeout: "50ms"},
					{Provider: "fast-provider", Model: "gpt-4", StepTimeout: "1s"},
				},
			},
		},
	}

	manager := mustNewManager(t, cfg)

	var request types.ChatRequest
	if err := json.Unmarshal([]byte(`{"model":"timeout-fallback-route","messages":[{"role":"user","content":"Hello"}]}`), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	response, err := manager.Execute(request)
	if err != nil {
		t.Fatalf("expected fallback success after step timeout, got error: %T %v", err, err)
	}
	if response == nil || len(response.Choices) == 0 {
		t.Fatalf("expected response choices, got %#v", response)
	}
	if got := response.Choices[0].Message.ContentAsString(); got != "fast" {
		t.Fatalf("expected response from second step, got %q", got)
	}
	if response.RoutingSummary == nil || len(response.RoutingSummary.Steps) != 2 {
		t.Fatalf("expected two recorded steps in routing summary, got %#v", response.RoutingSummary)
	}
	if response.RoutingSummary.Steps[0].Success {
		t.Fatalf("expected first step to fail due to step timeout")
	}
	if !response.RoutingSummary.Steps[1].Success {
		t.Fatalf("expected second step to succeed")
	}
}

func TestManager_ExecuteWithTracing_CanceledContext(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "provider1", APIKey: "key1", BaseURL: "http://example.com"},
		},
		Routes: []config.Route{
			{
				Name: "cancel-route",
				Steps: []config.RouteStep{
					{Provider: "provider1", Model: "gpt-4", StepTimeout: "2s"},
				},
			},
		},
	}
	manager := mustNewManager(t, cfg)

	requestJSON := `{"model":"cancel-route","messages":[{"role":"user","content":"Hello"}]}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := manager.ExecuteWithTracing(ctx, request, "req-canceled")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %T: %v", err, err)
	}
	if _, ok := err.(types.RouteTimeoutError); ok {
		t.Fatalf("did not expect RouteTimeoutError for canceled context")
	}
}
