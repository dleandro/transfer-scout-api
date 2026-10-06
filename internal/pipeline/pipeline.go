// Package pipeline runs the two batch stages behind the worker binaries:
// ingest (poll RSS feeds into articles) and extract (drain the
// unprocessed-article queue through the model and the clusterer). The
// stages communicate only through the articles table's processed flag, so
// extract's input is whatever is queued, not what the preceding ingest
// found.
//
// cmd/pipeline runs both in one process (the scheduled production job);
// cmd/ingest and cmd/extract run one each for local use.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/cluster"
	"github.com/dleandro/transfer-scout-api/internal/config"
	"github.com/dleandro/transfer-scout-api/internal/extract"
	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// batchSize is how many articles one ListUnprocessed call fetches. A run
// keeps fetching batches until the queue is drained or
// ExtractDeps.MaxArticles is reached.
const batchSize = 50

// Poller is the ingest stage; *ingest.Poller satisfies it.
type Poller interface {
	PollOnce(ctx context.Context) error
}

// ExtractStore is the subset of store.Store the extract stage needs.
type ExtractStore interface {
	ListUnprocessed(ctx context.Context, limit int) ([]models.Article, error)
	MarkExtracted(ctx context.Context, id uuid.UUID, extractionJSON []byte) error
}

type Upserter interface {
	Upsert(ctx context.Context, articleID, sourceID uuid.UUID, result extract.Result, transferWindow string) (uuid.UUID, error)
}

// ExtractDeps is everything RunExtract needs.
type ExtractDeps struct {
	Store          ExtractStore
	Extractor      extract.Extractor
	Clusterer      Upserter
	TransferWindow string
	// MaxArticles caps how many articles one run sends to the extractor.
	MaxArticles       int
	Model             string
	JevMinProbability float64
}

type ExtractStats struct {
	Batches    int
	Extracted  int
	Clustered  int
	Rejected   int
	Incomplete int
	Failed     int
	Total      int
	Unmarked   int
	Skipped    int
	Capped     bool
}

// NewExtractor builds the extractor the config asks for: the stub when no
// API key is set, otherwise Claude behind the Jev gate — and an error when
// EXTRACT_API_KEY is set without TYPESAFE_API_KEY. See extract.NewFromConfig.
func NewExtractor(cfg config.Config) (extract.Extractor, error) {
	return extract.NewFromConfig(cfg)
}

// NewExtractDeps wires RunExtract's dependencies from config and the real
// store.
func NewExtractDeps(cfg config.Config, s *store.Store) (ExtractDeps, error) {
	extractor, err := NewExtractor(cfg)
	if err != nil {
		return ExtractDeps{}, err
	}
	return ExtractDeps{
		Store:             s,
		Extractor:         extractor,
		Clusterer:         cluster.New(s),
		TransferWindow:    cfg.TransferWindow,
		MaxArticles:       cfg.ExtractMaxArticlesPerRun,
		Model:             cfg.ExtractModel,
		JevMinProbability: cfg.JevMinProbability,
	}, nil
}

// Run is the scheduled job: ingest, then extract. Extract runs whatever
// ingest did — its input is the queue, which may hold earlier articles
// even when this ingest found nothing or failed. A failure in either stage
// is returned (joined) so the job execution is marked failed.
func Run(ctx context.Context, p Poller, d ExtractDeps) error {
	ingestErr := RunIngest(ctx, p)
	if ingestErr != nil {
		slog.Error("pipeline: ingest failed, running extract anyway", "error", ingestErr)
	}
	_, extractErr := RunExtract(ctx, d)
	if extractErr != nil {
		slog.Error("pipeline: extract failed", "error", extractErr)
	}
	return errors.Join(ingestErr, extractErr)
}

// RunIngest polls every source once.
func RunIngest(ctx context.Context, p Poller) error {
	slog.Info("ingest: starting")
	if err := p.PollOnce(ctx); err != nil {
		return err
	}
	slog.Info("ingest: done")
	return nil
}

// RunExtract drains the unprocessed-article queue in batches of batchSize
// until it is empty or d.MaxArticles articles have been attempted.
//
// Every article attempted is marked processed whatever the outcome, so the
// next ListUnprocessed returns fresh ones. An article whose MarkExtracted
// fails stays queued and comes back at the head of the next batch; it is
// not extracted again this run, and a batch holding only such articles
// ends the run — otherwise the loop would never terminate.
func RunExtract(ctx context.Context, d ExtractDeps) (ExtractStats, error) {
	var st ExtractStats
	attempted := make(map[uuid.UUID]struct{})

drain:
	for {
		if err := ctx.Err(); err != nil {
			return st, interrupted(st, err)
		}

		articles, err := d.Store.ListUnprocessed(ctx, batchSize)
		if err != nil {
			return st, fmt.Errorf("extract: list unprocessed: %w", err)
		}

		fresh := make([]models.Article, 0, len(articles))
		for _, a := range articles {
			if _, seen := attempted[a.ID]; !seen {
				fresh = append(fresh, a)
			}
		}
		if len(fresh) == 0 {
			// Everything listed already failed MarkExtracted this run.
			break
		}

		st.Batches++
		slog.Info("extract: starting batch", "batch", st.Batches, "articles", len(fresh), "model", d.Model)

		for i, article := range fresh {
			if st.Total >= d.MaxArticles {
				st.Capped = true
				break drain
			}
			if err := ctx.Err(); err != nil {
				st.Skipped = len(fresh) - i
				return st, interrupted(st, err)
			}
			attempted[article.ID] = struct{}{}
			st.Total++
			extractOne(ctx, d, article, &st)
		}

		if len(articles) < batchSize {
			break
		}
	}

	if st.Capped {
		slog.Warn("extract: stopped at the per-run cap with articles still queued — the backlog is growing faster than one run drains it",
			"max_articles", d.MaxArticles, "total", st.Total, "batches", st.Batches)
	}
	if st.Unmarked > 0 {
		slog.Warn("extract: articles could not be marked processed and stay queued — the next run will extract them again",
			"unmarked", st.Unmarked)
	}
	slog.Info("extract: batch complete",
		"extracted", st.Extracted,
		"clustered", st.Clustered,
		"rejected", st.Rejected,
		"incomplete", st.Incomplete,
		"failed", st.Failed,
		"total", st.Total,
		"batches", st.Batches,
		"unmarked", st.Unmarked,
		"capped", st.Capped,
		"jev_min_probability", d.JevMinProbability)
	return st, nil
}

func interrupted(st ExtractStats, err error) error {
	slog.Warn("extract: interrupted",
		"extracted", st.Extracted,
		"clustered", st.Clustered,
		"rejected", st.Rejected,
		"incomplete", st.Incomplete,
		"failed", st.Failed,
		"skipped", st.Skipped,
		"total", st.Total,
		"batches", st.Batches)
	return fmt.Errorf("extract: interrupted: %w", err)
}

func extractOne(ctx context.Context, d ExtractDeps, article models.Article, st *ExtractStats) {
	text := article.Title
	if article.Content != nil && *article.Content != "" {
		text += "\n\n" + *article.Content
	}

	result, err := d.Extractor.Extract(ctx, text)
	var extractionJSON []byte
	if err != nil {
		st.Failed++
		slog.Warn("extract: article extraction failed", "article_id", article.ID, "url", article.URL, "error", err)
	} else {
		st.Extracted++
		if extractionJSON, err = json.Marshal(result); err != nil {
			slog.Error("extract: marshal result", "article_id", article.ID, "error", err)
			extractionJSON = nil
		}

		clusterOne(ctx, d, article, result, st)
	}

	if err := d.Store.MarkExtracted(ctx, article.ID, extractionJSON); err != nil {
		st.Unmarked++
		slog.Error("extract: mark processed", "article_id", article.ID, "error", err)
	}
}

func clusterOne(ctx context.Context, d ExtractDeps, article models.Article, result extract.Result, st *ExtractStats) {
	if result.RejectedByJev {
		st.Rejected++
		slog.Info("extract: extraction rejected by the gate",
			"article_id", article.ID,
			"url", article.URL,
			"jev_probability", jevProbability(result),
			"jev_model", result.JevModel)
		return
	}

	_, err := d.Clusterer.Upsert(ctx, article.ID, article.SourceID, result, d.TransferWindow)
	switch {
	case errors.Is(err, cluster.ErrIncomplete):
		st.Incomplete++
		slog.Info("extract: extraction incomplete, not stored",
			"article_id", article.ID,
			"url", article.URL,
			"player_name", result.PlayerName,
			"to_club_name", result.ToClubName,
			"confidence", result.Confidence)
	case err != nil:
		slog.Error("extract: cluster upsert failed", "article_id", article.ID, "error", err)
	default:
		st.Clustered++
	}
}

func jevProbability(r extract.Result) any {
	if r.JevProbability == nil {
		return nil
	}
	return *r.JevProbability
}
