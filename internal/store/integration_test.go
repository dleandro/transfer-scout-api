package store_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/db"
	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// newTestStore connects to a real Postgres instance via DATABASE_URL (see
// .env.example) with migrations already applied. It skips rather than
// fails when DATABASE_URL is unset, so `go test ./...` still passes
// without a database running (e.g. a bare CI without services).
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test (needs a real Postgres with migrations applied)")
	}
	pool, err := db.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return store.New(pool)
}

// uniqueName avoids collisions with rows left behind by previous runs of
// this test against the same dev database.
func uniqueName(prefix string) string {
	return prefix + " " + uuid.NewString()[:8]
}

func TestIntegration_GetOrCreateClub_IsCaseInsensitiveAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	name := uniqueName("Integration Test Club")

	id1, err := s.GetOrCreateClub(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id2, err := s.GetOrCreateClub(ctx, strings.ToUpper(name))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 != id2 {
		t.Errorf("expected a case-insensitive match to reuse the same club, got %s and %s", id1, id2)
	}
}

func TestIntegration_GetOrCreatePlayer_IsCaseInsensitiveAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	name := uniqueName("Integration Test Player")

	id1, err := s.GetOrCreatePlayer(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id2, err := s.GetOrCreatePlayer(ctx, strings.ToLower(name))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 != id2 {
		t.Errorf("expected a case-insensitive match to reuse the same player, got %s and %s", id1, id2)
	}
}

// This is also the real-database proof for two things CLAUDE.md previously
// flagged as untested: pgx v5 scanning NUMERIC into *float64 and the
// rumour_status enum into models.RumourStatus both work as expected.
func TestIntegration_UpsertRumour_ClustersStatusForwardOnlyAndWidensFeeRange(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	playerID, err := s.GetOrCreatePlayer(ctx, uniqueName("Integration Test Player"))
	if err != nil {
		t.Fatalf("get or create player: %v", err)
	}
	toClubID, err := s.GetOrCreateClub(ctx, uniqueName("Integration Test Club"))
	if err != nil {
		t.Fatalf("get or create club: %v", err)
	}

	fee1min, fee1max := 20_000_000.0, 30_000_000.0
	r1, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: playerID, ToClubID: toClubID, TransferWindow: "summer-2026",
		Status: models.StatusAdvanced, FeeMinEUR: &fee1min, FeeMaxEUR: &fee1max,
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// A regression: "talks" after "advanced" should not roll the status
	// back, but the fee range should still widen.
	fee2min, fee2max := 15_000_000.0, 25_000_000.0
	r2, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: playerID, ToClubID: toClubID, TransferWindow: "summer-2026",
		Status: models.StatusTalks, FeeMinEUR: &fee2min, FeeMaxEUR: &fee2max,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if r1.ID != r2.ID {
		t.Fatalf("expected the same rumour row (clustered on player/to_club/window), got %s and %s", r1.ID, r2.ID)
	}
	if r2.Status != models.StatusAdvanced {
		t.Errorf("status regressed: got %s, want advanced (a later 'talks' report should not roll it back)", r2.Status)
	}
	if r2.FeeMinEUR == nil || *r2.FeeMinEUR != 15_000_000 {
		t.Errorf("fee min did not widen: got %v, want 15000000", r2.FeeMinEUR)
	}
	if r2.FeeMaxEUR == nil || *r2.FeeMaxEUR != 30_000_000 {
		t.Errorf("fee max did not widen: got %v, want 30000000", r2.FeeMaxEUR)
	}

	// rumour_events.article_id/source_id are FK-constrained, so the event
	// needs a real article (and thus a real source) behind it.
	sources, err := s.ListSources(ctx)
	if err != nil || len(sources) == 0 {
		t.Fatalf("list sources: %v (need at least one seeded source)", err)
	}
	articleID, _, err := s.InsertArticle(ctx, models.Article{
		SourceID: sources[0].ID,
		URL:      "https://example.com/integration-test/" + uuid.NewString(),
		Title:    "Integration test article",
	})
	if err != nil {
		t.Fatalf("insert article: %v", err)
	}

	if err := s.InsertRumourEvent(ctx, store.RumourEventParams{
		RumourID: r2.ID, ArticleID: articleID, SourceID: sources[0].ID, Status: models.StatusTalks,
	}); err != nil {
		t.Fatalf("insert rumour event: %v", err)
	}

	_, events, err := s.GetRumourByID(ctx, r2.ID)
	if err != nil {
		t.Fatalf("get rumour by id: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
}
