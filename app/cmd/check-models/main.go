package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"ai-gateway/config"
	"ai-gateway/providers"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	results := providers.CheckUpstreamModels(context.Background(), cfg.Providers)
	fail := false
	for _, r := range results {
		if !r.OK {
			fmt.Printf("%s: %s (%dms)\n", r.Provider, r.Error, r.ResponseTimeMs)
			fail = true
			continue
		}
		fmt.Printf("%s: OK (%d models, %dms)\n", r.Provider, r.ModelCount, r.ResponseTimeMs)
	}
	if fail {
		os.Exit(1)
	}
}
