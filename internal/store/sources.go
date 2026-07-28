package store

import (
	"context"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// ListSources returns all news sources, including those without a feed_url
// yet (see milestone 1.2).
func (s *Store) ListSources(ctx context.Context) ([]models.Source, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, name, feed_url, reliability_score, created_at
		FROM sources
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []models.Source
	for rows.Next() {
		var src models.Source
		if err := rows.Scan(&src.ID, &src.Name, &src.FeedURL, &src.ReliabilityScore, &src.CreatedAt); err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}
	return sources, rows.Err()
}
