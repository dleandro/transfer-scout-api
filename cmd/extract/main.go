// Command extract pulls a batch of unprocessed articles, extracts
// structured rumour data from each via the model configured by
// EXTRACT_MODEL/EXTRACT_API_KEY, upserts a rumour + timeline event for
// every usable extraction, and marks the article processed either way.
// It is a one-shot batch job, not a ticker loop — run it on a schedule
// (cron, systemd timer, etc). See internal/extract for the model
// contract and internal/cluster for the upsert logic.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/cluster"
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
	clusterer := cluster.New(s)

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

	var extracted, clustered, failed, skipped int
	interrupted := false
	for i, article := range articles {
		if ctx.Err() != nil {
			interrupted = true
			skipped = len(articles) - i
			break
		}

		result, err := extractOne(ctx, extractor, article)
		var extractionJSON []byte
		if err != nil {
			failed++
			slog.Warn("extract: article extraction failed", "article_id", article.ID, "url", article.URL, "error", err)
		} else {
			extracted++
			if extractionJSON, err = json.Marshal(result); err != nil {
				slog.Error("extract: marshal result", "article_id", article.ID, "error", err)
				extractionJSON = nil
			}

			rumourID, err := clusterer.Upsert(ctx, article.ID, article.SourceID, result, cfg.TransferWindow)
			if err != nil {
				slog.Error("extract: cluster upsert failed", "article_id", article.ID, "error", err)
			} else if rumourID != uuid.Nil {
				clustered++
			}
		}

		if err := s.MarkExtracted(ctx, article.ID, extractionJSON); err != nil {
			slog.Error("extract: mark processed", "article_id", article.ID, "error", err)
		}
	}

	if interrupted {
		slog.Warn("extract: interrupted", "extracted", extracted, "clustered", clustered, "failed", failed, "skipped", skipped, "total", len(articles))
		os.Exit(1)
	}

	slog.Info("extract: batch complete", "extracted", extracted, "clustered", clustered, "failed", failed, "total", len(articles))
}

// extractOne calls the model for a single article and returns the parsed
// result.
func extractOne(ctx context.Context, extractor extract.Extractor, article models.Article) (extract.Result, error) {
	text := article.Title
	if article.Content != nil && *article.Content != "" {
		text += "\n\n" + *article.Content
	}
	return extractor.Extract(ctx, text)
}
