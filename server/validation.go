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

		// Validate content - can be string or array
		if len(msg.Content) == 0 {
			return fmt.Errorf("message[%d]: content is required", i)
		}

		// Check if content is valid (either string or array)
		if !msg.IsContentString() && !msg.IsContentArray() {
			return fmt.Errorf("message[%d]: content must be either a string or an array of content blocks", i)
		}

		// If content is a string, ensure it's not empty
		if msg.IsContentString() {
			contentStr := msg.ContentAsString()
			if strings.TrimSpace(contentStr) == "" {
				return fmt.Errorf("message[%d]: content string cannot be empty", i)
			}
		}

		// If content is an array, validate it has at least one element
		if msg.IsContentArray() {
			contentArr := msg.ContentAsArray()
			if len(contentArr) == 0 {
				return fmt.Errorf("message[%d]: content array cannot be empty", i)
			}
			// Could add more validation for array elements here if needed
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