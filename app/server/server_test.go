package server

import (
	"testing"
	"time"

	"ai-gateway/config"
	"ai-gateway/logger"
)

func TestNewServer_UsesEffectiveHTTPServerTimeouts(t *testing.T) {
	cfg := &config.Config{
		APIKey:              "test-key",
		Port:                8080,
		DefaultRouteTimeout: "5m",
		Routes: []config.Route{
			{
				Name: "test-route",
				Steps: []config.RouteStep{
					{Provider: "p1", Model: "m1", StepTimeout: "5m"},
				},
			},
		},
	}
	log := logger.NewLogger()
	manager := mustNewProviderManager(t, cfg, log)
	srv := mustNewServer(t, cfg, log, manager)

	if srv.httpSrv.ReadTimeout != 5*time.Minute {
		t.Fatalf("ReadTimeout = %v, want %v", srv.httpSrv.ReadTimeout, 5*time.Minute)
	}
	if srv.httpSrv.WriteTimeout != 5*time.Minute {
		t.Fatalf("WriteTimeout = %v, want %v", srv.httpSrv.WriteTimeout, 5*time.Minute)
	}
}

func TestNewServer_NilInputs(t *testing.T) {
	log := logger.NewLogger()
	cfg := &config.Config{Port: 8080}
	manager := mustNewProviderManager(t, &config.Config{Providers: []config.Provider{}, Routes: []config.Route{}}, log)

	if _, err := NewServer(nil, log, manager); err == nil {
		t.Fatal("expected error for nil config")
	}
	if _, err := NewServer(cfg, nil, manager); err == nil {
		t.Fatal("expected error for nil logger")
	}
	if _, err := NewServer(cfg, log, nil); err == nil {
		t.Fatal("expected error for nil manager")
	}
}
