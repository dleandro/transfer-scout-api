package store

import (
	"context"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// ListLeagues returns every league, alphabetically by name. Unbounded (no
// pagination) — same reasoning as ListClubs/ListPlayers at current scale.
func (s *Store) ListLeagues(ctx context.Context) ([]models.League, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, name, short_name, created_at
		FROM leagues
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leagues []models.League
	for rows.Next() {
		var l models.League
		if err := rows.Scan(&l.ID, &l.Name, &l.ShortName, &l.CreatedAt); err != nil {
			return nil, err
		}
		leagues = append(leagues, l)
	}
	return leagues, rows.Err()
}
