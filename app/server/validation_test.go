package server

import (
	"testing"

	"ai-gateway/types"
)

func TestValidateChatRequest(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		wantErr  bool
	}{
		{
			name:     "valid request",
			jsonData: `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`,
			wantErr:  false,
		},
		{
			name:     "empty messages",
			jsonData: `{"model":"gpt-4","messages":[]}`,
			wantErr:  true,
		},
		{
			name:     "missing role",
			jsonData: `{"model":"gpt-4","messages":[{"content":"Hello"}]}`,
			wantErr:  true,
		},
		{
			name:     "empty role",
			jsonData: `{"model":"gpt-4","messages":[{"role":"","content":"Hello"}]}`,
			wantErr:  true,
		},
		{
			name:     "missing content",
			jsonData: `{"model":"gpt-4","messages":[{"role":"user"}]}`,
			wantErr:  true,
		},
		{
			name:     "empty content",
			jsonData: `{"model":"gpt-4","messages":[{"role":"user","content":""}]}`,
			wantErr:  true,
		},
		{
			name:     "invalid role",
			jsonData: `{"model":"gpt-4","messages":[{"role":"invalid","content":"Hello"}]}`,
			wantErr:  true,
		},
		{
			name:     "assistant empty content with reasoning",
			jsonData: `{"model":"gpt-4","messages":[{"role":"assistant","content":"","reasoning":"classified as spam"}]}`,
			wantErr:  false,
		},
		{
			name:     "assistant empty content with tool_calls",
			jsonData: `{"model":"gpt-4","messages":[{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"final_result","arguments":"{}"}}]}]}`,
			wantErr:  false,
		},
		{
			name:     "assistant empty content without reasoning or tool_calls",
			jsonData: `{"model":"gpt-4","messages":[{"role":"assistant","content":""}]}`,
			wantErr:  true,
		},
		{
			name:     "user empty content with reasoning still invalid",
			jsonData: `{"model":"gpt-4","messages":[{"role":"user","content":"","reasoning":"should not bypass"}]}`,
			wantErr:  true,
		},
		{
			name:     "retry conversation with reasoning-only assistant turn",
			jsonData: `{"model":"ai-antispam","messages":[{"role":"system","content":"You are a classifier"},{"role":"user","content":"hello"},{"role":"assistant","content":"","reasoning":"likely spam"},{"role":"user","content":"Validation feedback: return a tool call"}]}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request types.ChatRequest
			if err := request.UnmarshalJSON([]byte(tt.jsonData)); err != nil {
				t.Fatalf("Failed to unmarshal test data: %v", err)
			}

			err := validateChatRequest(&request)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateChatRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
