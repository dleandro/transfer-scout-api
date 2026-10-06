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
	// ExtractMaxArticlesPerRun bounds how many articles one extract run
	// sends to the model. The run drains the queue in batches until it is
	// empty or this cap is reached, so it is the per-run cost and duration
	// ceiling — and a backlog bigger than it is logged as a warning.
	ExtractMaxArticlesPerRun int
	// ExtractBaseURL overrides the Anthropic Messages endpoint. Empty means
	// the real API; it exists for local smoke runs against a fake server.
	ExtractBaseURL string
	// TypesafeAPIKey, JevModel and JevMinProbability configure the Jev
	// classifier that decides whether an article is a transfer rumour
	// before Claude is paid to extract it (see extract.GatedExtractor).
	TypesafeAPIKey string
	// JevModel is pinned to a version, not jev-latest: JevMinProbability
	// is only meaningful for the version it was measured against.
	JevModel          string
	JevMinProbability float64
	// JevBaseURL overrides the TypeSafe endpoint; empty means the real API.
	JevBaseURL     string
	TransferWindow string
	// AuthJWTSecret and GoogleClientID are only used by cmd/api (not
	// cmd/ingest/cmd/extract/cmd/pipeline, which also call Load()) — so
	// they're NOT validated here; cmd/api checks them itself right after
	// Load(), the same fail-fast spirit as DatabaseURL without breaking the
	// worker binaries, which never set or need them.
	AuthJWTSecret  string
	GoogleClientID string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: getEnv("DATABASE_URL", ""),
		// Cloud Run injects PORT and requires the container to listen on
		// it; API_PORT is the local-dev override, falling back to 8080.
		APIPort:                  getEnv("PORT", getEnv("API_PORT", "8080")),
		ExtractModel:             getEnv("EXTRACT_MODEL", "claude-haiku-4-5-20251001"),
		ExtractAPIKey:            getEnv("EXTRACT_API_KEY", ""),
		ExtractMaxArticlesPerRun: getEnvPositiveInt("EXTRACT_MAX_ARTICLES_PER_RUN", defaultExtractMaxArticlesPerRun),
		ExtractBaseURL:           getEnv("EXTRACT_BASE_URL", ""),
		TypesafeAPIKey:           getEnv("TYPESAFE_API_KEY", ""),
		JevModel:                 getEnv("JEV_MODEL", DefaultJevModel),
		JevMinProbability:        getEnvFloat("JEV_MIN_PROBABILITY", DefaultJevMinProbability),
		JevBaseURL:               getEnv("TYPESAFE_BASE_URL", ""),
		TransferWindow:           getEnv("TRANSFER_WINDOW", "summer-2026"),
		AuthJWTSecret:            getEnv("AUTH_JWT_SECRET", ""),
		GoogleClientID:           getEnv("GOOGLE_CLIENT_ID", ""),
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

// DefaultJevModel is pinned to a version, not jev-latest: a threshold
// measured against one version says nothing about the next, and the alias
// moves without a change on our side.
const DefaultJevModel = "jev-1.13.0"

// DefaultJevMinProbability is a placeholder, not a measurement: 0.5 is
// simply where Jev's calibrated probability says "more likely yes than no".
// Replace it with a value read off the eval's threshold sweep. Exported so
// the eval reports at the same default the extract job runs with.
const DefaultJevMinProbability = 0.5

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

// defaultExtractMaxArticlesPerRun covers a week's ingest (~290 articles on
// the first production run) plus room to work down a backlog, while still
// bounding what a single run can spend on model calls.
const defaultExtractMaxArticlesPerRun = 500

// getEnvPositiveInt falls back on a malformed or non-positive value for the
// same reason as getEnvFloat: a bad cap should not stop the job running.
func getEnvPositiveInt(key string, fallback int) int {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
