package store

import "strings"

// clubCrests maps a club's canonical name (lower-cased) to a stable crest
// image URL. It is the single source of truth for club crests:
// GetOrCreateClub stamps the matching URL onto a club row when it creates
// the club, and backfills it onto an already-seeded row the next time that
// club is referenced (see the ON CONFLICT clause there). Both the curated
// clubs in seed/seed.sql and any club the clustering pipeline creates at
// runtime pick their crest up from here — the URLs are never duplicated
// into SQL.
//
// Keys must match the club names seeded in seed/seed.sql, compared
// case-insensitively. A club absent from this map keeps an empty crest_url
// (the API hides it via omitempty) rather than a guessed, broken URL.
//
// URLs are en.wikipedia upload.wikimedia.org file URLs — PL crests are
// non-free logos, hosted per-wiki rather than on Wikimedia Commons. Each
// was verified to resolve (HTTP 200) when added.
var clubCrests = map[string]string{
	"arsenal":                 "https://upload.wikimedia.org/wikipedia/en/5/53/Arsenal_FC.svg",
	"aston villa":             "https://upload.wikimedia.org/wikipedia/en/9/9a/Aston_Villa_FC_new_crest.svg",
	"bournemouth":             "https://upload.wikimedia.org/wikipedia/en/e/e5/AFC_Bournemouth_%282013%29.svg",
	"brentford":               "https://upload.wikimedia.org/wikipedia/en/2/2a/Brentford_FC_crest.svg",
	"brighton & hove albion":  "https://upload.wikimedia.org/wikipedia/en/d/d0/Brighton_and_Hove_Albion_FC_crest.svg",
	"burnley":                 "https://upload.wikimedia.org/wikipedia/en/6/6d/Burnley_FC_Logo.svg",
	"chelsea":                 "https://upload.wikimedia.org/wikipedia/en/c/cc/Chelsea_FC.svg",
	"crystal palace":          "https://upload.wikimedia.org/wikipedia/en/a/a2/Crystal_Palace_FC_logo_%282022%29.svg",
	"everton":                 "https://upload.wikimedia.org/wikipedia/en/7/7c/Everton_FC_logo.svg",
	"fulham":                  "https://upload.wikimedia.org/wikipedia/en/e/eb/Fulham_FC_%28shield%29.svg",
	"leeds united":            "https://upload.wikimedia.org/wikipedia/en/5/54/Leeds_United_F.C._logo.svg",
	"liverpool":               "https://upload.wikimedia.org/wikipedia/en/0/0c/Liverpool_FC.svg",
	"manchester city":         "https://upload.wikimedia.org/wikipedia/en/e/eb/Manchester_City_FC_badge.svg",
	"manchester united":       "https://upload.wikimedia.org/wikipedia/en/7/7a/Manchester_United_FC_crest.svg",
	"newcastle united":        "https://upload.wikimedia.org/wikipedia/en/5/56/Newcastle_United_Logo.svg",
	"nottingham forest":       "https://upload.wikimedia.org/wikipedia/en/e/e5/Nottingham_Forest_F.C._logo.svg",
	"sunderland":              "https://upload.wikimedia.org/wikipedia/en/7/77/Logo_Sunderland.svg",
	"tottenham hotspur":       "https://upload.wikimedia.org/wikipedia/en/b/b4/Tottenham_Hotspur.svg",
	"west ham united":         "https://upload.wikimedia.org/wikipedia/en/c/c2/West_Ham_United_FC_logo.svg",
	"wolverhampton wanderers": "https://upload.wikimedia.org/wikipedia/en/f/fc/Wolverhampton_Wanderers.svg",
}

// crestURLFor returns the crest URL for a club name, or nil when the club
// is not in clubCrests. The match is case-insensitive and whitespace-
// trimmed, mirroring GetOrCreateClub's own name handling. Returning nil
// (rather than "") lets callers pass the result straight through to the
// nullable crest_url column.
func crestURLFor(name string) *string {
	if url, ok := clubCrests[strings.ToLower(strings.TrimSpace(name))]; ok {
		return &url
	}
	return nil
}
