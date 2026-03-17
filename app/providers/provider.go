package providers

import (
	"ai-gateway/types"
)

// Provider defines the interface for AI providers
type Provider interface {
	// Call executes a chat completion request
	Call(request types.ChatRequest) (*types.ChatResponse, error)
	// Name returns the provider name
	Name() string
	// IsAvailable checks if the provider is available
	IsAvailable() bool
}