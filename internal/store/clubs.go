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
func (s *Store) GetOrCreateClub(ctx context.Context, name string) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO clubs (name) VALUES ($1)
		ON CONFLICT (lower(name)) DO UPDATE SET name = clubs.name
		RETURNING id`, name).Scan(&id)
	return id, err
}

// ListClubs returns every club, alphabetically by name. Unbounded (no
// pagination) — fine at current single-window PL scale (20 clubs); revisit
// if this ever spans multiple windows/leagues.
func (s *Store) ListClubs(ctx context.Context) ([]models.Club, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, name, short_name, crest_url, created_at
		FROM clubs
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clubs []models.Club
	for rows.Next() {
		var c models.Club
		if err := rows.Scan(&c.ID, &c.Name, &c.ShortName, &c.CrestURL, &c.CreatedAt); err != nil {
			return nil, err
		}
		clubs = append(clubs, c)
	}
	return clubs, rows.Err()
}
