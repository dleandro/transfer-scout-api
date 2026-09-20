package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// TestIntegration_ListLeagues_ReturnsSeededPremierLeague proves migration
// 0008 seeds the Premier League row and ListLeagues surfaces it.
func TestIntegration_ListLeagues_ReturnsSeededPremierLeague(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	leagues, err := s.ListLeagues(ctx)
	if err != nil {
		t.Fatalf("ListLeagues: %v", err)
	}

	var found bool
	for _, l := range leagues {
		if l.Name == "Premier League" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Premier League among %d leagues, not found", len(leagues))
	}
}

// TestIntegration_GetOrCreateClub_StampsLeagueForKnownPLClubOnly proves
// GetOrCreateClub stamps league_id from the same known-club check as
// crest_url (see clubCrests) — a club in that map gets the Premier
// League's id, on both the create and ON CONFLICT backfill paths, while a
// club outside it (e.g. a foreign club a rumour mentions in passing) stays
// leagueless rather than defaulting to the PL.
func TestIntegration_GetOrCreateClub_StampsLeagueForKnownPLClubOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	leagues, err := s.ListLeagues(ctx)
	if err != nil {
		t.Fatalf("ListLeagues: %v", err)
	}
	var premierLeagueID uuid.UUID
	for _, l := range leagues {
		if l.Name == "Premier League" {
			premierLeagueID = l.ID
		}
	}
	if premierLeagueID == uuid.Nil {
		t.Fatal("precondition: Premier League not seeded")
	}

	leagueOf := func(id uuid.UUID) *uuid.UUID {
		var leagueID *uuid.UUID
		if err := s.Pool.QueryRow(ctx, `SELECT league_id FROM clubs WHERE id = $1`, id).Scan(&leagueID); err != nil {
			t.Fatalf("select league_id: %v", err)
		}
		return leagueID
	}

	t.Run("stamps the Premier League id when creating a known PL club", func(t *testing.T) {
		id, err := s.GetOrCreateClub(ctx, "Everton")
		if err != nil {
			t.Fatalf("GetOrCreateClub: %v", err)
		}
		got := leagueOf(id)
		if got == nil || *got != premierLeagueID {
			t.Errorf("league_id = %v, want %v", got, premierLeagueID)
		}
	})

	t.Run("backfills a pre-existing leagueless seeded row on conflict", func(t *testing.T) {
		var seededID uuid.UUID
		if err := s.Pool.QueryRow(ctx, `
			INSERT INTO clubs (name) VALUES ('Everton')
			ON CONFLICT (lower(name)) DO UPDATE SET league_id = NULL
			RETURNING id`).Scan(&seededID); err != nil {
			t.Fatalf("seed leagueless Everton: %v", err)
		}
		if got := leagueOf(seededID); got != nil {
			t.Fatalf("precondition: expected seeded Everton to start leagueless, got %v", *got)
		}

		gotID, err := s.GetOrCreateClub(ctx, "everton")
		if err != nil {
			t.Fatalf("GetOrCreateClub: %v", err)
		}
		if gotID != seededID {
			t.Errorf("expected the existing Everton row (%s), got %s", seededID, gotID)
		}
		if got := leagueOf(gotID); got == nil || *got != premierLeagueID {
			t.Errorf("league_id = %v, want %v", got, premierLeagueID)
		}
	})

	t.Run("leaves league_id nil for a club outside the known PL set", func(t *testing.T) {
		id, err := s.GetOrCreateClub(ctx, uniqueName("Real Madrid"))
		if err != nil {
			t.Fatalf("GetOrCreateClub: %v", err)
		}
		if got := leagueOf(id); got != nil {
			t.Errorf("league_id = %v, want nil for a club outside the known PL set", *got)
		}
	})
}

// TestIntegration_ListRumours_FilterByLeague proves LeagueID matches a
// rumour whose to_club or from_club belongs to the given league, mirroring
// ClubID's either-side semantics, without matching a rumour whose clubs
// belong to a different league.
func TestIntegration_ListRumours_FilterByLeague(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	var leagueAID, leagueBID uuid.UUID
	if err := s.Pool.QueryRow(ctx, `INSERT INTO leagues (name) VALUES ($1) RETURNING id`,
		uniqueName("League Filter Test League A")).Scan(&leagueAID); err != nil {
		t.Fatalf("insert league A: %v", err)
	}
	if err := s.Pool.QueryRow(ctx, `INSERT INTO leagues (name) VALUES ($1) RETURNING id`,
		uniqueName("League Filter Test League B")).Scan(&leagueBID); err != nil {
		t.Fatalf("insert league B: %v", err)
	}

	newClubInLeague := func(leagueID uuid.UUID) uuid.UUID {
		var id uuid.UUID
		if err := s.Pool.QueryRow(ctx, `INSERT INTO clubs (name, league_id) VALUES ($1, $2) RETURNING id`,
			uniqueName("League Filter Test Club"), leagueID).Scan(&id); err != nil {
			t.Fatalf("insert club: %v", err)
		}
		return id
	}

	clubA1 := newClubInLeague(leagueAID)
	clubA2 := newClubInLeague(leagueAID)
	clubB := newClubInLeague(leagueBID)

	player, err := s.GetOrCreatePlayer(ctx, uniqueName("League Filter Player"))
	if err != nil {
		t.Fatalf("get or create player: %v", err)
	}

	// A: -> clubA1 (matches league A on the "to" side).
	rumourA, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: player, ToClubID: clubA1, TransferWindow: uniqueName("window"),
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour A: %v", err)
	}
	// B: clubA2 -> clubB (matches league A on the "from" side).
	rumourB, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: player, FromClubID: &clubA2, ToClubID: clubB, TransferWindow: uniqueName("window"),
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour B: %v", err)
	}
	// C: -> clubB, no relation to league A at all.
	rumourC, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: player, ToClubID: clubB, TransferWindow: uniqueName("window"),
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour C: %v", err)
	}

	items, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{LeagueID: &leagueAID}, nil)
	if err != nil {
		t.Fatalf("list rumours: %v", err)
	}
	if !containsID(items, rumourA.ID) || !containsID(items, rumourB.ID) {
		t.Errorf("expected A and B (both touch league A), got %d rumours", len(items))
	}
	if containsID(items, rumourC.ID) {
		t.Error("rumour C doesn't touch league A, should not match")
	}
}

// TestIntegration_ListClubs_FilterByLeague proves the leagueID param on
// ListClubs narrows to only clubs in that league.
func TestIntegration_ListClubs_FilterByLeague(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	var leagueID uuid.UUID
	if err := s.Pool.QueryRow(ctx, `INSERT INTO leagues (name) VALUES ($1) RETURNING id`,
		uniqueName("Clubs Filter Test League")).Scan(&leagueID); err != nil {
		t.Fatalf("insert league: %v", err)
	}

	var inLeagueID uuid.UUID
	if err := s.Pool.QueryRow(ctx, `INSERT INTO clubs (name, league_id) VALUES ($1, $2) RETURNING id`,
		uniqueName("Clubs Filter Test Club In League"), leagueID).Scan(&inLeagueID); err != nil {
		t.Fatalf("insert in-league club: %v", err)
	}
	outOfLeagueID, err := s.GetOrCreateClub(ctx, uniqueName("Clubs Filter Test Club Leagueless"))
	if err != nil {
		t.Fatalf("get or create leagueless club: %v", err)
	}

	clubs, err := s.ListClubs(ctx, nil, &leagueID)
	if err != nil {
		t.Fatalf("ListClubs: %v", err)
	}
	if !mustContainClub(clubs, inLeagueID) {
		t.Error("expected the in-league club to match the league filter")
	}
	if mustContainClub(clubs, outOfLeagueID) {
		t.Error("expected the leagueless club not to match the league filter")
	}
}

func mustContainClub(clubs []store.ClubFeedItem, id uuid.UUID) bool {
	for _, c := range clubs {
		if c.ID == id {
			return true
		}
	}
	return false
}
