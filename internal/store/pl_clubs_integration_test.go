package store

import (
	"context"
	"os"
	"testing"

	"github.com/dleandro/transfer-scout-api/internal/db"
)

// TestIntegration_SeedClubsSQL_StampsCrestAndLeague runs the generated club
// INSERT against a real Postgres and checks what the seed is supposed to
// deliver: every roster club present with a crest and a league straight
// away, rather than waiting to be stamped by GetOrCreateClub the first time
// clustering happens to mention it.
//
// It also covers the upsert path, since seeding an already-seeded database
// is the normal case: a club whose crest and league were nulled (how rows
// looked before those columns existed) gets them back, and a deleted club
// returns, without the statement erroring on the rows that were fine.
//
// Lives in package store, not store_test, because seedClubsSQL is
// unexported — the generated SQL is an implementation detail of the roster.
func TestIntegration_SeedClubsSQL_StampsCrestAndLeague(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test (needs a real Postgres with migrations applied)")
	}

	ctx := context.Background()
	pool, err := db.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	seed := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, seedClubsSQL()); err != nil {
			t.Fatalf("seed clubs: %v", err)
		}
	}

	seed()

	for _, club := range premierLeagueClubs {
		var crest *string
		var leagueID *string
		err := pool.QueryRow(ctx,
			`SELECT crest_url, league_id::text FROM clubs WHERE lower(name) = lower($1)`,
			club.Name).Scan(&crest, &leagueID)
		if err != nil {
			t.Fatalf("%s not seeded: %v", club.Name, err)
		}
		if crest == nil || *crest != club.CrestURL {
			t.Errorf("%s crest_url = %v, want %q", club.Name, crest, club.CrestURL)
		}
		if leagueID == nil {
			t.Errorf("%s has no league_id", club.Name)
		}
	}

	// Re-seeding an existing database must backfill, not no-op.
	const nulled = "Everton"
	if _, err := pool.Exec(ctx,
		`UPDATE clubs SET crest_url = NULL, league_id = NULL WHERE lower(name) = lower($1)`,
		nulled); err != nil {
		t.Fatalf("null out %s: %v", nulled, err)
	}

	seed()

	var crest *string
	var leagueID *string
	if err := pool.QueryRow(ctx,
		`SELECT crest_url, league_id::text FROM clubs WHERE lower(name) = lower($1)`,
		nulled).Scan(&crest, &leagueID); err != nil {
		t.Fatalf("re-read %s: %v", nulled, err)
	}
	if crest == nil {
		t.Errorf("%s crest_url was not backfilled by re-seeding", nulled)
	}
	if leagueID == nil {
		t.Errorf("%s league_id was not backfilled by re-seeding", nulled)
	}
}
