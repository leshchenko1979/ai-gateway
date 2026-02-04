package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/providers"
)

func TestAuthMiddleware(t *testing.T) {
	cfg := &config.Config{
		APIKey: "test-api-key",
		Port:   8080,
	}
	logger := logger.NewLogger()
	manager := providers.NewManager([]config.Provider{}, []config.Route{}, logger)
	srv := NewServer(cfg, logger, manager)

	tests := []struct {
		name           string
		apiKey         string
		authHeader     string
		path           string
		expectedStatus int
	}{
		{
			name:           "valid X-Api-Key",
			apiKey:         "test-api-key",
			path:           "/v1/models",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "valid Authorization Bearer",
			authHeader:     "Bearer test-api-key",
			path:           "/v1/models",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing auth",
			path:           "/v1/models",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid api key",
			apiKey:         "wrong-key",
			path:           "/v1/models",
			expectedStatus: http.StatusUnauthorized,
		},
		// Note: health endpoint doesn't go through auth middleware
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.apiKey != "" {
				req.Header.Set("X-Api-Key", tt.apiKey)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			handler := srv.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			handler(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}