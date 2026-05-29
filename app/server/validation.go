package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"ai-gateway/types"
)

type validationMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Reasoning json.RawMessage `json:"reasoning,omitempty"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
}

func (m validationMessage) asMessage() types.Message {
	return types.Message{Role: m.Role, Content: m.Content}
}

func (m validationMessage) allowsEmptyContent() bool {
	if m.Role != "assistant" {
		return false
	}
	return hasNonEmptyReasoning(m.Reasoning) || hasNonEmptyToolCalls(m.ToolCalls)
}

func hasNonEmptyReasoning(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return false
	}
	return strings.TrimSpace(s) != ""
}

func hasNonEmptyToolCalls(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var calls []json.RawMessage
	if err := json.Unmarshal(raw, &calls); err != nil {
		return false
	}
	return len(calls) > 0
}

// validateChatRequest performs basic validation on chat completion requests
func validateChatRequest(req *types.ChatRequest) error {
	var temp struct {
		Messages []validationMessage `json:"messages"`
	}
	if err := json.Unmarshal(req.Raw, &temp); err != nil {
		return fmt.Errorf("failed to parse messages: %w", err)
	}

	if len(temp.Messages) == 0 {
		return fmt.Errorf("messages array is required and cannot be empty")
	}

	validRoles := []string{"system", "user", "assistant", "tool"}

	for i, vm := range temp.Messages {
		if strings.TrimSpace(vm.Role) == "" {
			return fmt.Errorf("message[%d]: role is required", i)
		}

		valid := false
		for _, role := range validRoles {
			if vm.Role == role {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("message[%d]: invalid role '%s', must be one of: %s", i, vm.Role, strings.Join(validRoles, ", "))
		}

		if vm.allowsEmptyContent() {
			continue
		}

		msg := vm.asMessage()

		if len(msg.Content) == 0 {
			return fmt.Errorf("message[%d]: content is required", i)
		}

		if !msg.IsContentString() && !msg.IsContentArray() {
			return fmt.Errorf("message[%d]: content must be either a string or an array of content blocks", i)
		}

		if msg.IsContentString() {
			if strings.TrimSpace(msg.ContentAsString()) == "" {
				return fmt.Errorf("message[%d]: content string cannot be empty", i)
			}
		}

		if msg.IsContentArray() {
			if len(msg.ContentAsArray()) == 0 {
				return fmt.Errorf("message[%d]: content array cannot be empty", i)
			}
		}
	}

	return nil
}
