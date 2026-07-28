package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// fakeStore is an in-memory Store for tests. InsertArticle dedupes by URL,
// mirroring the real store's ON CONFLICT (url) DO NOTHING behavior.
type fakeStore struct {
	sources  []models.Source
	inserted []models.Article
}

func (f *fakeStore) ListSources(ctx context.Context) ([]models.Source, error) {
	return f.sources, nil
}

func (f *fakeStore) InsertArticle(ctx context.Context, a models.Article) (uuid.UUID, bool, error) {
	for _, existing := range f.inserted {
		if existing.URL == a.URL {
			return uuid.Nil, false, nil
		}
	}
	f.inserted = append(f.inserted, a)
	return uuid.New(), true, nil
}

const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <title>Player X linked with move to Club Y</title>
      <link>https://example.com/article-1</link>
      <description>Sources claim Player X is a target.</description>
      <pubDate>Mon, 02 Jan 2026 15:04:05 GMT</pubDate>
    </item>
    <item>
      <title>Duplicate link, should be deduped</title>
      <link>https://example.com/article-1</link>
      <description>Same URL as the item above.</description>
    </item>
    <item>
      <title>Another rumour</title>
      <link>https://example.com/article-2</link>
      <description>A different article entirely.</description>
    </item>
  </channel>
</rss>`

func TestPoller_PollOnce_StoresArticlesAndDedupesByURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()

	feedURL := srv.URL
	fs := &fakeStore{
		sources: []models.Source{
			{ID: uuid.New(), Name: "Test Source", FeedURL: &feedURL},
		},
	}

	NewPoller(fs).PollOnce(context.Background())

	if len(fs.inserted) != 2 {
		t.Fatalf("got %d inserted articles, want 2 (the duplicate URL should be deduped)", len(fs.inserted))
	}

	first := fs.inserted[0]
	if first.URL != "https://example.com/article-1" {
		t.Errorf("unexpected first article URL: %s", first.URL)
	}
	if first.Content == nil || *first.Content != "Sources claim Player X is a target." {
		t.Errorf("unexpected content: %v", first.Content)
	}
	if first.PublishedAt == nil {
		t.Error("expected PublishedAt to be parsed from pubDate")
	}

	second := fs.inserted[1]
	if second.URL != "https://example.com/article-2" {
		t.Errorf("unexpected second article URL: %s", second.URL)
	}
}

func TestPoller_PollOnce_SkipsSourcesWithoutFeedURL(t *testing.T) {
	fs := &fakeStore{
		sources: []models.Source{
			{ID: uuid.New(), Name: "No Feed Yet", FeedURL: nil},
		},
	}

	NewPoller(fs).PollOnce(context.Background())

	if len(fs.inserted) != 0 {
		t.Fatalf("got %d inserted articles, want 0 for a source with no feed_url", len(fs.inserted))
	}
}
