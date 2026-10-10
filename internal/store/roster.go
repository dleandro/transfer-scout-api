package store

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed roster/*.json
var rosterFiles embed.FS

type rosterLeague struct {
	Name      string       `json:"name"`
	ShortName string       `json:"short_name"`
	Clubs     []rosterClub `json:"clubs"`
}

type rosterClub struct {
	Name      string   `json:"name"`
	ShortName string   `json:"short_name"`
	CrestURL  string   `json:"crest_url"`
	Aliases   []string `json:"aliases,omitempty"`
}

type rosterEntry struct {
	rosterClub
	League string
}

type roster struct {
	leagues []rosterLeague
	byName  map[string]rosterEntry
	byAlias map[string]rosterEntry
}

var clubRoster = mustLoadRoster(rosterFiles)

func mustLoadRoster(fsys fs.FS) *roster {
	r, err := loadRoster(fsys)
	if err != nil {
		panic(err)
	}
	return r
}

var allowedCrestPrefixes = []string{
	"https://upload.wikimedia.org/",
	"https://en.wikipedia.org/",
}

func loadRoster(fsys fs.FS) (*roster, error) {
	files, err := fs.Glob(fsys, "roster/*.json")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("roster: no league files")
	}

	r := &roster{byName: map[string]rosterEntry{}, byAlias: map[string]rosterEntry{}}
	leagueNames := map[string]string{}
	aliasClaims := map[string][]rosterEntry{}

	for _, file := range files {
		league, err := decodeRosterLeague(fsys, file)
		if err != nil {
			return nil, err
		}
		if prior, dup := leagueNames[strings.ToLower(league.Name)]; dup {
			return nil, fmt.Errorf("roster: league %q is defined in both %s and %s", league.Name, prior, file)
		}
		leagueNames[strings.ToLower(league.Name)] = file

		for _, club := range league.Clubs {
			entry := rosterEntry{rosterClub: club, League: league.Name}
			key := clubKey(club.Name)
			if prior, dup := r.byName[key]; dup {
				return nil, fmt.Errorf("roster: club %q (%s) collides with %q (%s)", club.Name, league.Name, prior.Name, prior.League)
			}
			r.byName[key] = entry

			seen := map[string]bool{}
			for _, alias := range club.Aliases {
				aliasKey := clubKey(alias)
				if aliasKey == "" || aliasKey == key || seen[aliasKey] {
					return nil, fmt.Errorf("roster: club %q has an empty, repeated or redundant alias %q", club.Name, alias)
				}
				seen[aliasKey] = true
				aliasClaims[aliasKey] = append(aliasClaims[aliasKey], entry)
			}
		}
		r.leagues = append(r.leagues, league)
	}

	for aliasKey, claims := range aliasClaims {
		if owner, isName := r.byName[aliasKey]; isName {
			return nil, fmt.Errorf("roster: alias %q of %q is the name of %q", aliasKey, claims[0].Name, owner.Name)
		}
		if len(claims) == 1 {
			r.byAlias[aliasKey] = claims[0]
		}
	}

	sort.Slice(r.leagues, func(i, j int) bool { return r.leagues[i].Name < r.leagues[j].Name })
	return r, nil
}

func decodeRosterLeague(fsys fs.FS, file string) (rosterLeague, error) {
	raw, err := fs.ReadFile(fsys, file)
	if err != nil {
		return rosterLeague{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var league rosterLeague
	if err := decoder.Decode(&league); err != nil {
		return rosterLeague{}, fmt.Errorf("roster: %s: %w", path.Base(file), err)
	}
	if league.Name == "" || league.ShortName == "" || len(league.Clubs) == 0 {
		return rosterLeague{}, fmt.Errorf("roster: %s needs a name, a short_name and clubs", path.Base(file))
	}
	for _, club := range league.Clubs {
		if club.Name == "" || club.ShortName == "" {
			return rosterLeague{}, fmt.Errorf("roster: %s has a club without a name or short_name: %+v", path.Base(file), club)
		}
		if club.Name != tidyClubName(club.Name) {
			return rosterLeague{}, fmt.Errorf("roster: %s: club name %q has stray whitespace", path.Base(file), club.Name)
		}
		if club.CrestURL != "" && !hasAnyPrefix(club.CrestURL, allowedCrestPrefixes) {
			return rosterLeague{}, fmt.Errorf("roster: %s: %s crest %q is not a Wikipedia/Wikimedia URL", path.Base(file), club.Name, club.CrestURL)
		}
	}
	return league, nil
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func (r *roster) lookup(name string) (rosterEntry, bool) {
	key := clubKey(name)
	if key == "" {
		return rosterEntry{}, false
	}
	if entry, ok := r.byName[key]; ok {
		return entry, true
	}
	if entry, ok := r.byAlias[key]; ok {
		return entry, true
	}
	if entry, ok := r.byName[clubKey(stripClubAffixes(tidyClubName(name)))]; ok {
		return entry, true
	}
	return rosterEntry{}, false
}

func (r *roster) canonicalName(name string) string {
	if entry, ok := r.lookup(name); ok {
		return entry.Name
	}
	return tidyClubName(name)
}

func tidyClubName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

func clubKey(name string) string {
	return foldDiacritics.Replace(strings.ToLower(tidyClubName(name)))
}

var foldDiacritics = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n", "ý", "y", "ÿ", "y",
	"ß", "ss",
)

func stripClubAffixes(name string) string {
	upper := strings.ToUpper(name)
	for _, suffix := range []string{" F.C.", " FC", " AFC"} {
		if strings.HasSuffix(upper, suffix) {
			return strings.TrimSpace(name[:len(name)-len(suffix)])
		}
	}
	for _, prefix := range []string{"AFC ", "FC "} {
		if strings.HasPrefix(upper, prefix) {
			return strings.TrimSpace(name[len(prefix):])
		}
	}
	return name
}
