package store

import (
	"context"
	"strings"

	"github.com/google/uuid"
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
