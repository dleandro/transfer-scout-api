package store

import (
	"context"
	"strings"

	"github.com/google/uuid"
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
