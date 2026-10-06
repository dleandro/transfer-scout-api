// Command pipeline is the scheduled production job: it runs ingest, then
// extract, in one process, and exits. Extract always runs — its input is
// the unprocessed-article queue, not what this ingest found — and a failure
// in either stage makes the process exit non-zero so the Cloud Run Job
// execution shows as failed. See internal/pipeline.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dleandro/transfer-scout-api/internal/config"
	"github.com/dleandro/transfer-scout-api/internal/db"
	"github.com/dleandro/transfer-scout-api/internal/ingest"
	"github.com/dleandro/transfer-scout-api/internal/pipeline"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	s := store.New(pool)
	deps, err := pipeline.NewExtractDeps(cfg, s)
	if err != nil {
		slog.Error("extract: build extractor", "error", err)
		pool.Close()
		os.Exit(1)
	}

	if err := pipeline.Run(ctx, ingest.NewPoller(s), deps); err != nil {
		slog.Error("pipeline: failed", "error", err)
		pool.Close()
		os.Exit(1)
	}
}
