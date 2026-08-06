// Package config loads Transfer Scout runtime configuration from
// environment variables (see .env.example).
package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL    string
	APIPort        string
	ExtractModel   string
	ExtractAPIKey  string
	TransferWindow string
	// AuthJWTSecret and GoogleClientID are only used by cmd/api (not
	// cmd/ingest/cmd/extract, which also call Load()) — so they're NOT
	// validated here; cmd/api checks them itself right after Load(),
	// the same fail-fast spirit as DatabaseURL without breaking the
	// other two binaries, which never set or need them.
	AuthJWTSecret  string
	GoogleClientID string
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
		AuthJWTSecret:  getEnv("AUTH_JWT_SECRET", ""),
		GoogleClientID: getEnv("GOOGLE_CLIENT_ID", ""),
	}

	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("config: DATABASE_URL is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
