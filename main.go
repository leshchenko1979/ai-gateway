// AI Gateway - Lightweight OpenAI-compatible API gateway
package main

import (
	"context"
	"fmt"
	"log"

	"ai-gateway/config"
	"ai-gateway/logger"
	"ai-gateway/providers"
	"ai-gateway/server"
	"ai-gateway/telemetry"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Configure observability (tracing/logging)
	shutdown, err := telemetry.Init(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize telemetry: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			log.Printf("Telemetry shutdown failed: %v", err)
		}
	}()

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
