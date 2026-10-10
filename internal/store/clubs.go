package store

import (
	"context"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

func (s *Store) GetOrCreateClub(ctx context.Context, name string) (uuid.UUID, error) {
	var shortName, crestURL, league *string
	if entry, known := clubRoster.lookup(name); known {
		name = entry.Name
		shortName = &entry.ShortName
		league = &entry.League
		if entry.CrestURL != "" {
			crestURL = &entry.CrestURL
		}
	} else {
		name = tidyClubName(name)
	}

	var id uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO clubs (name, short_name, crest_url, league_id)
		VALUES ($1, $2, $3, (SELECT id FROM leagues WHERE lower(name) = lower($4)))
		ON CONFLICT (lower(name)) DO UPDATE
		SET short_name = COALESCE(EXCLUDED.short_name, clubs.short_name),
		    crest_url = COALESCE(EXCLUDED.crest_url, clubs.crest_url),
		    league_id = COALESCE(EXCLUDED.league_id, clubs.league_id)
		RETURNING id`, name, shortName, crestURL, league).Scan(&id)
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
