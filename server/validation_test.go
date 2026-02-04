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