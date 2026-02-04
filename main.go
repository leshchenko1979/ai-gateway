// AI Gateway - Lightweight OpenAI-compatible API gateway
package main

import (
	"fmt"
	"log"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/providers"
	"ai-gateway/server"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create logger and provider manager
	logger := logger.NewLogger()
	manager := providers.NewManager(cfg.Providers, cfg.Routes, logger)

	// Create and start server
	srv := server.NewServer(cfg, logger, manager)
	fmt.Printf("Starting AI Gateway on port %d\n", cfg.Port)

	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}