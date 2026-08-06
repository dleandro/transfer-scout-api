// Command extract pulls a batch of unprocessed articles, extracts
// structured rumour data from each via the model configured by
// EXTRACT_MODEL/EXTRACT_API_KEY, and marks them processed. It is a
// one-shot batch job, not a ticker loop — run it on a schedule (cron,
// systemd timer, etc). See internal/extract for the model contract.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dleandro/transfer-scout-api/internal/config"
	"github.com/dleandro/transfer-scout-api/internal/db"
	"github.com/dleandro/transfer-scout-api/internal/extract"
	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

const batchSize = 50

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

	var extractor extract.Extractor
	if cfg.ExtractAPIKey == "" {
		slog.Warn("extract: EXTRACT_API_KEY not set, using stub extractor — no articles will actually be extracted")
		extractor = extract.StubExtractor{}
	} else {
		extractor = extract.NewAnthropicExtractor(cfg.ExtractAPIKey, cfg.ExtractModel)
	}

	articles, err := s.ListUnprocessed(ctx, batchSize)
	if err != nil {
		slog.Error("extract: list unprocessed", "error", err)
		os.Exit(1)
	}

	slog.Info("extract: starting batch", "articles", len(articles), "model", cfg.ExtractModel)

	var extracted, failed, skipped int
	interrupted := false
	for i, article := range articles {
		if ctx.Err() != nil {
			interrupted = true
			skipped = len(articles) - i
			break
		}

		extractionJSON, err := extractOne(ctx, extractor, article)
		if err != nil {
			failed++
			slog.Warn("extract: article extraction failed", "article_id", article.ID, "url", article.URL, "error", err)
		} else {
			extracted++
		}

		if err := s.MarkExtracted(ctx, article.ID, extractionJSON); err != nil {
			slog.Error("extract: mark processed", "article_id", article.ID, "error", err)
		}
	}

	if interrupted {
		slog.Warn("extract: interrupted", "extracted", extracted, "failed", failed, "skipped", skipped, "total", len(articles))
		os.Exit(1)
	}

	slog.Info("extract: batch complete", "extracted", extracted, "failed", failed, "total", len(articles))
}

// extractOne calls the model for a single article and marshals a
// successful result to JSON for storage. It returns a nil slice (not an
// error) when extraction fails, so the caller still marks the article
// processed.
func extractOne(ctx context.Context, extractor extract.Extractor, article models.Article) ([]byte, error) {
	text := article.Title
	if article.Content != nil && *article.Content != "" {
		text += "\n\n" + *article.Content
	}

	result, err := extractor.Extract(ctx, text)
	if err != nil {
		return nil, err
	}

	extractionJSON, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return extractionJSON, nil
}
