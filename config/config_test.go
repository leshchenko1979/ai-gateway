package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Set up test environment variables
	os.Setenv("GATEWAY_API_KEY", "test-gateway-key")
	os.Setenv("PROVIDER1_API_KEY", "test-provider1-key")
	os.Setenv("PROVIDER2_API_KEY", "test-provider2-key")
	defer func() {
		os.Unsetenv("GATEWAY_API_KEY")
		os.Unsetenv("PROVIDER1_API_KEY")
		os.Unsetenv("PROVIDER2_API_KEY")
	}()

	// Try to load from test directory first, then fallback to main config
	cfg, err := LoadConfig("test/config.yaml")
	if err != nil {
		// Fallback to main config if test file doesn't exist
		cfg, err = LoadConfig("config.yaml")
		if err != nil {
			t.Skipf("Skipping test - no config file available: %v", err)
			return
		}
	}

	if cfg.APIKey != "test-gateway-key" {
		t.Errorf("Expected API key 'test-gateway-key', got '%s'", cfg.APIKey)
	}

	if len(cfg.Providers) == 0 {
		t.Error("Expected at least one provider")
	}

	if cfg.Providers[0].Name == "" {
		t.Error("Provider name is required")
	}

	if len(cfg.Routes) == 0 {
		t.Error("Expected at least one route")
	}

	if cfg.Routes[0].Name == "" {
		t.Error("Route name is required")
	}
}

func TestGetTimeout(t *testing.T) {
	// Test step timeout provided
	timeout := GetTimeout("30s", "60s")
	if timeout != 30*time.Second {
		t.Errorf("Expected 30s, got %v", timeout)
	}

	// Test default timeout used when step timeout is empty
	timeout2 := GetTimeout("", "60s")
	if timeout2 != 60*time.Second {
		t.Errorf("Expected default 60s, got %v", timeout2)
	}

	// Test fallback to 30s when both are empty
	timeout3 := GetTimeout("", "")
	if timeout3 != 30*time.Second {
		t.Errorf("Expected fallback 30s, got %v", timeout3)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				APIKey: "test-key",
				DefaultTimeout: "30s",
				Providers: []Provider{
					{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
				},
				Routes: []Route{
					{
						Name: "test-model",
						Steps: []RouteStep{
							{Provider: "test", Model: "gpt-4"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing api_key",
			config: &Config{
				Providers: []Provider{
					{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
				},
				Routes: []Route{
					{
						Name: "test-model",
						Steps: []RouteStep{
							{Provider: "test", Model: "gpt-4"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing providers",
			config: &Config{
				APIKey:    "test-key",
				Providers: []Provider{},
				Routes: []Route{
					{
						Name: "test-model",
						Steps: []RouteStep{
							{Provider: "test", Model: "gpt-4"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "route missing steps",
			config: &Config{
				APIKey: "test-key",
				Providers: []Provider{
					{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
				},
				Routes: []Route{
					{Name: "test-model", Steps: []RouteStep{}},
				},
			},
			wantErr: true,
		},
		{
			name: "route step invalid provider reference",
			config: &Config{
				APIKey: "test-key",
				Providers: []Provider{
					{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
				},
				Routes: []Route{
					{
						Name: "test-model",
						Steps: []RouteStep{
							{Provider: "nonexistent", Model: "gpt-4"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid conflict_resolution",
			config: &Config{
				APIKey: "test-key",
				Providers: []Provider{
					{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
				},
				Routes: []Route{
					{
						Name: "test-model",
						Steps: []RouteStep{
							{Provider: "test", Model: "gpt-4", ConflictResolution: "invalid"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "route missing name",
			config: &Config{
				APIKey: "test-key",
				Providers: []Provider{
					{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
				},
				Routes: []Route{
					{
						Name: "",
						Steps: []RouteStep{
							{Provider: "test", Model: "gpt-4"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "route step missing provider",
			config: &Config{
				APIKey: "test-key",
				Providers: []Provider{
					{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
				},
				Routes: []Route{
					{
						Name: "test-model",
						Steps: []RouteStep{
							{Provider: "", Model: "gpt-4"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "route step missing model",
			config: &Config{
				APIKey: "test-key",
				Providers: []Provider{
					{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
				},
				Routes: []Route{
					{
						Name: "test-model",
						Steps: []RouteStep{
							{Provider: "test", Model: ""},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple routes same name",
			config: &Config{
				APIKey: "test-key",
				Providers: []Provider{
					{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
				},
				Routes: []Route{
					{
						Name: "duplicate",
						Steps: []RouteStep{
							{Provider: "test", Model: "gpt-4"},
						},
					},
					{
						Name: "duplicate",
						Steps: []RouteStep{
							{Provider: "test", Model: "claude-3"},
						},
					},
				},
			},
			wantErr: false, // This should actually be allowed - routes can have same name but different steps
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetDefaultTimeout(t *testing.T) {
	tests := []struct {
		name           string
		defaultTimeout string
		expected       time.Duration
	}{
		{
			name:           "explicit timeout",
			defaultTimeout: "60s",
			expected:       60 * time.Second,
		},
		{
			name:           "empty timeout",
			defaultTimeout: "",
			expected:       30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{DefaultTimeout: tt.defaultTimeout}
			result := config.GetDefaultTimeout()
			if result != tt.expected {
				t.Errorf("GetDefaultTimeout() = %v, want %v", result, tt.expected)
			}
		})
	}
}