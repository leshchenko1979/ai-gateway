package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"ai-gateway/types"
)

// generateRequestID generates a unique request ID
func generateRequestID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{"status": "healthy"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleModels handles model listing requests
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	model := types.Model{
		ID:      "dynamic/model",
		Object:  "model",
		Created: 1677610602,
		OwnedBy: "ai-gateway",
	}

	response := types.ModelsResponse{
		Object: "list",
		Data:   []types.Model{model},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleChatCompletions handles chat completion requests
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	// Generate unique request ID for tracing
	requestID := generateRequestID()

	// Parse request
	var req types.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error("Failed to parse request", err, map[string]interface{}{
			"request_id": requestID,
		})
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate request
	if err := validateChatRequest(&req); err != nil {
		s.logger.Error("Invalid request", err, map[string]interface{}{
			"request_id": requestID,
		})
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Convert request to JSON for logging (with truncated message contents)
	truncatedReq := req.TruncateRequestForLogging()
	requestJSON, _ := json.Marshal(truncatedReq)

	// Extract message count for logging
	var temp struct {
		Messages []interface{} `json:"messages"`
	}
	json.Unmarshal(req.Raw, &temp)
	messageCount := len(temp.Messages)

	// Log request summary with truncated JSON
	s.logger.Info("Chat completion request", map[string]interface{}{
		"request_id":   requestID,
		"model":       req.Model,
		"messages":    messageCount,
		"request_json": string(requestJSON),
	})

	// Execute route for the requested model
	response, err := s.manager.ExecuteWithTracing(req, requestID)
	if err != nil {
		s.logger.Error("Request execution failed", err, map[string]interface{}{
			"request_id": requestID,
			"model":      req.Model,
		})

		// Check if it's a route lookup error (no route found)
		if err.Error() == fmt.Sprintf("route lookup failed: no route found for model '%s'", req.Model) {
			http.Error(w, fmt.Sprintf("No route configured for model '%s'", req.Model), http.StatusNotFound)
			return
		}

		// Check if it's a detailed route error with step information
		if routeErr, ok := err.(types.RouteError); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(routeErr)
			return
		}

		// Fallback for other errors
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}