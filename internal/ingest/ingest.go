// Package ingest polls configured news sources' RSS feeds and stores new
// articles for later extraction.
package ingest

import (
	"context"
	"log/slog"
	"time"

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

// Run polls every source's RSS feed immediately, then again on the given
// interval, until ctx is cancelled.
func (p *Poller) Run(ctx context.Context, interval time.Duration) {
	p.PollOnce(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.PollOnce(ctx)
		}
	}
}

// PollOnce fetches every source's feed and stores any new articles. Sources
// without a feed_url are skipped (see milestone 1.2).
func (p *Poller) PollOnce(ctx context.Context) {
	sources, err := p.store.ListSources(ctx)
	if err != nil {
		slog.Error("ingest: list sources", "error", err)
		return
	}

	for _, src := range sources {
		if src.FeedURL == nil || *src.FeedURL == "" {
			continue
		}
		p.pollSource(ctx, src)
	}
}

func (p *Poller) pollSource(ctx context.Context, src models.Source) {
	feed, err := p.parser.ParseURLWithContext(*src.FeedURL, ctx)
	if err != nil {
		slog.Error("ingest: parse feed", "source", src.Name, "url", *src.FeedURL, "error", err)
		return
	}

	stored := 0
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
			continue
		}
		if inserted {
			stored++
		}
	}

	if stored > 0 {
		slog.Info("ingest: polled source", "source", src.Name, "new_articles", stored)
	}
}
