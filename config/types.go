package config

import (
	"time"
)

// Config represents the gateway configuration
type Config struct {
	APIKey         string     `yaml:"api_key"`
	Port           int        `yaml:"port"`
	DefaultTimeout string     `yaml:"default_timeout"`
	Providers      []Provider `yaml:"providers"`
	Routes         []Route    `yaml:"routes"`
}

// Provider represents a single AI provider configuration
type Provider struct {
	Name    string `yaml:"name"`
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

// Route represents a route configuration that matches incoming request models
type Route struct {
	Name  string      `yaml:"name"`
	Steps []RouteStep `yaml:"steps"`
}

// RouteStep represents a single step in a route
type RouteStep struct {
	Provider           string `yaml:"provider"`
	Model              string `yaml:"model"`
	Timeout            string `yaml:"timeout,omitempty"`
	ConflictResolution string `yaml:"conflict_resolution,omitempty"`
}

// GetTimeout returns the timeout as a time.Duration for a route step
func GetTimeout(stepTimeout, defaultTimeout string) time.Duration {
	// Use step timeout if provided, otherwise use default
	timeoutStr := stepTimeout
	if timeoutStr == "" {
		timeoutStr = defaultTimeout
	}
	if timeoutStr == "" {
		return 30 * time.Second // Final fallback
	}

	duration, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return 30 * time.Second // Default on parse error
	}
	return duration
}

// GetDefaultTimeout returns the default timeout as a time.Duration
func (c *Config) GetDefaultTimeout() time.Duration {
	return GetTimeout("", c.DefaultTimeout)
}