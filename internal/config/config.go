// Package config loads Transfer Scout runtime configuration from
// environment variables (see .env.example).
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL   string
	APIPort       string
	ExtractModel  string
	ExtractAPIKey string
	// ExtractMinConfidence is the floor an extraction's self-reported
	// field confidence must clear before it is stored. Provisional: the
	// value that belongs here has to come from measuring a labelled set
	// (see extract/testdata), not from taste. The is_transfer_rumour
	// boolean, not this number, is what keeps non-rumours out.
	ExtractMinConfidence float64
	TransferWindow       string
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
		APIPort:              getEnv("PORT", getEnv("API_PORT", "8080")),
		ExtractModel:         getEnv("EXTRACT_MODEL", "claude-haiku-4-5-20251001"),
		ExtractAPIKey:        getEnv("EXTRACT_API_KEY", ""),
		ExtractMinConfidence: getEnvFloat("EXTRACT_MIN_CONFIDENCE", defaultExtractMinConfidence),
		TransferWindow:       getEnv("TRANSFER_WINDOW", "summer-2026"),
		AuthJWTSecret:        getEnv("AUTH_JWT_SECRET", ""),
		GoogleClientID:       getEnv("GOOGLE_CLIENT_ID", ""),
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

// defaultExtractMinConfidence is low on purpose. The classification boolean
// is what rejects non-rumours; this floor only drops rumours whose details
// the model itself distrusts, and setting it high without having measured a
// labelled set would silently discard real rumours to fix a problem it was
// never the cause of.
const defaultExtractMinConfidence = 0.4

// getEnvFloat falls back rather than failing on a malformed value: an
// unparseable threshold should not stop the extract job from running, and
// the fallback is the documented default either way.
func getEnvFloat(key string, fallback float64) float64 {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 || v > 1 {
		return fallback
	}
	return v
}
