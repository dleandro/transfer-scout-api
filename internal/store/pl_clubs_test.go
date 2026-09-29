package store

import (
	"os"
	"strings"
	"testing"
)

func TestCrestURLFor(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// want is the expected URL; "" means crestURLFor should return nil.
		want string
	}{
		{"exact canonical name", "Arsenal", "https://upload.wikimedia.org/wikipedia/en/5/53/Arsenal_FC.svg"},
		{"case-insensitive match", "chelsea", "https://upload.wikimedia.org/wikipedia/en/c/cc/Chelsea_FC.svg"},
		{"surrounding whitespace trimmed", "  Liverpool  ", "https://upload.wikimedia.org/wikipedia/en/0/0c/Liverpool_FC.svg"},
		{"multi-word name with ampersand", "Brighton & Hove Albion", "https://upload.wikimedia.org/wikipedia/en/d/d0/Brighton_and_Hove_Albion_FC_crest.svg"},
		{"unmapped club is nil", "Real Madrid", ""},
		{"empty string is nil", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crestURLFor(tt.input)
			if tt.want == "" {
				if got != nil {
					t.Errorf("crestURLFor(%q) = %q, want nil", tt.input, *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("crestURLFor(%q) = nil, want %q", tt.input, tt.want)
			}
			if *got != tt.want {
				t.Errorf("crestURLFor(%q) = %q, want %q", tt.input, *got, tt.want)
			}
		})
	}
}

// TestSeedSQLMatchesRoster is the mechanical check that stops seed/seed.sql
// and premierLeagueClubs from drifting apart. Promotion and relegation churn
// about three clubs a season; before this, a club added to one and not the
// other silently lost its crest and league, or never seeded at all.
func TestSeedSQLMatchesRoster(t *testing.T) {
	const seedPath = "../../seed/seed.sql"

	raw, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("reading %s: %v", seedPath, err)
	}

	want := seedClubsSQL()
	if !strings.Contains(string(raw), want) {
		t.Errorf("%s does not contain the club INSERT generated from premierLeagueClubs.\n"+
			"Replace that statement in the file with:\n\n%s\n", seedPath, want)
	}
}

func TestSeedClubsSQLEscapesQuotes(t *testing.T) {
	// The roster has no apostrophes today, so exercise the escaping directly
	// rather than waiting for a club like "Nott'm Forest" to break the seed.
	if got, want := quoteSQL("Nott'm Forest"), "'Nott''m Forest'"; got != want {
		t.Errorf("quoteSQL = %s, want %s", got, want)
	}
}

func TestEveryRosterClubHasCrestAndShortName(t *testing.T) {
	for _, club := range premierLeagueClubs {
		if club.ShortName == "" {
			t.Errorf("%s has no short name", club.Name)
		}
		if !strings.HasPrefix(club.CrestURL, "https://upload.wikimedia.org/") {
			t.Errorf("%s crest URL is %q, want an upload.wikimedia.org URL", club.Name, club.CrestURL)
		}
	}
}

func TestCanonicalClubName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"canonical name is unchanged", "Tottenham Hotspur", "Tottenham Hotspur"},
		{"canonical name is case-insensitive", "tottenham hotspur", "Tottenham Hotspur"},
		{"common alias", "Spurs", "Tottenham Hotspur"},
		{"alias is case-insensitive", "man utd", "Manchester United"},
		{"abbreviation with different spacing", "  Man   City  ", "Manchester City"},
		{"and-vs-ampersand variant", "Brighton and Hove Albion", "Brighton & Hove Albion"},
		{"trailing FC is dropped", "Arsenal FC", "Arsenal"},
		{"trailing F.C. is dropped", "Everton F.C.", "Everton"},
		{"leading AFC is dropped", "AFC Bournemouth", "Bournemouth"},
		// The safety property: anything we do not recognise keeps its own
		// identity rather than being snapped to the nearest roster club.
		{"unknown club passes through", "Real Madrid", "Real Madrid"},
		{"unknown club keeps its affix", "Racing Club FC", "Racing Club FC"},
		{"unknown club is still tidied", "  Real   Madrid ", "Real Madrid"},
		{"empty stays empty", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalClubName(tt.input); got != tt.want {
				t.Errorf("canonicalClubName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Every alias must resolve to a club that is actually on the roster.
// Without this, an alias pointing at a name we do not carry would quietly
// create a differently-named club row — the exact problem aliases exist to
// prevent.
func TestEveryAliasTargetsARosterClub(t *testing.T) {
	for alias, canonical := range clubAliases {
		if _, ok := rosterByLowerName[strings.ToLower(canonical)]; !ok {
			t.Errorf("alias %q maps to %q, which is not in premierLeagueClubs", alias, canonical)
		}
		if alias != strings.ToLower(alias) {
			t.Errorf("alias %q must be lower-cased; lookups are case-folded", alias)
		}
		if canonicalClubName(alias) != canonical {
			t.Errorf("canonicalClubName(%q) = %q, want %q", alias, canonicalClubName(alias), canonical)
		}
	}
}
