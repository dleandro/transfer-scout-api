package store

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/db"
)

func newRosterTestStore(t *testing.T) *Store {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test (needs a real Postgres with migrations applied and seed/seed.sql loaded)")
	}
	pool, err := db.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return New(pool)
}

type clubRow struct {
	ID       uuid.UUID
	Name     string
	CrestURL *string
	League   *string
}

func readClub(t *testing.T, s *Store, name string) clubRow {
	t.Helper()
	var row clubRow
	err := s.Pool.QueryRow(context.Background(), `
		SELECT c.id, c.name, c.crest_url, l.name
		FROM clubs c LEFT JOIN leagues l ON l.id = c.league_id
		WHERE lower(c.name) = lower($1)`, name).Scan(&row.ID, &row.Name, &row.CrestURL, &row.League)
	if err != nil {
		t.Fatalf("read club %q: %v", name, err)
	}
	return row
}

func fixtureRoster(t *testing.T, leagueName string, clubs ...string) *roster {
	t.Helper()
	body := fmt.Sprintf(`{"name": %q, "short_name": "FX", "clubs": [`, leagueName)
	for i, club := range clubs {
		if i > 0 {
			body += ","
		}
		body += club
	}
	r, err := loadRoster(rosterFS(map[string]string{"fixture.json": body + "]}"}))
	if err != nil {
		t.Fatalf("load fixture roster: %v", err)
	}
	return r
}

func TestIntegration_SyncRoster_BackfillsLeaguelessBarcelona(t *testing.T) {
	s := newRosterTestStore(t)
	ctx := context.Background()

	if _, err := s.Pool.Exec(ctx, `
		INSERT INTO clubs (name) VALUES ('Barcelona')
		ON CONFLICT (lower(name)) DO UPDATE SET league_id = NULL, crest_url = NULL`); err != nil {
		t.Fatalf("make Barcelona leagueless: %v", err)
	}
	before := readClub(t, s, "Barcelona")
	if before.League != nil || before.CrestURL != nil {
		t.Fatalf("precondition: Barcelona has league %v, crest %v", before.League, before.CrestURL)
	}

	want, _ := clubRoster.lookup("Barcelona")
	for run := 1; run <= 2; run++ {
		if _, err := s.SyncRoster(ctx); err != nil {
			t.Fatalf("sync run %d: %v", run, err)
		}
		after := readClub(t, s, "Barcelona")
		if after.ID != before.ID {
			t.Errorf("run %d: Barcelona row changed from %s to %s", run, before.ID, after.ID)
		}
		if after.League == nil || *after.League != want.League {
			t.Errorf("run %d: Barcelona league = %v, want %q", run, after.League, want.League)
		}
		if after.CrestURL == nil || *after.CrestURL != want.CrestURL {
			t.Errorf("run %d: Barcelona crest = %v, want %q", run, after.CrestURL, want.CrestURL)
		}
	}
}

func TestIntegration_SyncRoster_StampsEveryRosterClub(t *testing.T) {
	s := newRosterTestStore(t)
	ctx := context.Background()

	if _, err := s.SyncRoster(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
	for _, entry := range clubRoster.entries() {
		row := readClub(t, s, entry.Name)
		if row.Name != entry.Name {
			t.Errorf("%s stored as %q", entry.Name, row.Name)
		}
		if row.League == nil || *row.League != entry.League {
			t.Errorf("%s league = %v, want %q", entry.Name, row.League, entry.League)
		}
		if entry.CrestURL != "" && (row.CrestURL == nil || *row.CrestURL != entry.CrestURL) {
			t.Errorf("%s crest = %v, want %q", entry.Name, row.CrestURL, entry.CrestURL)
		}
	}
}

func TestIntegration_SyncRoster_RenamesAliasRowsAndReportsDuplicates(t *testing.T) {
	s := newRosterTestStore(t)
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	canonical := "Sync Rename Club " + suffix
	alias := "SRC " + suffix
	secondAlias := "Sync Rename " + suffix
	r := fixtureRoster(t, "Sync Rename League "+suffix, fmt.Sprintf(
		`{"name": %q, "short_name": "SRC", "crest_url": "https://upload.wikimedia.org/x.svg", "aliases": [%q, %q]}`,
		canonical, alias, secondAlias))

	var aliasRowID uuid.UUID
	if err := s.Pool.QueryRow(ctx, `INSERT INTO clubs (name) VALUES ($1) RETURNING id`, alias).Scan(&aliasRowID); err != nil {
		t.Fatalf("insert alias-named row: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO clubs (name) VALUES ($1)`, secondAlias); err != nil {
		t.Fatalf("insert second alias-named row: %v", err)
	}

	result, err := s.syncRoster(ctx, r)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	row := readClub(t, s, canonical)
	if row.ID != aliasRowID {
		t.Errorf("canonical row is %s, want the renamed alias row %s", row.ID, aliasRowID)
	}
	if row.League == nil || row.CrestURL == nil {
		t.Errorf("renamed row league = %v, crest = %v; both should be stamped", row.League, row.CrestURL)
	}
	if !slices.Contains(result.Renamed, fmt.Sprintf("%q -> %q", alias, canonical)) {
		t.Errorf("Renamed = %v, want the alias row reported", result.Renamed)
	}
	if !slices.Contains(result.Unmerged, fmt.Sprintf("%q duplicates %q", secondAlias, canonical)) {
		t.Errorf("Unmerged = %v, want the second alias row reported", result.Unmerged)
	}
	if leftover := readClub(t, s, secondAlias); leftover.League != nil {
		t.Errorf("unmerged duplicate was stamped with league %v", *leftover.League)
	}
}

func TestIntegration_SyncRoster_DetachesClubsThatLeftARosterLeague(t *testing.T) {
	s := newRosterTestStore(t)
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	league := "Sync Detach League " + suffix
	stayed := "Sync Detach Stayed " + suffix
	left := "Sync Detach Left " + suffix
	club := func(name string) string {
		return fmt.Sprintf(`{"name": %q, "short_name": "SDC", "crest_url": ""}`, name)
	}

	if _, err := s.syncRoster(ctx, fixtureRoster(t, league, club(stayed), club(left))); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if row := readClub(t, s, left); row.League == nil || *row.League != league {
		t.Fatalf("precondition: %s league = %v", left, row.League)
	}

	if _, err := s.syncRoster(ctx, fixtureRoster(t, league, club(stayed))); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if row := readClub(t, s, left); row.League != nil {
		t.Errorf("%s still in %s after leaving the roster", left, *row.League)
	}
	if row := readClub(t, s, stayed); row.League == nil || *row.League != league {
		t.Errorf("%s league = %v, want %q", stayed, row.League, league)
	}
}

func TestIntegration_GetOrCreateClub_ResolvesAliasesAndStampsLeague(t *testing.T) {
	s := newRosterTestStore(t)
	ctx := context.Background()

	canonical, err := s.GetOrCreateClub(ctx, "Tottenham Hotspur")
	if err != nil {
		t.Fatalf("get or create canonical: %v", err)
	}
	for _, spelling := range []string{"Spurs", "spurs", "  Tottenham  ", "Tottenham Hotspur FC"} {
		got, err := s.GetOrCreateClub(ctx, spelling)
		if err != nil {
			t.Fatalf("get or create %q: %v", spelling, err)
		}
		if got != canonical {
			t.Errorf("GetOrCreateClub(%q) = %v, want the existing row %v", spelling, got, canonical)
		}
	}
	if row := readClub(t, s, "Tottenham Hotspur"); row.Name != "Tottenham Hotspur" || row.CrestURL == nil || row.League == nil {
		t.Errorf("Tottenham row = %+v; want canonical name, crest and league", row)
	}

	if _, err := s.Pool.Exec(ctx, `UPDATE clubs SET league_id = NULL, crest_url = NULL WHERE lower(name) = 'barcelona'`); err != nil {
		t.Fatalf("make Barcelona leagueless: %v", err)
	}
	barcelona, err := s.GetOrCreateClub(ctx, "Barca")
	if err != nil {
		t.Fatalf("get or create Barca: %v", err)
	}
	row := readClub(t, s, "Barcelona")
	if row.ID != barcelona || row.League == nil || *row.League != "La Liga" || row.CrestURL == nil {
		t.Errorf("Barca resolved to %+v, want the Barcelona row stamped with La Liga and a crest", row)
	}

	ambiguous, err := s.GetOrCreateClub(ctx, "Inter")
	if err != nil {
		t.Fatalf("get or create Inter: %v", err)
	}
	if row := readClub(t, s, "Inter"); row.ID != ambiguous || row.Name != "Inter" || row.League != nil {
		t.Errorf("ambiguous Inter stored as %+v, want its own leagueless row", row)
	}
}

func TestIntegration_GetOrCreateClub_StoresUnknownClubWithoutLeague(t *testing.T) {
	s := newRosterTestStore(t)
	ctx := context.Background()

	name := "Unknown Roster Club " + uuid.NewString()[:8]
	id, err := s.GetOrCreateClub(ctx, name)
	if err != nil {
		t.Fatalf("GetOrCreateClub: %v", err)
	}
	row := readClub(t, s, name)
	if row.ID != id || row.Name != name {
		t.Errorf("unknown club stored as %+v, want %q", row, name)
	}
	if row.League != nil || row.CrestURL != nil {
		t.Errorf("unknown club league = %v, crest = %v; want both NULL", row.League, row.CrestURL)
	}
}
