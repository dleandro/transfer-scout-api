// Command extract drains the unprocessed-article queue once: each article
// goes through the model configured by EXTRACT_MODEL/EXTRACT_API_KEY, every
// usable extraction is upserted into a rumour + timeline event, and the
// article is marked processed either way. Production runs cmd/pipeline
// (ingest then extract) instead; this binary is for running the stage on
// its own. See internal/pipeline.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dleandro/transfer-scout-api/internal/config"
	"github.com/dleandro/transfer-scout-api/internal/db"
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

	deps, err := pipeline.NewExtractDeps(cfg, store.New(pool))
	if err != nil {
		slog.Error("extract: build extractor", "error", err)
		pool.Close()
		os.Exit(1)
	}

	if _, err := pipeline.RunExtract(ctx, deps); err != nil {
		slog.Error("extract: failed", "error", err)
		pool.Close()
		os.Exit(1)
	}
}
