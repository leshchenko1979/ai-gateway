package config

import (
	"time"
)

// Config represents the gateway configuration
type Config struct {
	APIKey              string     `yaml:"api_key"`
	Port                int        `yaml:"port"`
	DefaultStepTimeout  string     `yaml:"default_step_timeout,omitempty"`
	DefaultRouteTimeout string     `yaml:"default_route_timeout,omitempty"`
	Providers           []Provider `yaml:"providers"`
	Routes              []Route    `yaml:"routes"`
	EnvVars             []string   `yaml:"-"`
}

const (
	fallbackStepTimeout      = 30 * time.Second
	minHTTPServerTimeout     = 30 * time.Second
	defaultRouteTimeoutValue = 30 * time.Second
)

// Provider represents a single AI provider configuration
type Provider struct {
	Name    string `yaml:"name"`
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

// Route represents a route configuration that matches incoming request models
type Route struct {
	Name         string      `yaml:"name"`
	RouteTimeout string      `yaml:"route_timeout,omitempty"`
	Steps        []RouteStep `yaml:"steps"`
}

// RouteStep represents a single step in a route
type RouteStep struct {
	Provider           string `yaml:"provider"`
	Model              string `yaml:"model"`
	StepTimeout        string `yaml:"step_timeout,omitempty"`
	ConflictResolution string `yaml:"conflict_resolution,omitempty"`
}

// GetStepTimeout returns the timeout as a time.Duration for a route step.
func GetStepTimeout(stepTimeout, defaultStepTimeout string) time.Duration {
	if stepTimeout == "" {
		stepTimeout = defaultStepTimeout
	}
	if stepTimeout == "" {
		return fallbackStepTimeout
	}

	duration, err := time.ParseDuration(stepTimeout)
	if err != nil {
		return fallbackStepTimeout
	}
	return duration
}

// GetRouteTimeout resolves the effective timeout for a route.
func (c *Config) GetRouteTimeout(route Route) time.Duration {
	if route.RouteTimeout != "" {
		if d, err := time.ParseDuration(route.RouteTimeout); err == nil {
			return d
		}
	}
	if c.DefaultRouteTimeout != "" {
		if d, err := time.ParseDuration(c.DefaultRouteTimeout); err == nil {
			return d
		}
	}

	derived := c.GetRouteDerivedTimeout(route)
	if derived > 0 {
		return derived
	}
	return defaultRouteTimeoutValue
}

// GetRouteDerivedTimeout returns the sum of step timeouts for a route.
func (c *Config) GetRouteDerivedTimeout(route Route) time.Duration {
	total := time.Duration(0)
	for _, step := range route.Steps {
		total += GetStepTimeout(step.StepTimeout, c.DefaultStepTimeout)
	}
	return total
}

// MaxSequentialRouteDuration returns the maximum sum of step timeouts across routes.
func (c *Config) MaxSequentialRouteDuration() time.Duration {
	maxDuration := time.Duration(0)
	for _, route := range c.Routes {
		routeTotal := c.GetRouteDerivedTimeout(route)
		if routeTotal > maxDuration {
			maxDuration = routeTotal
		}
	}
	return routeTotalOrFallback(maxDuration)
}

// MaxEffectiveRouteTimeout returns the largest effective route timeout.
func (c *Config) MaxEffectiveRouteTimeout() time.Duration {
	maxDuration := time.Duration(0)
	for _, route := range c.Routes {
		effective := c.GetRouteTimeout(route)
		if effective > maxDuration {
			maxDuration = effective
		}
	}
	return routeTotalOrFallback(maxDuration)
}

// EffectiveHTTPServerTimeouts resolves read and write timeouts for the HTTP server.
func (c *Config) EffectiveHTTPServerTimeouts() (time.Duration, time.Duration) {
	// Keep server timeouts at least as high as the maximum effective route budget
	// so true route timeout errors can surface instead of premature server cutoff.
	derived := c.MaxSequentialRouteDuration()
	routeBudget := c.MaxEffectiveRouteTimeout()
	effective := clampHTTPServerTimeout(maxDuration(derived, routeBudget))
	return effective, effective
}

func routeTotalOrFallback(d time.Duration) time.Duration {
	if d <= 0 {
		return minHTTPServerTimeout
	}
	return d
}

func clampHTTPServerTimeout(d time.Duration) time.Duration {
	if d < minHTTPServerTimeout {
		return minHTTPServerTimeout
	}
	return d
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
