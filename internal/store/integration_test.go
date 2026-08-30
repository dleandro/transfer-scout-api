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

// TestIntegration_ListRumours_HasMoreReflectsWhetherAFurtherPageExists
// proves the limit+1-and-trim pagination trick against a real database:
// requesting exactly as many rows as exist should report has_more=false,
// and requesting one fewer should report has_more=true.
//
// This is deliberately count-relative rather than asserting on a fixed
// number of rows, since the dev database this runs against already has
// real rumours in it from earlier milestone work (and other tests in
// this file, run repeatedly, leave rows behind too — same convention as
// the other integration tests here). We only insert enough of our own
// rows (2, via a no-store-helper-yet raw insert — store.UpsertRumour
// doesn't exist until milestone 1.4) to guarantee at least 2 rumours
// exist to page through even against an empty database.
func TestIntegration_ListRumours_HasMoreReflectsWhetherAFurtherPageExists(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	var playerID, clubID uuid.UUID
	if err := s.Pool.QueryRow(ctx, `INSERT INTO players (name) VALUES ($1) RETURNING id`,
		uniqueName("Integration Test Player")).Scan(&playerID); err != nil {
		t.Fatalf("insert test player: %v", err)
	}
	if err := s.Pool.QueryRow(ctx, `INSERT INTO clubs (name) VALUES ($1) RETURNING id`,
		uniqueName("Integration Test Club")).Scan(&clubID); err != nil {
		t.Fatalf("insert test club: %v", err)
	}

	windowPrefix := uniqueName("pagination-window")
	for i := 0; i < 2; i++ {
		window := windowPrefix + "-" + uuid.NewString()[:4]
		if _, err := s.Pool.Exec(ctx, `
			INSERT INTO rumours (player_id, to_club_id, transfer_window, status)
			VALUES ($1, $2, $3, 'rumoured')`, playerID, clubID, window); err != nil {
			t.Fatalf("insert test rumour: %v", err)
		}
	}

	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM rumours`).Scan(&total); err != nil {
		t.Fatalf("count rumours: %v", err)
	}
	if total < 2 {
		t.Fatalf("expected at least 2 rumours to test pagination against, got %d", total)
	}

	all, hasMoreAll, err := s.ListRumours(ctx, total, 0, store.RumourFilter{})
	if err != nil {
		t.Fatalf("list rumours (limit covering every row): %v", err)
	}
	if len(all) != total {
		t.Fatalf("got %d rumours, want %d (all of them)", len(all), total)
	}
	if hasMoreAll {
		t.Error("expected has_more=false when limit covers every row")
	}

	partial, hasMorePartial, err := s.ListRumours(ctx, total-1, 0, store.RumourFilter{})
	if err != nil {
		t.Fatalf("list rumours (limit one short): %v", err)
	}
	if len(partial) != total-1 {
		t.Fatalf("got %d rumours, want %d (one short of all of them)", len(partial), total-1)
	}
	if !hasMorePartial {
		t.Error("expected has_more=true when one row remains beyond the requested page")
	}
}

// containsID reports whether items includes a rumour with the given id.
func containsID(items []store.RumourFeedItem, id uuid.UUID) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

// TestIntegration_ListRumours_FilterByClubAndPlayer proves ClubID matches
// either side of the deal (to_club_id OR from_club_id), PlayerID matches
// the player, and both combine with AND — against real Postgres, since
// the WHERE clause is built dynamically per filter.
func TestIntegration_ListRumours_FilterByClubAndPlayer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	player1, err := s.GetOrCreatePlayer(ctx, uniqueName("Filter Test Player One"))
	if err != nil {
		t.Fatalf("get or create player1: %v", err)
	}
	player2, err := s.GetOrCreatePlayer(ctx, uniqueName("Filter Test Player Two"))
	if err != nil {
		t.Fatalf("get or create player2: %v", err)
	}
	club1, err := s.GetOrCreateClub(ctx, uniqueName("Filter Test Club One"))
	if err != nil {
		t.Fatalf("get or create club1: %v", err)
	}
	club2, err := s.GetOrCreateClub(ctx, uniqueName("Filter Test Club Two"))
	if err != nil {
		t.Fatalf("get or create club2: %v", err)
	}

	// A: player1 -> club1 (matches club1 on the "to" side, matches player1).
	rumourA, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: player1, ToClubID: club1, TransferWindow: uniqueName("window"),
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour A: %v", err)
	}
	// B: player2, club1 -> club2 (matches club1 on the "from" side, not player1).
	rumourB, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: player2, FromClubID: &club1, ToClubID: club2, TransferWindow: uniqueName("window"),
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour B: %v", err)
	}
	// C: player1 -> club2 (matches player1, not club1).
	rumourC, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: player1, ToClubID: club2, TransferWindow: uniqueName("window"),
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour C: %v", err)
	}

	t.Run("club filter matches both to_club_id and from_club_id", func(t *testing.T) {
		items, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{ClubID: &club1})
		if err != nil {
			t.Fatalf("list rumours: %v", err)
		}
		if !containsID(items, rumourA.ID) || !containsID(items, rumourB.ID) {
			t.Errorf("expected A and B (both touch club1), got %d rumours", len(items))
		}
		if containsID(items, rumourC.ID) {
			t.Error("rumour C doesn't touch club1, should not match")
		}
	})

	t.Run("player filter matches player_id", func(t *testing.T) {
		items, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{PlayerID: &player1})
		if err != nil {
			t.Fatalf("list rumours: %v", err)
		}
		if !containsID(items, rumourA.ID) || !containsID(items, rumourC.ID) {
			t.Errorf("expected A and C (both player1), got %d rumours", len(items))
		}
		if containsID(items, rumourB.ID) {
			t.Error("rumour B is player2, should not match")
		}
	})

	t.Run("club and player filters combine with AND", func(t *testing.T) {
		items, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{ClubID: &club1, PlayerID: &player1})
		if err != nil {
			t.Fatalf("list rumours: %v", err)
		}
		if !containsID(items, rumourA.ID) {
			t.Error("expected A (club1 AND player1) to match")
		}
		if containsID(items, rumourB.ID) || containsID(items, rumourC.ID) {
			t.Error("B (wrong player) and C (wrong club) should not match the combined filter")
		}
	})
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
	r1, resolved1, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: playerID, ToClubID: toClubID, TransferWindow: "summer-2026",
		Status: models.StatusAdvanced, FeeMinEUR: &fee1min, FeeMaxEUR: &fee1max,
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if resolved1 {
		t.Error("expected justResolved=false for a non-terminal status")
	}

	// A regression: "talks" after "advanced" should not roll the status
	// back, but the fee range should still widen.
	fee2min, fee2max := 15_000_000.0, 25_000_000.0
	r2, resolved2, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: playerID, ToClubID: toClubID, TransferWindow: "summer-2026",
		Status: models.StatusTalks, FeeMinEUR: &fee2min, FeeMaxEUR: &fee2max,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if resolved2 {
		t.Error("expected justResolved=false when status doesn't move to a terminal state")
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

	item, events, err := s.GetRumourByID(ctx, r2.ID)
	if err != nil {
		t.Fatalf("get rumour by id: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if item.Credibility == nil {
		t.Error("expected credibility to be non-nil once the rumour has a contributing source")
	}
}

func TestIntegration_NudgeSourceReliability_ChangesScoreAndCredibility(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sources, err := s.ListSources(ctx)
	if err != nil || len(sources) == 0 {
		t.Fatalf("list sources: %v (need at least one seeded source)", err)
	}
	source := sources[0]

	playerID, err := s.GetOrCreatePlayer(ctx, uniqueName("Integration Test Player"))
	if err != nil {
		t.Fatalf("get or create player: %v", err)
	}
	toClubID, err := s.GetOrCreateClub(ctx, uniqueName("Integration Test Club"))
	if err != nil {
		t.Fatalf("get or create club: %v", err)
	}
	rumour, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: playerID, ToClubID: toClubID, TransferWindow: "summer-2026",
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour: %v", err)
	}
	articleID, _, err := s.InsertArticle(ctx, models.Article{
		SourceID: source.ID,
		URL:      "https://example.com/integration-test/" + uuid.NewString(),
		Title:    "Integration test article",
	})
	if err != nil {
		t.Fatalf("insert article: %v", err)
	}
	if err := s.InsertRumourEvent(ctx, store.RumourEventParams{
		RumourID: rumour.ID, ArticleID: articleID, SourceID: source.ID, Status: models.StatusRumoured,
	}); err != nil {
		t.Fatalf("insert rumour event: %v", err)
	}

	before, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	var scoreBefore float64
	for _, src := range before {
		if src.ID == source.ID {
			scoreBefore = src.ReliabilityScore
		}
	}

	if err := s.NudgeSourceReliability(ctx, rumour.ID, 2.0); err != nil {
		t.Fatalf("nudge source reliability: %v", err)
	}

	after, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	var scoreAfter float64
	for _, src := range after {
		if src.ID == source.ID {
			scoreAfter = src.ReliabilityScore
		}
	}
	if scoreAfter != scoreBefore+2.0 {
		t.Errorf("reliability_score: got %v, want %v", scoreAfter, scoreBefore+2.0)
	}

	item, _, err := s.GetRumourByID(ctx, rumour.ID)
	if err != nil {
		t.Fatalf("get rumour by id: %v", err)
	}
	if item.Credibility == nil || *item.Credibility != scoreAfter {
		t.Errorf("credibility: got %v, want %v (single contributing source's score)", item.Credibility, scoreAfter)
	}
}
