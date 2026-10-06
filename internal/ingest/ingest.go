// Package ingest polls configured news sources' RSS feeds and stores new
// articles for later extraction. One-shot per invocation — run it on a
// schedule (Cloud Scheduler triggering a Cloud Run Job; cron/systemd
// timer locally). See internal/pipeline for how it is run.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// Store is the subset of store.Store the ingest worker needs. Defined here
// (rather than depending on the concrete *store.Store) so PollOnce can be
// exercised in tests against a fake, without a real Postgres connection.
type Store interface {
	ListSources(ctx context.Context) ([]models.Source, error)
	InsertArticle(ctx context.Context, a models.Article) (uuid.UUID, bool, error)
}

type Poller struct {
	store  Store
	parser *gofeed.Parser
}

func NewPoller(s Store) *Poller {
	return &Poller{store: s, parser: gofeed.NewParser()}
}

// PollOnce fetches every source's feed and stores any new articles. Sources
// without a feed_url are skipped (see milestone 1.2).
//
// A failing source (unreachable feed, failed insert) does not stop the
// others from being polled. Each failure is logged as it happens and all of
// them are returned joined, so a scheduled run that lost a source exits
// non-zero instead of reporting success.
func (p *Poller) PollOnce(ctx context.Context) error {
	sources, err := p.store.ListSources(ctx)
	if err != nil {
		slog.Error("ingest: list sources", "error", err)
		return fmt.Errorf("ingest: list sources: %w", err)
	}

	var errs []error
	for _, src := range sources {
		if src.FeedURL == nil || *src.FeedURL == "" {
			continue
		}
		if err := p.pollSource(ctx, src); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *Poller) pollSource(ctx context.Context, src models.Source) error {
	feed, err := p.parser.ParseURLWithContext(*src.FeedURL, ctx)
	if err != nil {
		slog.Error("ingest: parse feed", "source", src.Name, "url", *src.FeedURL, "error", err)
		return fmt.Errorf("ingest: source %q: parse feed: %w", src.Name, err)
	}

	stored, failed := 0, 0
	var firstErr error
	for _, item := range feed.Items {
		article := models.Article{
			SourceID: src.ID,
			URL:      item.Link,
			Title:    item.Title,
		}
		switch {
		case item.Content != "":
			article.Content = &item.Content
		case item.Description != "":
			article.Content = &item.Description
		}
		if item.PublishedParsed != nil {
			article.PublishedAt = item.PublishedParsed
		}

		_, inserted, err := p.store.InsertArticle(ctx, article)
		if err != nil {
			slog.Error("ingest: insert article", "source", src.Name, "url", item.Link, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			failed++
			continue
		}
		if inserted {
			stored++
		}
	}

	if stored > 0 {
		slog.Info("ingest: polled source", "source", src.Name, "new_articles", stored)
	}
	if firstErr != nil {
		return fmt.Errorf("ingest: source %q: %d of %d inserts failed, first: %w", src.Name, failed, len(feed.Items), firstErr)
	}
	return nil
}
