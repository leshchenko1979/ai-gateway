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
	"ai-gateway/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Server represents the HTTP server
type Server struct {
	config  *config.Config
	manager *providers.Manager
	logger  *logger.Logger
	httpSrv *http.Server
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

	return s.instrument(mux)
}

func (s *Server) instrument(next http.Handler) http.Handler {
	tracer := telemetry.Tracer("ai-gateway.server")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), fmt.Sprintf("http.%s", r.URL.Path),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", r.URL.Path),
			),
		)
		defer span.End()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Start starts the HTTPS server
func (s *Server) Start() error {
	s.logger.Info("Starting server", map[string]interface{}{
		"port":       s.config.Port,
		"providers":  len(s.config.Providers),
		"routes":     len(s.config.Routes),
		"route_names": func(routes []config.Route) []string {
			names := make([]string, 0, len(routes))
			for _, route := range routes {
				names = append(names, route.Name)
			}
			return names
		}(s.config.Routes),
		"env_vars": s.config.EnvVars,
	})

	// For now, start HTTP server. In production, this should be HTTPS
	return s.httpSrv.ListenAndServe()
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}
