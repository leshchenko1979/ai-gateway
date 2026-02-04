package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"ai-gateway/types"
)

// validateChatRequest performs basic validation on chat completion requests
func validateChatRequest(req *types.ChatRequest) error {
	// Extract messages from raw JSON
	var temp struct {
		Messages []types.Message `json:"messages"`
	}
	if err := json.Unmarshal(req.Raw, &temp); err != nil {
		return fmt.Errorf("failed to parse messages: %w", err)
	}

	// Check if messages array is present and not empty
	if len(temp.Messages) == 0 {
		return fmt.Errorf("messages array is required and cannot be empty")
	}

	// Validate each message
	for i, msg := range temp.Messages {
		if strings.TrimSpace(msg.Role) == "" {
			return fmt.Errorf("message[%d]: role is required", i)
		}
		if strings.TrimSpace(msg.Content) == "" {
			return fmt.Errorf("message[%d]: content is required", i)
		}

		// Validate role
		validRoles := []string{"system", "user", "assistant"}
		valid := false
		for _, role := range validRoles {
			if msg.Role == role {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("message[%d]: invalid role '%s', must be one of: %s", i, msg.Role, strings.Join(validRoles, ", "))
		}
	}

	return nil
}