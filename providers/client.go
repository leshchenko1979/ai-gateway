package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/types"
)

// Client implements the Provider interface for OpenAI-compatible APIs
type Client struct {
	name               string
	apiKey             string
	baseURL            string
	model              string
	timeout            time.Duration
	conflictResolution string // "tools" or "format" or empty
	logger             *logger.Logger
	client             *http.Client
}

// NewClient creates a new OpenAI-compatible provider client
func NewClient(cfg config.Provider, logger *logger.Logger) *Client {
	// Legacy constructor - uses default timeout and no conflict resolution
	return &Client{
		name:               cfg.Name,
		apiKey:             cfg.APIKey,
		baseURL:            cfg.BaseURL,
		model:              "", // Will be overridden by route step
		timeout:            30 * time.Second,
		conflictResolution: "",
		logger:             logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewClientWithRouteStep creates a provider client configured for a specific route step
func NewClientWithRouteStep(providerCfg config.Provider, step config.RouteStep, logger *logger.Logger) *Client {
	// Get timeout from step or use default
	timeout := config.GetTimeout(step.Timeout, "30s") // Default to 30s if no default configured

	return &Client{
		name:               providerCfg.Name,
		apiKey:             providerCfg.APIKey,
		baseURL:            providerCfg.BaseURL,
		model:              step.Model,
		timeout:            timeout,
		conflictResolution: step.ConflictResolution,
		logger:             logger,
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
func (c *Client) Call(request types.ChatRequest) (*types.ChatResponse, error) {
	// Override model with provider's configured model
	request.Model = c.model

	// Apply conflict resolution if specified
	if c.conflictResolution != "" {
		if err := c.applyConflictResolution(&request); err != nil {
			return nil, fmt.Errorf("failed to apply conflict resolution: %w", err)
		}
	}

	// Prepare request body
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/chat/completions", c.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
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

// applyConflictResolution modifies the request to resolve tools/response_format conflicts
func (c *Client) applyConflictResolution(request *types.ChatRequest) error {
	// Parse the raw JSON to manipulate it
	var reqMap map[string]interface{}
	if err := json.Unmarshal(request.Raw, &reqMap); err != nil {
		return fmt.Errorf("failed to parse request JSON: %w", err)
	}

	switch c.conflictResolution {
	case "tools":
		// Remove response_format field, keep tools
		delete(reqMap, "response_format")
	case "format":
		// Remove tools field, keep response_format
		delete(reqMap, "tools")
	default:
		// No conflict resolution needed
		return nil
	}

	// Re-marshal the modified request
	modifiedRaw, err := json.Marshal(reqMap)
	if err != nil {
		return fmt.Errorf("failed to marshal modified request: %w", err)
	}

	request.Raw = modifiedRaw
	return nil
}