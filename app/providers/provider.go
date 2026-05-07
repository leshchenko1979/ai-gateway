package providers

import (
	"context"

	"ai-gateway/types"
)

// Provider defines the interface for AI providers
type Provider interface {
	// Call executes a chat completion request
	Call(ctx context.Context, request types.ChatRequest) (*types.ChatResponse, error)
	// Name returns the provider name
	Name() string
	// IsAvailable checks if the provider is available
	IsAvailable() bool
}
