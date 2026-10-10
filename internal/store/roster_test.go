package store

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func rosterFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys["roster/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func TestRosterLoadsEveryLeagueFile(t *testing.T) {
	files, err := rosterFiles.ReadDir("roster")
	if err != nil {
		t.Fatalf("read embedded roster: %v", err)
	}
	if len(clubRoster.leagues) != len(files) {
		t.Fatalf("loaded %d leagues from %d files", len(clubRoster.leagues), len(files))
	}

	for _, league := range clubRoster.leagues {
		if n := len(league.Clubs); n < 10 || n > 40 {
			t.Errorf("%s has %d clubs, want between 10 and 40", league.Name, n)
		}
		for _, club := range league.Clubs {
			entry, ok := clubRoster.lookup(club.Name)
			if !ok || entry.Name != club.Name || entry.League != league.Name {
				t.Errorf("%s does not resolve to itself in %s: got %+v, %v", club.Name, league.Name, entry, ok)
			}
		}
	}
}

func TestRosterCarriesTheOwnerLeagues(t *testing.T) {
	want := []string{
		"Premier League", "La Liga", "Serie A", "Bundesliga", "Ligue 1", "Scottish Premiership",
		"Liga Portugal", "Eredivisie", "Saudi Pro League", "Major League Soccer",
		"Brasileirão Série A", "Argentine Primera División",
	}
	loaded := map[string]bool{}
	for _, league := range clubRoster.leagues {
		loaded[league.Name] = true
	}
	for _, name := range want {
		if !loaded[name] {
			t.Errorf("league %q is not in the roster", name)
		}
	}
}

func TestLoadRosterRejectsInvalidFiles(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			"same club name in two leagues",
			map[string]string{
				"a.json": `{"name": "Liga Portugal", "short_name": "LP", "clubs": [{"name": "Nacional", "short_name": "NAC", "crest_url": ""}]}`,
				"b.json": `{"name": "Uruguayan Primera División", "short_name": "UPD", "clubs": [{"name": "Nacional", "short_name": "NAC", "crest_url": ""}]}`,
			},
			"collides",
		},
		{
			"names differing only by case and accents",
			map[string]string{
				"a.json": `{"name": "La Liga", "short_name": "LL", "clubs": [{"name": "Atlético Madrid", "short_name": "ATM", "crest_url": ""}]}`,
				"b.json": `{"name": "Other", "short_name": "OT", "clubs": [{"name": "atletico madrid", "short_name": "ATM", "crest_url": ""}]}`,
			},
			"collides",
		},
		{
			"same league in two files",
			map[string]string{
				"a.json": `{"name": "Serie A", "short_name": "SA", "clubs": [{"name": "Roma", "short_name": "ROM", "crest_url": ""}]}`,
				"b.json": `{"name": "serie a", "short_name": "SA", "clubs": [{"name": "Lazio", "short_name": "LAZ", "crest_url": ""}]}`,
			},
			"defined in both",
		},
		{
			"alias that is another club's name",
			map[string]string{
				"a.json": `{"name": "Serie A", "short_name": "SA", "clubs": [
					{"name": "Roma", "short_name": "ROM", "crest_url": "", "aliases": ["Lazio"]},
					{"name": "Lazio", "short_name": "LAZ", "crest_url": ""}]}`,
			},
			"is the name of",
		},
		{
			"alias that only repeats the club's own name",
			map[string]string{
				"a.json": `{"name": "La Liga", "short_name": "LL", "clubs": [{"name": "Atlético Madrid", "short_name": "ATM", "crest_url": "", "aliases": ["Atletico Madrid"]}]}`,
			},
			"redundant alias",
		},
		{
			"unknown field",
			map[string]string{
				"a.json": `{"name": "Serie A", "short_name": "SA", "country": "Italy", "clubs": [{"name": "Roma", "short_name": "ROM", "crest_url": ""}]}`,
			},
			"unknown field",
		},
		{
			"crest outside Wikipedia",
			map[string]string{
				"a.json": `{"name": "Serie A", "short_name": "SA", "clubs": [{"name": "Roma", "short_name": "ROM", "crest_url": "https://example.com/roma.svg"}]}`,
			},
			"not a Wikipedia",
		},
		{
			"club without a short name",
			map[string]string{
				"a.json": `{"name": "Serie A", "short_name": "SA", "clubs": [{"name": "Roma", "crest_url": ""}]}`,
			},
			"without a name or short_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadRoster(rosterFS(tt.files))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("loadRoster error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestAmbiguousAliasDoesNotResolve(t *testing.T) {
	r, err := loadRoster(rosterFS(map[string]string{
		"italy.json": `{"name": "Serie A", "short_name": "SA", "clubs": [{"name": "Inter Milan", "short_name": "INT", "crest_url": "", "aliases": ["Inter", "Internazionale"]}]}`,
		"usa.json":   `{"name": "Major League Soccer", "short_name": "MLS", "clubs": [{"name": "Inter Miami", "short_name": "MIA", "crest_url": "", "aliases": ["Inter"]}]}`,
	}))
	if err != nil {
		t.Fatalf("loadRoster: %v", err)
	}

	if entry, ok := r.lookup("Inter"); ok {
		t.Errorf("lookup(Inter) resolved to %q, want no match", entry.Name)
	}
	if got := r.canonicalName("  inter "); got != "inter" {
		t.Errorf("canonicalName(inter) = %q, want the input left as written", got)
	}
	if got := r.canonicalName("Internazionale"); got != "Inter Milan" {
		t.Errorf("canonicalName(Internazionale) = %q, want Inter Milan", got)
	}
}

func TestRosterLeavesCrossLeagueAliasesUnresolved(t *testing.T) {
	for _, alias := range []string{"Inter", "Real", "Sporting", "Atletico", "Racing", "Athletic"} {
		if entry, ok := clubRoster.lookup(alias); ok {
			t.Errorf("%q resolved to %s (%s), want it ambiguous", alias, entry.Name, entry.League)
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
		{"leading FC is dropped", "FC Barcelona", "Barcelona"},
		{"La Liga alias", "Barca", "Barcelona"},
		{"accent-folded alias", "Barça", "Barcelona"},
		{"accents are optional", "Atletico Madrid", "Atlético Madrid"},
		{"Ligue 1 alias", "PSG", "Paris Saint-Germain"},
		{"Bundesliga alias", "Bayern", "Bayern Munich"},
		{"Serie A alias", "Juve", "Juventus"},
		{"Scottish alias", "Hibs", "Hibernian"},
		{"unknown club passes through", "Leyton Orient", "Leyton Orient"},
		{"unknown club keeps its affix", "Leyton Orient FC", "Leyton Orient FC"},
		{"unknown club is still tidied", "  Leyton   Orient ", "Leyton Orient"},
		{"empty stays empty", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clubRoster.canonicalName(tt.input); got != tt.want {
				t.Errorf("canonicalName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEveryAliasResolvesUnlessShared(t *testing.T) {
	claims := map[string]int{}
	for _, entry := range clubRoster.entries() {
		for _, alias := range entry.Aliases {
			claims[clubKey(alias)]++
		}
	}

	for _, entry := range clubRoster.entries() {
		for _, alias := range entry.Aliases {
			got, ok := clubRoster.lookup(alias)
			if claims[clubKey(alias)] > 1 {
				if ok {
					t.Errorf("alias %q is shared but resolved to %s", alias, got.Name)
				}
				continue
			}
			if !ok || got.Name != entry.Name {
				t.Errorf("alias %q of %s resolved to %q, %v", alias, entry.Name, got.Name, ok)
			}
		}
	}
}

func TestSeedSQLMatchesRoster(t *testing.T) {
	const seedPath = "../../seed/seed.sql"

	raw, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("reading %s: %v", seedPath, err)
	}

	want := rosterSeedSQL()
	if !strings.Contains(string(raw), want) {
		t.Errorf("%s does not contain the league and club statements generated from internal/store/roster.\n"+
			"Replace them in the file with:\n\n%s\n", seedPath, want)
	}
}

func TestQuoteSQLEscapesQuotes(t *testing.T) {
	if got, want := quoteSQL("Nott'm Forest"), "'Nott''m Forest'"; got != want {
		t.Errorf("quoteSQL = %s, want %s", got, want)
	}
}
