package store

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// GetOrCreatePlayer returns the ID of the player matching name
// (case-insensitive), creating a new row if none exists. See the
// migration 0003 comment for the known name-collision limitation.
func (s *Store) GetOrCreatePlayer(ctx context.Context, name string) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO players (name) VALUES ($1)
		ON CONFLICT (lower(name)) DO UPDATE SET name = players.name
		RETURNING id`, name).Scan(&id)
	return id, err
}

// ListPlayers returns every player, alphabetically by name. Unbounded (no
// pagination) — same reasoning as ListClubs.
func (s *Store) ListPlayers(ctx context.Context) ([]models.Player, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, name, current_club_id, created_at
		FROM players
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []models.Player
	for rows.Next() {
		var p models.Player
		if err := rows.Scan(&p.ID, &p.Name, &p.CurrentClubID, &p.CreatedAt); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}
