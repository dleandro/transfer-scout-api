package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/db"
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

	all, hasMoreAll, err := s.ListRumours(ctx, total, 0)
	if err != nil {
		t.Fatalf("list rumours (limit covering every row): %v", err)
	}
	if len(all) != total {
		t.Fatalf("got %d rumours, want %d (all of them)", len(all), total)
	}
	if hasMoreAll {
		t.Error("expected has_more=false when limit covers every row")
	}

	partial, hasMorePartial, err := s.ListRumours(ctx, total-1, 0)
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
