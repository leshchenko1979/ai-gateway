package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/providers"
)

// Server represents the HTTP server
type Server struct {
	config   *config.Config
	manager  *providers.Manager
	logger   *logger.Logger
	httpSrv  *http.Server
}

// NewServer creates a new server instance
func NewServer(cfg *config.Config, logger *logger.Logger, manager *providers.Manager) *Server {
	srv := &Server{
		config:  cfg,
		logger:  logger,
		manager: manager,
	}

	mux := srv.setupRoutes()
	srv.httpSrv = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return srv
}

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Health endpoint (no auth required)
	mux.HandleFunc("/health", s.handleHealth)

	// Protected endpoints
	mux.HandleFunc("/v1/models", s.authMiddleware(s.handleModels))
	mux.HandleFunc("/v1/chat/completions", s.authMiddleware(s.handleChatCompletions))

	return mux
}

// Start starts the HTTPS server
func (s *Server) Start() error {
	s.logger.Info("Starting server", map[string]interface{}{
		"port": s.config.Port,
		"providers": len(s.config.Providers),
	})

	// For now, start HTTP server. In production, this should be HTTPS
	return s.httpSrv.ListenAndServe()
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}