package server

import (
	"net/http"
	"strings"
)

// authMiddleware validates API key authentication
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health endpoint
		if r.URL.Path == "/health" {
			next(w, r)
			return
		}

		// Check X-Api-Key header
		apiKey := r.Header.Get("X-Api-Key")

		// If not found, check Authorization header
		if apiKey == "" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				apiKey = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		// Validate API key
		if apiKey == "" || apiKey != s.config.APIKey {
			s.logger.Error("Authentication failed", nil, map[string]interface{}{
				"path": r.URL.Path,
				"has_key": apiKey != "",
			})
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Call next handler
		next(w, r)
	}
}