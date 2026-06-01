package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/types"
)

// RequestAdapter adapts a chat request for provider/model peculiarities
type RequestAdapter func(request *types.ChatRequest) error

// Client implements the Provider interface for OpenAI-compatible APIs
type Client struct {
	name      string
	apiKey    string
	baseURL   string
	model     string
	timeout   time.Duration
	adapters  []RequestAdapter
	logger    *logger.Logger
	client    *http.Client
}

// NewClient creates a new OpenAI-compatible provider client
func NewClient(cfg config.Provider, logger *logger.Logger) *Client {
	// Legacy constructor - uses default timeout and no conflict resolution
	return &Client{
		name:     cfg.Name,
		apiKey:   cfg.APIKey,
		baseURL:  cfg.BaseURL,
		model:    "", // Will be overridden by route step
		timeout:  30 * time.Second,
		adapters: defaultAdapters(),
		logger:   logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewClientWithRouteStep creates a provider client configured for a specific route step
func NewClientWithRouteStep(providerCfg config.Provider, step config.RouteStep, defaultStepTimeout string, logger *logger.Logger) *Client {
	// Get timeout from step or use default
	timeout := config.GetStepTimeout(step.StepTimeout, defaultStepTimeout)

	return &Client{
		name:     providerCfg.Name,
		apiKey:   providerCfg.APIKey,
		baseURL:  providerCfg.BaseURL,
		model:    step.Model,
		timeout:  timeout,
		adapters: defaultAdapters(),
		logger:   logger,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the provider name
func (c *Client) Name() string {
	return c.name
}

// IsAvailable checks if the provider is available
func (c *Client) IsAvailable() bool {
	// Simple check - could be enhanced with actual health check
	return c.apiKey != "" && c.baseURL != ""
}

// Call executes a chat completion request
func (c *Client) Call(ctx context.Context, request types.ChatRequest) (*types.ChatResponse, error) {
	// Override model with provider's configured model
	request.Model = c.model

	// Apply request adapters
	for i, adapter := range c.adapters {
		if err := adapter(&request); err != nil {
			return nil, fmt.Errorf("adapter[%d] failed: %w", i, err)
		}
	}

	// Prepare request body
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/chat/completions", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	// Execute request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(body))
	}

	// Store response as raw JSON (pass through unchanged)
	var response types.ChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// defaultAdapters returns the standard set of request adapters
func defaultAdapters() []RequestAdapter {
	return []RequestAdapter{
		adaptConflictResolution,
		adaptToolChoice,
	}
}

// adaptConflictResolution resolves tools/response_format conflicts by preferring tools
func adaptConflictResolution(request *types.ChatRequest) error {
	// Parse the raw JSON to manipulate it
	var reqMap map[string]interface{}
	if err := json.Unmarshal(request.Raw, &reqMap); err != nil {
		return fmt.Errorf("failed to parse request JSON: %w", err)
	}

	// If tools are present, remove response_format to avoid conflicts
	if _, hasTools := reqMap["tools"]; hasTools {
		delete(reqMap, "response_format")
	}

	// Re-marshal the modified request
	modifiedRaw, err := json.Marshal(reqMap)
	if err != nil {
		return fmt.Errorf("failed to marshal modified request: %w", err)
	}

	request.Raw = modifiedRaw
	return nil
}

// adaptToolChoice relaxes forced tool_choice for models that don't support it
func adaptToolChoice(request *types.ChatRequest) error {
	// Only applies to thinking/reasoning models
	if !isThinkingModel(request.Model) {
		return nil
	}

	// Parse the raw JSON to manipulate it
	var reqMap map[string]interface{}
	if err := json.Unmarshal(request.Raw, &reqMap); err != nil {
		return fmt.Errorf("failed to parse request JSON: %w", err)
	}

	// If tool_choice is "required", relax to "auto"
	if tc, ok := reqMap["tool_choice"]; ok {
		if tcStr, ok := tc.(string); ok && tcStr == "required" {
			reqMap["tool_choice"] = "auto"
		}
	}

	// Re-marshal the modified request
	modifiedRaw, err := json.Marshal(reqMap)
	if err != nil {
		return fmt.Errorf("failed to marshal modified request: %w", err)
	}

	request.Raw = modifiedRaw
	return nil
}

// isThinkingModel returns true if the model uses thinking/reasoning mode
func isThinkingModel(model string) bool {
	switch model {
	case "deepseek-v4-flash", "deepseek/deepseek-v4-flash",
		"deepseek-reasoner", "deepseek/deepseek-r1":
		return true
	}
	return false
}
