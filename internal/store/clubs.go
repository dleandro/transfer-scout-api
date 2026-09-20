package store

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// GetOrCreateClub returns the ID of the club matching name
// (case-insensitive), creating a new row if none exists. Note: this is an
// exact match on name, not fuzzy/alias matching — "Man United" and
// "Manchester United" would create two separate club rows. Revisit with an
// alias table if extraction output turns out to vary enough to matter.
//
// It also stamps crest_url from crestURLFor (the single source of truth for
// crests): on insert for a newly created club, and on conflict to backfill
// a row that pre-dates the crest (e.g. the clubs from seed/seed.sql). The
// COALESCE keeps an existing crest when the club is not in the map, so a
// club dropping out of the map never nulls a crest that is already set.
//
// league_id is stamped the same way, reusing the same "is this club in
// clubCrests" check rather than a second map — every club that map knows
// about is a Premier League club for now. A club outside it (e.g. a
// foreign club a rumour mentions only as the "from" side, like Real Madrid
// in a Real Madrid -> PL move) gets no crest and no league — it stays NULL
// rather than defaulting to the PL, which would misrepresent it.
func (s *Store) GetOrCreateClub(ctx context.Context, name string) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	isKnownPLClub := crestURLFor(name) != nil
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO clubs (name, crest_url, league_id)
		VALUES ($1, $2, CASE WHEN $3 THEN (SELECT id FROM leagues WHERE lower(name) = 'premier league') END)
		ON CONFLICT (lower(name)) DO UPDATE
		SET crest_url = COALESCE(EXCLUDED.crest_url, clubs.crest_url),
		    league_id = COALESCE(EXCLUDED.league_id, clubs.league_id)
		RETURNING id`, name, crestURLFor(name), isKnownPLClub).Scan(&id)
	return id, err
}

// ClubFeedItem is a club plus whether the viewer passed to ListClubs
// follows it — mirrors RumourFeedItem's LikedByMe.
type ClubFeedItem struct {
	models.Club
	// FollowedByMe reports whether the viewer passed to ListClubs actively
	// follows this club. Always false for a nil (anonymous) viewer.
	FollowedByMe bool `json:"followed_by_me"`
	// LeagueName is the joined leagues.name for this club's league_id, nil
	// when the club has none — same "enrich via the feed item, not the
	// plain model" convention as RumourFeedItem.ToClubName.
	LeagueName *string `json:"league_name,omitempty"`
}

// ListClubs returns every club, alphabetically by name, plus whether
// viewerID (nil for an anonymous caller) follows each one. leagueID, when
// non-nil, narrows the result to clubs in that league. Unbounded (no
// pagination) — fine at current single-window PL scale (20 clubs); revisit
// if this ever spans multiple windows/leagues.
func (s *Store) ListClubs(ctx context.Context, viewerID *uuid.UUID, leagueID *uuid.UUID) ([]ClubFeedItem, error) {
	query := `
		SELECT c.id, c.name, c.short_name, c.crest_url, c.league_id, c.created_at,
		       EXISTS (SELECT 1 FROM follows f
		                WHERE f.club_id = c.id AND f.user_id = $1 AND f.deleted_at IS NULL) AS followed_by_me,
		       l.name AS league_name
		FROM clubs c
		LEFT JOIN leagues l ON l.id = c.league_id`
	args := []any{viewerID}
	if leagueID != nil {
		query += "\n\tWHERE c.league_id = $2"
		args = append(args, *leagueID)
	}
	query += "\n\tORDER BY c.name"

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clubs []ClubFeedItem
	for rows.Next() {
		var c ClubFeedItem
		if err := rows.Scan(&c.ID, &c.Name, &c.ShortName, &c.CrestURL, &c.LeagueID, &c.CreatedAt, &c.FollowedByMe, &c.LeagueName); err != nil {
			return nil, err
		}
		clubs = append(clubs, c)
	}
	return clubs, rows.Err()
}

// ClubExists reports whether a club with this id exists — mirrors
// RumourExists, used by FollowClub/UnfollowClub-adjacent 404 checks
// without paying for a full club fetch.
func (s *Store) ClubExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM clubs WHERE id = $1)`, id).Scan(&exists)
	return exists, err
}
