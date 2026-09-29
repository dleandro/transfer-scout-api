package store

import (
	"fmt"
	"strings"
)

// premierLeagueClub is one row of the Premier League roster.
type premierLeagueClub struct {
	Name      string
	ShortName string
	CrestURL  string
}

// premierLeagueClubs is the single source of truth for the Premier League
// roster: canonical names, abbreviations and crest URLs.
//
// Three things read it. GetOrCreateClub stamps crest_url from it when it
// creates a club and backfills it on conflict; the same "is this club in the
// roster" check stamps league_id, since every club here is a PL club. And
// seed/seed.sql's club INSERT is generated from it — TestSeedSQLMatchesRoster
// regenerates the block and fails if the file has drifted, which is what stops
// the roster and the seed from disagreeing when promotion and relegation churn
// three clubs a season.
//
// Names are compared case-insensitively and whitespace-trimmed. A club absent
// from this list keeps an empty crest_url and a NULL league_id rather than a
// guessed, broken URL or a wrong league.
//
// URLs are en.wikipedia upload.wikimedia.org file URLs — PL crests are
// non-free logos, hosted per-wiki rather than on Wikimedia Commons. Each was
// verified to resolve (HTTP 200) when added.
//
// The identical name list inside migration 0008 is deliberately NOT derived
// from this: a migration is frozen point-in-time history and must keep
// backfilling exactly the clubs that were in the league when it was written.
var premierLeagueClubs = []premierLeagueClub{
	{"Arsenal", "ARS", "https://upload.wikimedia.org/wikipedia/en/5/53/Arsenal_FC.svg"},
	{"Aston Villa", "AVL", "https://upload.wikimedia.org/wikipedia/en/9/9a/Aston_Villa_FC_new_crest.svg"},
	{"Bournemouth", "BOU", "https://upload.wikimedia.org/wikipedia/en/e/e5/AFC_Bournemouth_%282013%29.svg"},
	{"Brentford", "BRE", "https://upload.wikimedia.org/wikipedia/en/2/2a/Brentford_FC_crest.svg"},
	{"Brighton & Hove Albion", "BHA", "https://upload.wikimedia.org/wikipedia/en/d/d0/Brighton_and_Hove_Albion_FC_crest.svg"},
	{"Burnley", "BUR", "https://upload.wikimedia.org/wikipedia/en/6/6d/Burnley_FC_Logo.svg"},
	{"Chelsea", "CHE", "https://upload.wikimedia.org/wikipedia/en/c/cc/Chelsea_FC.svg"},
	{"Crystal Palace", "CRY", "https://upload.wikimedia.org/wikipedia/en/a/a2/Crystal_Palace_FC_logo_%282022%29.svg"},
	{"Everton", "EVE", "https://upload.wikimedia.org/wikipedia/en/7/7c/Everton_FC_logo.svg"},
	{"Fulham", "FUL", "https://upload.wikimedia.org/wikipedia/en/e/eb/Fulham_FC_%28shield%29.svg"},
	{"Leeds United", "LEE", "https://upload.wikimedia.org/wikipedia/en/5/54/Leeds_United_F.C._logo.svg"},
	{"Liverpool", "LIV", "https://upload.wikimedia.org/wikipedia/en/0/0c/Liverpool_FC.svg"},
	{"Manchester City", "MCI", "https://upload.wikimedia.org/wikipedia/en/e/eb/Manchester_City_FC_badge.svg"},
	{"Manchester United", "MUN", "https://upload.wikimedia.org/wikipedia/en/7/7a/Manchester_United_FC_crest.svg"},
	{"Newcastle United", "NEW", "https://upload.wikimedia.org/wikipedia/en/5/56/Newcastle_United_Logo.svg"},
	{"Nottingham Forest", "NFO", "https://upload.wikimedia.org/wikipedia/en/e/e5/Nottingham_Forest_F.C._logo.svg"},
	{"Sunderland", "SUN", "https://upload.wikimedia.org/wikipedia/en/7/77/Logo_Sunderland.svg"},
	{"Tottenham Hotspur", "TOT", "https://upload.wikimedia.org/wikipedia/en/b/b4/Tottenham_Hotspur.svg"},
	{"West Ham United", "WHU", "https://upload.wikimedia.org/wikipedia/en/c/c2/West_Ham_United_FC_logo.svg"},
	{"Wolverhampton Wanderers", "WOL", "https://upload.wikimedia.org/wikipedia/en/f/fc/Wolverhampton_Wanderers.svg"},
}

// clubCrests indexes premierLeagueClubs by lower-cased name, so
// GetOrCreateClub pays a map lookup per insert rather than a scan.
var clubCrests = func() map[string]string {
	index := make(map[string]string, len(premierLeagueClubs))
	for _, club := range premierLeagueClubs {
		index[strings.ToLower(club.Name)] = club.CrestURL
	}
	return index
}()

// crestURLFor returns the crest URL for a club name, or nil when the club
// is not in the roster. The match is case-insensitive and whitespace-
// trimmed, mirroring GetOrCreateClub's own name handling. Returning nil
// (rather than "") lets callers pass the result straight through to the
// nullable crest_url column.
func crestURLFor(name string) *string {
	if url, ok := clubCrests[strings.ToLower(strings.TrimSpace(name))]; ok {
		return &url
	}
	return nil
}

// seedClubsSQL renders the club INSERT that seed/seed.sql carries.
//
// The rows arrive already carrying crest_url and league_id rather than
// waiting to be stamped by GetOrCreateClub on first reference, so a freshly
// seeded database shows crests and leagues immediately. ON CONFLICT mirrors
// GetOrCreateClub's own upsert, which makes re-seeding an existing database
// a backfill rather than a no-op.
//
// The LEFT JOIN (not a CROSS JOIN) is deliberate: if the Premier League row
// is somehow missing, every club still inserts with a NULL league_id — the
// documented "unknown league" state — instead of the statement silently
// inserting nothing.
func seedClubsSQL() string {
	var b strings.Builder
	b.WriteString("INSERT INTO clubs (name, short_name, crest_url, league_id)\n")
	b.WriteString("SELECT c.name, c.short_name, c.crest_url, l.id\n")
	b.WriteString("FROM (VALUES\n")

	for i, club := range premierLeagueClubs {
		sep := ","
		if i == len(premierLeagueClubs)-1 {
			sep = ""
		}
		fmt.Fprintf(&b, "    (%s, %s, %s)%s\n",
			quoteSQL(club.Name), quoteSQL(club.ShortName), quoteSQL(club.CrestURL), sep)
	}

	b.WriteString(") AS c(name, short_name, crest_url)\n")
	b.WriteString("LEFT JOIN leagues l ON lower(l.name) = 'premier league'\n")
	b.WriteString("ON CONFLICT (lower(name)) DO UPDATE\n")
	b.WriteString("SET short_name = EXCLUDED.short_name,\n")
	b.WriteString("    crest_url = COALESCE(EXCLUDED.crest_url, clubs.crest_url),\n")
	b.WriteString("    league_id = COALESCE(EXCLUDED.league_id, clubs.league_id);")
	return b.String()
}

// quoteSQL renders a Go string as a SQL string literal, doubling embedded
// single quotes. The roster has none today; a club like "Nott'm Forest"
// would otherwise generate a broken seed file.
func quoteSQL(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
