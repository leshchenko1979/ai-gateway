package types

import (
	"encoding/json"
	"testing"
)

func TestChatRequest_ReplacesOnlyModel(t *testing.T) {
	// Test JSON with many OpenAI API fields
	originalJSON := `{
		"model": "gpt-3.5-turbo",
		"messages": [
			{"role": "user", "content": "Hello"}
		],
		"temperature": 0.7,
		"max_tokens": 100,
		"response_format": {"type": "json_object"},
		"stream": false,
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get weather information"
				}
			}
		],
		"custom_field": "preserved",
		"another_field": 42
	}`

	// Unmarshal into ChatRequest
	var request ChatRequest
	if err := json.Unmarshal([]byte(originalJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify extracted model
	if request.Model != "gpt-3.5-turbo" {
		t.Errorf("Expected model 'gpt-3.5-turbo', got '%s'", request.Model)
	}

	// Override model (as gateway does)
	request.Model = "claude-3-opus"

	// Marshal back to JSON
	result, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Parse result as map to verify all fields preserved except model
	var resultMap map[string]interface{}
	if err := json.Unmarshal(result, &resultMap); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Verify model was replaced
	if resultMap["model"] != "claude-3-opus" {
		t.Errorf("Model not replaced: got %v", resultMap["model"])
	}

	// Verify all original fields are preserved
	expectedFields := map[string]interface{}{
		"temperature":  0.7,
		"max_tokens":   float64(100),
		"stream":       false,
		"custom_field": "preserved",
		"another_field": float64(42),
	}

	for field, expected := range expectedFields {
		if actual, exists := resultMap[field]; !exists {
			t.Errorf("Missing field: %s", field)
		} else if actual != expected {
			t.Errorf("Field %s: expected %v, got %v", field, expected, actual)
		}
	}

	// Verify complex objects
	if respFmt, ok := resultMap["response_format"].(map[string]interface{}); !ok {
		t.Error("response_format missing or wrong type")
	} else if respFmt["type"] != "json_object" {
		t.Errorf("response_format.type: expected 'json_object', got %v", respFmt["type"])
	}

	if tools, ok := resultMap["tools"].([]interface{}); !ok || len(tools) != 1 {
		t.Error("tools field incorrect")
	}

	// Verify messages are preserved
	if messages, ok := resultMap["messages"].([]interface{}); !ok || len(messages) != 1 {
		t.Error("messages field incorrect")
	} else if msg, ok := messages[0].(map[string]interface{}); !ok {
		t.Error("message structure incorrect")
	} else if msg["role"] != "user" || msg["content"] != "Hello" {
		t.Errorf("message content incorrect: %+v", msg)
	}
}

func TestChatRequest_TruncateRequestForLogging(t *testing.T) {
	originalJSON := `{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "This is a very long message that should be truncated for logging purposes"},
			{"role": "assistant", "content": "Short response"}
		],
		"temperature": 0.7
	}`

	var request ChatRequest
	if err := json.Unmarshal([]byte(originalJSON), &request); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	truncated := request.TruncateRequestForLogging()

	// Verify truncation worked by checking the raw JSON
	if truncated.Raw == nil {
		t.Fatal("Truncated request has no raw JSON")
	}

	var truncatedMap map[string]interface{}
	if err := json.Unmarshal(truncated.Raw, &truncatedMap); err != nil {
		t.Fatalf("Failed to unmarshal truncated JSON: %v", err)
	}

	messages, ok := truncatedMap["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Errorf("Expected 2 messages in truncated JSON, got %v", truncatedMap["messages"])
		return
	}

	// First message should be truncated (assuming it's longer than 100 chars)
	if msg0, ok := messages[0].(map[string]interface{}); ok {
		if content, ok := msg0["content"].(string); ok {
			if len(content) > 103 { // 100 + "..."
				t.Errorf("First message not truncated: length %d", len(content))
			}
		}
	}

	// Second message should not be truncated (it's short)
	if msg1, ok := messages[1].(map[string]interface{}); ok {
		if content, ok := msg1["content"].(string); ok {
			if content != "Short response" {
				t.Errorf("Second message modified: %s", content)
			}
		}
	}

	// Model should be preserved
	if model, ok := truncatedMap["model"].(string); !ok || model != "gpt-4" {
		t.Errorf("Model not preserved: %v", truncatedMap["model"])
	}
}