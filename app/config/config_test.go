package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadConfigMissingEnvVar(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configData := `
api_key: ${GATEWAY_API_KEY}
providers:
  - name: test
    api_key: ${MISSING_PROVIDER_KEY}
    base_url: https://example.com
routes:
  - name: test-route
    steps:
      - provider: test
        model: test-model
`
	if err := os.WriteFile(configPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	os.Setenv("GATEWAY_API_KEY", "test-gateway-key")
	defer os.Unsetenv("GATEWAY_API_KEY")
	os.Unsetenv("MISSING_PROVIDER_KEY")

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected error for missing environment variable, got nil")
	}
	if !strings.Contains(err.Error(), "MISSING_PROVIDER_KEY") {
		t.Fatalf("expected missing env var name in error, got: %v", err)
	}
}

func TestLoadConfig_UnknownFieldsFail(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configData := `
api_key: ${GATEWAY_API_KEY}
default_timeout: 30s
providers:
  - name: test
    api_key: ${PROVIDER_API_KEY}
    base_url: https://example.com
routes:
  - name: test-route
    steps:
      - provider: test
        model: test-model
        timeout: 10s
`
	if err := os.WriteFile(configPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	os.Setenv("GATEWAY_API_KEY", "test-gateway-key")
	os.Setenv("PROVIDER_API_KEY", "test-provider-key")
	defer func() {
		os.Unsetenv("GATEWAY_API_KEY")
		os.Unsetenv("PROVIDER_API_KEY")
	}()

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected error for unknown legacy timeout fields, got nil")
	}
	if !strings.Contains(err.Error(), "field default_timeout not found") {
		t.Fatalf("expected unknown field error for default_timeout, got: %v", err)
	}
}

func TestFindEnvVars(t *testing.T) {
	configData := `
api_key: ${GATEWAY_API_KEY}
providers:
  - name: test
    api_key: ${PROVIDER_API_KEY}
    base_url: https://example.com
routes:
  - name: test-route
    steps:
      - provider: test
        model: test-model
`

	vars := findEnvVars(configData)
	if len(vars) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(vars))
	}
	if vars[0] != "GATEWAY_API_KEY" || vars[1] != "PROVIDER_API_KEY" {
		t.Fatalf("unexpected env vars: %v", vars)
	}
}

func TestGetStepTimeout(t *testing.T) {
	if got := GetStepTimeout("30s", "60s"); got != 30*time.Second {
		t.Fatalf("GetStepTimeout(\"30s\") = %v, want 30s", got)
	}
	if got := GetStepTimeout("", "60s"); got != 60*time.Second {
		t.Fatalf("GetStepTimeout(\"\", \"60s\") = %v, want 60s", got)
	}
	if got := GetStepTimeout("", ""); got != 30*time.Second {
		t.Fatalf("GetStepTimeout(\"\", \"\") = %v, want 30s fallback", got)
	}
	if got := GetStepTimeout("nope", "60s"); got != 30*time.Second {
		t.Fatalf("GetStepTimeout(\"nope\", \"60s\") = %v, want 30s fallback", got)
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
				APIKey:              "test-key",
				DefaultRouteTimeout: "30s",
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

func TestMaxSequentialRouteDuration(t *testing.T) {
	cfg := &Config{
		DefaultRouteTimeout: "45s",
		Routes: []Route{
			{
				Name: "short",
				Steps: []RouteStep{
					{Provider: "p1", Model: "m1", StepTimeout: "10s"},
					{Provider: "p2", Model: "m2"},
				},
			},
			{
				Name: "long",
				Steps: []RouteStep{
					{Provider: "p1", Model: "m1", StepTimeout: "2m"},
					{Provider: "p2", Model: "m2", StepTimeout: "30s"},
				},
			},
		},
	}

	got := cfg.MaxSequentialRouteDuration()
	want := 150 * time.Second
	if got != want {
		t.Fatalf("MaxSequentialRouteDuration() = %v, want %v", got, want)
	}
}

func TestGetRouteTimeoutPrecedence(t *testing.T) {
	cfg := &Config{
		DefaultRouteTimeout: "90s",
		Routes: []Route{
			{
				Name:         "with-override",
				RouteTimeout: "40s",
				Steps: []RouteStep{
					{Provider: "p1", Model: "m1", StepTimeout: "5s"},
				},
			},
			{
				Name: "with-default",
				Steps: []RouteStep{
					{Provider: "p1", Model: "m1", StepTimeout: "5s"},
				},
			},
		},
	}
	if got := cfg.GetRouteTimeout(cfg.Routes[0]); got != 40*time.Second {
		t.Fatalf("route override precedence failed: got %v", got)
	}
	if got := cfg.GetRouteTimeout(cfg.Routes[1]); got != 90*time.Second {
		t.Fatalf("global default precedence failed: got %v", got)
	}
}

func TestGetRouteTimeoutFallbackToDerived(t *testing.T) {
	cfg := &Config{
		Routes: []Route{
			{
				Name: "derived",
				Steps: []RouteStep{
					{Provider: "p1", Model: "m1", StepTimeout: "20s"},
					{Provider: "p2", Model: "m2", StepTimeout: "15s"},
				},
			},
		},
	}
	if got := cfg.GetRouteTimeout(cfg.Routes[0]); got != 35*time.Second {
		t.Fatalf("GetRouteTimeout() fallback derived = %v, want 35s", got)
	}
}

func TestEffectiveHTTPServerTimeouts_UsesMaxEffectiveRouteBudget(t *testing.T) {
	cfg := &Config{
		DefaultRouteTimeout: "10m",
		Routes: []Route{
			{
				Name: "route-a",
				Steps: []RouteStep{
					{Provider: "p1", Model: "m1"}, // falls back to 30s
					{Provider: "p2", Model: "m2"}, // falls back to 30s
				},
			},
		},
	}

	gotRead, gotWrite := cfg.EffectiveHTTPServerTimeouts()
	if gotRead != 10*time.Minute || gotWrite != 10*time.Minute {
		t.Fatalf("EffectiveHTTPServerTimeouts() = (%v, %v), want (%v, %v)", gotRead, gotWrite, 10*time.Minute, 10*time.Minute)
	}
}

func TestEffectiveHTTPServerTimeouts(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *Config
		wantRead  time.Duration
		wantWrite time.Duration
	}{
		{
			name: "derived from worst route sum",
			cfg: &Config{
				Routes: []Route{
					{
						Name: "route-a",
						Steps: []RouteStep{
							{Provider: "p1", Model: "m1", StepTimeout: "10s"},
							{Provider: "p2", Model: "m2"},
						},
					},
					{
						Name: "route-b",
						Steps: []RouteStep{
							{Provider: "p1", Model: "m1", StepTimeout: "40s"},
							{Provider: "p2", Model: "m2"},
						},
					},
				},
			},
			wantRead:  70 * time.Second,
			wantWrite: 70 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRead, gotWrite := tt.cfg.EffectiveHTTPServerTimeouts()
			if gotRead != tt.wantRead || gotWrite != tt.wantWrite {
				t.Fatalf("EffectiveHTTPServerTimeouts() = (%v, %v), want (%v, %v)", gotRead, gotWrite, tt.wantRead, tt.wantWrite)
			}
		})
	}
}

func TestValidateConfig_InvalidDefaultRouteTimeout(t *testing.T) {
	cfg := &Config{
		APIKey:              "test-key",
		DefaultRouteTimeout: "abc",
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
	}

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid default_route_timeout, got nil")
	}
	if !strings.Contains(err.Error(), "default_route_timeout") {
		t.Fatalf("expected error to mention default_route_timeout, got: %v", err)
	}
}

func TestValidateConfig_InvalidDefaultStepTimeout(t *testing.T) {
	cfg := &Config{
		APIKey:             "test-key",
		DefaultStepTimeout: "abc",
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
	}
	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid default_step_timeout, got nil")
	}
	if !strings.Contains(err.Error(), "default_step_timeout") {
		t.Fatalf("expected error to mention default_step_timeout, got: %v", err)
	}
}

func TestValidateConfig_InvalidRouteAndStepTimeout(t *testing.T) {
	cfgRoute := &Config{
		APIKey: "test-key",
		Providers: []Provider{
			{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
		},
		Routes: []Route{
			{
				Name:         "test-model",
				RouteTimeout: "xyz",
				Steps: []RouteStep{
					{Provider: "test", Model: "gpt-4"},
				},
			},
		},
	}
	if err := validateConfig(cfgRoute); err == nil || !strings.Contains(err.Error(), "route_timeout") {
		t.Fatalf("expected route_timeout validation error, got %v", err)
	}

	cfgStep := &Config{
		APIKey: "test-key",
		Providers: []Provider{
			{Name: "test", APIKey: "key", BaseURL: "http://test.com"},
		},
		Routes: []Route{
			{
				Name: "test-model",
				Steps: []RouteStep{
					{Provider: "test", Model: "gpt-4", StepTimeout: "xyz"},
				},
			},
		},
	}
	if err := validateConfig(cfgStep); err == nil || !strings.Contains(err.Error(), "step_timeout") {
		t.Fatalf("expected step_timeout validation error, got %v", err)
	}
}

func TestValidateTimeoutSettings(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{
			name: "valid timeouts",
			cfg: &Config{
				DefaultStepTimeout:  "20s",
				DefaultRouteTimeout: "2m",
				Routes: []Route{
					{
						Name:         "ok",
						RouteTimeout: "90s",
						Steps: []RouteStep{
							{Provider: "p1", Model: "m1", StepTimeout: "5s"},
						},
					},
				},
			},
		},
		{
			name:    "invalid default route timeout",
			cfg:     &Config{DefaultRouteTimeout: "bad"},
			wantErr: "default_route_timeout",
		},
		{
			name:    "invalid default step timeout",
			cfg:     &Config{DefaultStepTimeout: "bad"},
			wantErr: "default_step_timeout",
		},
		{
			name: "invalid route timeout",
			cfg: &Config{
				Routes: []Route{{Name: "r1", RouteTimeout: "bad"}},
			},
			wantErr: "route_timeout",
		},
		{
			name: "invalid step timeout",
			cfg: &Config{
				Routes: []Route{
					{
						Name:  "r1",
						Steps: []RouteStep{{Provider: "p1", Model: "m1", StepTimeout: "bad"}},
					},
				},
			},
			wantErr: "step_timeout",
		},
		{
			name:    "non-positive default route timeout",
			cfg:     &Config{DefaultRouteTimeout: "0s"},
			wantErr: "duration must be > 0",
		},
		{
			name:    "non-positive default step timeout",
			cfg:     &Config{DefaultStepTimeout: "-1s"},
			wantErr: "duration must be > 0",
		},
		{
			name: "non-positive route timeout",
			cfg: &Config{
				Routes: []Route{{Name: "r1", RouteTimeout: "0s"}},
			},
			wantErr: "duration must be > 0",
		},
		{
			name: "non-positive step timeout",
			cfg: &Config{
				Routes: []Route{
					{
						Name:  "r1",
						Steps: []RouteStep{{Provider: "p1", Model: "m1", StepTimeout: "-1s"}},
					},
				},
			},
			wantErr: "duration must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeoutSettings(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateTimeoutSettings() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateTimeoutSettings() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEffectiveHTTPServerTimeouts_DerivedFloorAndLargeBudget(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want time.Duration
	}{
		{
			name: "floor applies for tiny derived sum",
			cfg: &Config{
				Routes: []Route{
					{
						Name: "tiny",
						Steps: []RouteStep{
							{Provider: "p1", Model: "m1", StepTimeout: "1s"},
						},
					},
				},
			},
			want: 30 * time.Second,
		},
		{
			name: "large derived sum is preserved",
			cfg: &Config{
				Routes: []Route{
					{
						Name: "huge",
						Steps: []RouteStep{
							{Provider: "p1", Model: "m1", StepTimeout: "25h"},
						},
					},
				},
			},
			want: 25 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRead, gotWrite := tt.cfg.EffectiveHTTPServerTimeouts()
			if gotRead != tt.want || gotWrite != tt.want {
				t.Fatalf("EffectiveHTTPServerTimeouts() = (%v, %v), want (%v, %v)", gotRead, gotWrite, tt.want, tt.want)
			}
		})
	}
}
