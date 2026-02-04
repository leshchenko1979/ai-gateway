package types

import (
	"encoding/json"
	"fmt"

	"ai-gateway/config"
)

// ErrorResponse represents a unified error response format
type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

// ErrorDetails contains detailed error information
type ErrorDetails struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

// RouteError represents a detailed error when all route steps fail
type RouteError struct {
	Route  config.Route      `json:"route"`
	Errors []RouteStepError  `json:"errors"`
}

// RouteStepError represents an error from a specific route step
type RouteStepError struct {
	StepIndex  int    `json:"step_index"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Error      string `json:"error"`
}

// Error implements the error interface for RouteError
func (e RouteError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("all route steps failed for model '%s'", e.Route.Name)
	}
	lastErr := e.Errors[len(e.Errors)-1]
	return fmt.Sprintf("all route steps failed for model '%s', last error from %s/%s: %s",
		e.Route.Name, lastErr.Provider, lastErr.Model, lastErr.Error)
}

// truncateContent truncates content to first 100 characters
func truncateContent(content string) string {
	const maxLength = 100
	if len(content) <= maxLength {
		return content
	}
	return content[:maxLength] + "..."
}

// truncateMessageContent truncates message content (string or array)
func truncateMessageContent(content json.RawMessage) json.RawMessage {
	// Try to unmarshal as string
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		truncated := truncateContent(s)
		if truncatedBytes, err := json.Marshal(truncated); err == nil {
			return truncatedBytes
		}
	}

	// Try to unmarshal as array
	var a []interface{}
	if err := json.Unmarshal(content, &a); err == nil {
		// For arrays, truncate each text element
		truncated := make([]interface{}, len(a))
		for i, item := range a {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if itemMap["type"] == "text" {
					if text, ok := itemMap["text"].(string); ok {
						truncatedItem := make(map[string]interface{})
						for k, v := range itemMap {
							if k == "text" {
								truncatedItem[k] = truncateContent(text)
							} else {
								truncatedItem[k] = v
							}
						}
						truncated[i] = truncatedItem
						continue
					}
				}
			}
			truncated[i] = item
		}
		if truncatedBytes, err := json.Marshal(truncated); err == nil {
			return truncatedBytes
		}
	}

	// Return original if we can't process it
	return content
}

// truncateMessages creates a copy of messages with truncated content
func truncateMessages(messages []Message) []Message {
	truncated := make([]Message, len(messages))
	for i, msg := range messages {
		truncated[i] = Message{
			Role:    msg.Role,
			Content: truncateMessageContent(msg.Content),
		}
	}
	return truncated
}

// TruncateRequestForLogging creates a copy of ChatRequest with truncated message contents
func (r *ChatRequest) TruncateRequestForLogging() *ChatRequest {
	if r.Raw == nil {
		return &ChatRequest{Model: r.Model}
	}

	var temp map[string]interface{}
	if err := json.Unmarshal(r.Raw, &temp); err != nil {
		return &ChatRequest{Model: r.Model}
	}

	// Truncate message contents
	if msgs, ok := temp["messages"].([]interface{}); ok {
		truncatedMsgs := make([]interface{}, len(msgs))
		for i, msg := range msgs {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				truncatedMsg := make(map[string]interface{})
				for k, v := range msgMap {
					if k == "content" {
						if contentBytes, err := json.Marshal(v); err == nil {
							truncatedMsg[k] = json.RawMessage(truncateMessageContent(contentBytes))
						} else {
							truncatedMsg[k] = v
						}
					} else {
						truncatedMsg[k] = v
					}
				}
				truncatedMsgs[i] = truncatedMsg
			}
		}
		temp["messages"] = truncatedMsgs
	}

	if raw, err := json.Marshal(temp); err == nil {
		return &ChatRequest{
			Raw:   raw,
			Model: r.Model,
		}
	}

	return &ChatRequest{Model: r.Model}
}

// truncateChoices creates a copy of choices with truncated message contents
func truncateChoices(choices []Choice) []Choice {
	truncated := make([]Choice, len(choices))
	for i, choice := range choices {
		truncated[i] = Choice{
			Index:        choice.Index,
			Message:      Message{Role: choice.Message.Role, Content: truncateMessageContent(choice.Message.Content)},
			FinishReason: choice.FinishReason,
		}
	}
	return truncated
}

// TruncateResponseForLogging creates a copy of ChatResponse with truncated message contents
func (r *ChatResponse) TruncateResponseForLogging() *ChatResponse {
	if r.Raw == nil {
		return &ChatResponse{}
	}

	var temp map[string]interface{}
	if err := json.Unmarshal(r.Raw, &temp); err != nil {
		return &ChatResponse{}
	}

	// Truncate message contents in choices
	if choices, ok := temp["choices"].([]interface{}); ok {
		truncatedChoices := make([]interface{}, len(choices))
		for i, choice := range choices {
			if choiceMap, ok := choice.(map[string]interface{}); ok {
				truncatedChoice := make(map[string]interface{})
				for k, v := range choiceMap {
					if k == "message" {
						if msgMap, ok := v.(map[string]interface{}); ok {
							truncatedMsg := make(map[string]interface{})
							for msgK, msgV := range msgMap {
								if msgK == "content" {
									if contentBytes, err := json.Marshal(msgV); err == nil {
										truncatedMsg[msgK] = json.RawMessage(truncateMessageContent(contentBytes))
									} else {
										truncatedMsg[msgK] = msgV
									}
								} else {
									truncatedMsg[msgK] = msgV
								}
							}
							truncatedChoice[k] = truncatedMsg
						} else {
							truncatedChoice[k] = v
						}
					} else {
						truncatedChoice[k] = v
					}
				}
				truncatedChoices[i] = truncatedChoice
			}
		}
		temp["choices"] = truncatedChoices
	}

	if raw, err := json.Marshal(temp); err == nil {
		return &ChatResponse{Raw: raw}
	}

	return &ChatResponse{}
}

// ChatRequest represents an OpenAI-compatible chat completion request
// Stores raw JSON and allows model replacement only
type ChatRequest struct {
	Raw   json.RawMessage // Complete raw JSON from client
	Model string          // Extracted model for logging/validation
}

// UnmarshalJSON stores the raw JSON and extracts the model
func (r *ChatRequest) UnmarshalJSON(data []byte) error {
	r.Raw = make(json.RawMessage, len(data))
	copy(r.Raw, data)

	// Extract model for logging/validation
	var temp struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	r.Model = temp.Model
	return nil
}

// MarshalJSON replaces only the model field in the raw JSON
func (r ChatRequest) MarshalJSON() ([]byte, error) {
	if r.Raw == nil {
		return nil, fmt.Errorf("no raw JSON to marshal")
	}

	// Replace only the model field
	var temp map[string]interface{}
	if err := json.Unmarshal(r.Raw, &temp); err != nil {
		return nil, err
	}
	temp["model"] = r.Model
	return json.Marshal(temp)
}

// Message represents a chat message
type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ContentAsString returns the content as a string, or empty string if it's an array
func (m Message) ContentAsString() string {
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	return ""
}

// ContentAsArray returns the content as an array of interfaces, or nil if it's a string
func (m Message) ContentAsArray() []interface{} {
	var a []interface{}
	if err := json.Unmarshal(m.Content, &a); err == nil {
		return a
	}
	return nil
}

// IsContentString returns true if content is a string
func (m Message) IsContentString() bool {
	return m.ContentAsArray() == nil
}

// IsContentArray returns true if content is an array
func (m Message) IsContentArray() bool {
	return m.ContentAsArray() != nil
}

// ChatResponse represents an OpenAI-compatible chat completion response
// Stores raw JSON to pass responses through unchanged
type ChatResponse struct {
	Raw json.RawMessage // Complete raw JSON response from provider

	// Extracted fields for logging/processing
	ID      string   `json:"-"`
	Object  string   `json:"-"`
	Created int64    `json:"-"`
	Model   string   `json:"-"`
	Choices []Choice `json:"-"`
	Usage   Usage    `json:"-"`
}

// UnmarshalJSON stores raw JSON and extracts key fields for logging
func (r *ChatResponse) UnmarshalJSON(data []byte) error {
	r.Raw = make(json.RawMessage, len(data))
	copy(r.Raw, data)

	// Extract fields for logging
	var temp struct {
		ID      string   `json:"id"`
		Object  string   `json:"object"`
		Created int64    `json:"created"`
		Model   string   `json:"model"`
		Choices []Choice `json:"choices"`
		Usage   Usage    `json:"usage"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	r.ID = temp.ID
	r.Object = temp.Object
	r.Created = temp.Created
	r.Model = temp.Model
	r.Choices = temp.Choices
	r.Usage = temp.Usage
	return nil
}

// MarshalJSON returns the raw JSON unchanged
func (r ChatResponse) MarshalJSON() ([]byte, error) {
	if r.Raw == nil {
		return nil, fmt.Errorf("no raw JSON to marshal")
	}
	return r.Raw, nil
}

// Choice represents a chat completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ModelsResponse represents the models list response
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// Model represents a model in the models list
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}