// Package config loads Transfer Scout runtime configuration from
// environment variables (see .env.example).
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	DatabaseURL        string
	APIPort            string
	IngestPollInterval time.Duration
	ExtractModel       string
	ExtractAPIKey      string
	TransferWindow     string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: getEnv("DATABASE_URL", ""),
		// Cloud Run injects PORT and requires the container to listen on
		// it; API_PORT is the local-dev override, falling back to 8080.
		APIPort:        getEnv("PORT", getEnv("API_PORT", "8080")),
		ExtractModel:   getEnv("EXTRACT_MODEL", "claude-haiku-4-5-20251001"),
		ExtractAPIKey:  getEnv("EXTRACT_API_KEY", ""),
		TransferWindow: getEnv("TRANSFER_WINDOW", "summer-2026"),
	}

	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("config: DATABASE_URL is required")
	}

	pollIntervalStr := getEnv("INGEST_POLL_INTERVAL", "5m")
	interval, err := time.ParseDuration(pollIntervalStr)
	if err != nil {
		return cfg, fmt.Errorf("config: invalid INGEST_POLL_INTERVAL %q: %w", pollIntervalStr, err)
	}
	cfg.IngestPollInterval = interval

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
