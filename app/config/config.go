package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

	rawConfig := string(data)
	envVars := findEnvVars(rawConfig)

	// Expand environment variables
	expanded, err := expandEnvVars(rawConfig)
	if err != nil {
		return nil, err
	}

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

	config.EnvVars = envVars

	return &config, nil
}

// expandEnvVars replaces ${VAR_NAME} with environment variable values
func expandEnvVars(s string) (string, error) {
	missing := findMissingEnvVars(s)
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return os.Expand(s, func(key string) string {
		return os.Getenv(key)
	}), nil
}

func findMissingEnvVars(s string) []string {
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	matches := re.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := match[1]
		if os.Getenv(name) == "" {
			seen[name] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil
	}

	missing := make([]string, 0, len(seen))
	for name := range seen {
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return missing
}

func findEnvVars(s string) []string {
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	matches := re.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		seen[match[1]] = struct{}{}
	}

	if len(seen) == 0 {
		return nil
	}

	vars := make([]string, 0, len(seen))
	for name := range seen {
		vars = append(vars, name)
	}
	sort.Strings(vars)
	return vars
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