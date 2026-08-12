package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/types"
)

func TestClient_Call(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Parse request
		var req types.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Failed to decode request"))
			return
		}

		// Verify model override
		if req.Model != "gpt-4" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Return mock response as raw JSON
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
						"content": "Test response"
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 20,
				"total_tokens": 30
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	// Create client
	cfg := config.Provider{
		Name:    "test-provider",
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	}
	step := config.RouteStep{
		Provider:    "test-provider",
		Model:       "gpt-4",
		StepTimeout: "30s",
	}
	logger := logger.NewLogger()
	client := NewClientWithRouteStep(cfg, step, "30s", logger)

	// Test call - create request from JSON
	requestJSON := `{"model":"original-model","messages":[{"role":"user","content":"Hello"}]}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	response, err := client.Call(context.Background(), request)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	if response.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", response.Model)
	}

	if len(response.Choices) == 0 {
		t.Error("Expected at least one choice")
	}
}

func TestClient_Name(t *testing.T) {
	cfg := config.Provider{Name: "test-provider"}
	client := NewClient(cfg, logger.NewLogger())
	if client.Name() != "test-provider" {
		t.Errorf("Expected 'test-provider', got '%s'", client.Name())
	}
}

func TestClient_Call_ReplacesOnlyModel(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Read the raw request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Parse received request
		var received map[string]interface{}
		if err := json.Unmarshal(body, &received); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Verify model was replaced
		if received["model"] != "gpt-4" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Model not replaced"))
			return
		}

		// Verify all other fields are exactly as sent (except model)
		expected := map[string]interface{}{
			"temperature":     0.7,
			"max_tokens":      float64(100),
			"stream":          false,
			"response_format": map[string]interface{}{"type": "json_object"},
			"custom_field":    "preserved",
			"messages": []interface{}{
				map[string]interface{}{
					"role":    "user",
					"content": "Hello",
				},
			},
		}

		for key, expectedValue := range expected {
			if actualValue, exists := received[key]; !exists {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(fmt.Sprintf("Missing field: %s", key)))
				return
			} else if fmt.Sprintf("%v", actualValue) != fmt.Sprintf("%v", expectedValue) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(fmt.Sprintf("Field %s mismatch: got %v, want %v", key, actualValue, expectedValue)))
				return
			}
		}

		// Return mock response as raw JSON
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
						"content": "Test response"
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 20,
				"total_tokens": 30
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	// Create client
	cfg := config.Provider{
		Name:    "test-provider",
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	}
	step := config.RouteStep{
		Provider:    "test-provider",
		Model:       "gpt-4",
		StepTimeout: "30s",
	}
	logger := logger.NewLogger()
	client := NewClientWithRouteStep(cfg, step, "30s", logger)

	// Create request with many fields
	requestJSON := `{
		"model": "original-model",
		"messages": [{"role": "user", "content": "Hello"}],
		"temperature": 0.7,
		"max_tokens": 100,
		"stream": false,
		"response_format": {"type": "json_object"},
		"custom_field": "preserved"
	}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	// Call should succeed and replace only the model
	response, err := client.Call(context.Background(), request)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	if response.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", response.Model)
	}
}

func TestClient_IsAvailable(t *testing.T) {
	tests := []struct {
		name     string
		provider config.Provider
		want     bool
	}{
		{
			name: "available",
			provider: config.Provider{
				APIKey:  "key",
				BaseURL: "http://test.com",
			},
			want: true,
		},
		{
			name: "missing api key",
			provider: config.Provider{
				BaseURL: "http://test.com",
			},
			want: false,
		},
		{
			name: "missing base url",
			provider: config.Provider{
				APIKey: "key",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.provider, logger.NewLogger())
			if got := client.IsAvailable(); got != tt.want {
				t.Errorf("IsAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_ConflictResolution_Tools(t *testing.T) {
	// Create mock server that verifies conflict resolution
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Parse received request
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var received map[string]interface{}
		if err := json.Unmarshal(body, &received); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Verify model was replaced
		if received["model"] != "gpt-4" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Model not replaced"))
			return
		}

		// Verify response_format was removed (conflict_resolution: "tools")
		if _, exists := received["response_format"]; exists {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("response_format should be removed"))
			return
		}

		// Verify tools is still present
		if _, exists := received["tools"]; !exists {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("tools should be preserved"))
			return
		}

		// Return success response
		responseJSON := `{"id": "test", "object": "chat.completion", "created": 123, "model": "gpt-4", "choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}], "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	// Create client - adapters are always on
	cfg := config.Provider{
		Name:    "test-provider",
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	}
	step := config.RouteStep{
		Provider:    "test-provider",
		Model:       "gpt-4",
		StepTimeout: "30s",
	}
	logger := logger.NewLogger()
	client := NewClientWithRouteStep(cfg, step, "30s", logger)

	// Create request with both tools and response_format
	requestJSON := `{"model":"original","messages":[{"role":"user","content":"Hello"}],"tools":[{"function":{"name":"test"}}],"response_format":{"type":"json_object"}}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal test request: %v", err)
	}

	_, err := client.Call(context.Background(), request)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
}

func TestNewClientWithRouteStep_UsesFallbackWhenStepTimeoutEmpty(t *testing.T) {
	cfg := config.Provider{
		Name:    "test-provider",
		APIKey:  "test-api-key",
		BaseURL: "https://example.com",
	}
	step := config.RouteStep{
		Provider: "test-provider",
		Model:    "gpt-4",
	}

	client := NewClientWithRouteStep(cfg, step, "2m", logger.NewLogger())
	if client.client.Timeout != 2*time.Minute {
		t.Fatalf("client timeout = %v, want %v", client.client.Timeout, 2*time.Minute)
	}
}

// --- Strict JSON schema adapter tests ---

func TestClient_StrictSchemaAdapter_AppliesForStrictProviders(t *testing.T) {
	// Echo server captures the received body.
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &received); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	// Provider with StrictJSONSchema=true (e.g. groq).
	cfg := config.Provider{Name: "groq", APIKey: "k", BaseURL: server.URL, StrictJSONSchema: true}
	step := config.RouteStep{Provider: "groq", Model: "gpt-4"}
	client := NewClientWithRouteStep(cfg, step, "30s", logger.NewLogger())

	requestJSON := `{"model":"orig","messages":[{"role":"user","content":"Hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"spam","schema":{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"object","properties":{"c":{"type":"boolean"}}}},"required":["a"]}}}}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, err := client.Call(context.Background(), request); err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	rf := received["response_format"].(map[string]interface{})
	js := rf["json_schema"].(map[string]interface{})
	schema := js["schema"].(map[string]interface{})
	if ap, ok := schema["additionalProperties"]; !ok || ap != false {
		t.Fatalf("expected additionalProperties:false on root object, got %v", schema["additionalProperties"])
	}
	props := schema["properties"].(map[string]interface{})
	nested := props["b"].(map[string]interface{})
	if ap, ok := nested["additionalProperties"]; !ok || ap != false {
		t.Fatalf("expected additionalProperties:false on nested object b, got %v", nested["additionalProperties"])
	}
}

func TestClient_StrictSchemaAdapter_DoesNotApplyForNonStrictProviders(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	// Provider WITHOUT StrictJSONSchema (e.g. opencode) — request must pass through unchanged.
	cfg := config.Provider{Name: "opencode", APIKey: "k", BaseURL: server.URL}
	step := config.RouteStep{Provider: "opencode", Model: "deepseek-v4-flash"}
	client := NewClientWithRouteStep(cfg, step, "30s", logger.NewLogger())

	requestJSON := `{"model":"orig","messages":[{"role":"user","content":"Hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"spam","schema":{"type":"object","properties":{"a":{"type":"string"}},"required":["a"]}}}}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, err := client.Call(context.Background(), request); err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	rf := received["response_format"].(map[string]interface{})
	js := rf["json_schema"].(map[string]interface{})
	schema := js["schema"].(map[string]interface{})
	if _, exists := schema["additionalProperties"]; exists {
		t.Fatalf("additionalProperties should NOT be added for non-strict provider, got %v", schema["additionalProperties"])
	}
}

func TestClient_StrictSchemaAdapter_NoSchemaLeavesRequestUntouched(t *testing.T) {
	// Request without response_format.json_schema must pass through unchanged.
	cfg := config.Provider{Name: "groq", APIKey: "k", BaseURL: "https://example.com", StrictJSONSchema: true}
	step := config.RouteStep{Provider: "groq", Model: "gpt-4"}
	_ = NewClientWithRouteStep(cfg, step, "30s", logger.NewLogger())

	requestJSON := `{"model":"orig","messages":[{"role":"user","content":"Hi"}]}`
	var request types.ChatRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if err := adaptStrictJSONSchema(&request); err != nil {
		t.Fatalf("adaptStrictJSONSchema() error = %v", err)
	}
	if string(request.Raw) != requestJSON {
		t.Fatalf("request was modified without json_schema: %s", string(request.Raw))
	}
}
