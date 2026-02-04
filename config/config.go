package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads configuration from YAML file with environment variable substitution
func LoadConfig(filename string) (*Config, error) {
	// Try to load from current directory first, then from /etc/ai-gateway/
	paths := []string{
		filename,
		filepath.Join("/etc/ai-gateway", filename),
	}

	var data []byte
	var err error

	for _, path := range paths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read config file from any location: %w", err)
	}

	// Expand environment variables
	expanded := expandEnvVars(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Port == 0 {
		if port := os.Getenv("PORT"); port != "" {
			if p, err := strconv.Atoi(port); err == nil {
				config.Port = p
			}
		}
		if config.Port == 0 {
			config.Port = 8080
		}
	}

	// Set default timeout if not specified
	if config.DefaultTimeout == "" {
		config.DefaultTimeout = "30s"
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// expandEnvVars replaces ${VAR_NAME} with environment variable values
func expandEnvVars(s string) string {
	return os.Expand(s, func(key string) string {
		return os.Getenv(key)
	})
}

// validateConfig checks that required fields are present
func validateConfig(cfg *Config) error {
	if cfg.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}

	if len(cfg.Providers) == 0 {
		return fmt.Errorf("at least one provider must be configured")
	}

	// Build provider name map for route validation
	providerNames := make(map[string]bool)
	for _, provider := range cfg.Providers {
		providerNames[provider.Name] = true
	}

	// Validate providers
	for i, provider := range cfg.Providers {
		if strings.TrimSpace(provider.Name) == "" {
			return fmt.Errorf("provider[%d]: name is required", i)
		}
		if strings.TrimSpace(provider.APIKey) == "" {
			return fmt.Errorf("provider[%d] (%s): api_key is required", i, provider.Name)
		}
		if strings.TrimSpace(provider.BaseURL) == "" {
			return fmt.Errorf("provider[%d] (%s): base_url is required", i, provider.Name)
		}
		// Providers no longer have Model and Timeout fields
		cfg.Providers[i] = provider
	}

	// Validate routes
	for i, route := range cfg.Routes {
		if strings.TrimSpace(route.Name) == "" {
			return fmt.Errorf("route[%d]: name is required", i)
		}
		if len(route.Steps) == 0 {
			return fmt.Errorf("route[%d] (%s): at least one step must be configured", i, route.Name)
		}

		// Validate route steps
		for j, step := range route.Steps {
			if strings.TrimSpace(step.Provider) == "" {
				return fmt.Errorf("route[%d] (%s) step[%d]: provider is required", i, route.Name, j)
			}
			if strings.TrimSpace(step.Model) == "" {
				return fmt.Errorf("route[%d] (%s) step[%d]: model is required", i, route.Name, j)
			}
			// Validate provider reference
			if !providerNames[step.Provider] {
				return fmt.Errorf("route[%d] (%s) step[%d]: provider '%s' not found in providers list", i, route.Name, j, step.Provider)
			}
			// Validate timeout if provided
			if step.Timeout != "" {
				if _, err := time.ParseDuration(step.Timeout); err != nil {
					return fmt.Errorf("route[%d] (%s) step[%d]: invalid timeout format: %w", i, route.Name, j, err)
				}
			}
			// Validate conflict_resolution
			if step.ConflictResolution != "" {
				if step.ConflictResolution != "tools" && step.ConflictResolution != "format" {
					return fmt.Errorf("route[%d] (%s) step[%d]: conflict_resolution must be 'tools' or 'format', got '%s'", i, route.Name, j, step.ConflictResolution)
				}
			}
			cfg.Routes[i].Steps[j] = step
		}
		cfg.Routes[i] = route
	}

	return nil
}