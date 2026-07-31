// Command extract is the LLM extraction worker. It currently only reports
// how many articles are waiting on extraction — the model call itself is
// not yet implemented. See milestone 1.3 and internal/extract.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/dleandro/transfer-scout-api/internal/config"
	"github.com/dleandro/transfer-scout-api/internal/db"
	"github.com/dleandro/transfer-scout-api/internal/extract"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	s := store.New(pool)

	articles, err := s.ListUnprocessed(ctx, 50)
	if err != nil {
		slog.Error("extract: list unprocessed", "error", err)
		os.Exit(1)
	}

	var extractor extract.Extractor = extract.StubExtractor{}
	slog.Info("extract: worker not yet implemented", "unprocessed_articles", len(articles))

	if len(articles) > 0 {
		if _, err := extractor.Extract(ctx, ""); err != nil {
			slog.Warn("extract: stub extractor call", "error", err)
		}
	}
}
