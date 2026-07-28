package store

import (
	"context"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

const rumourColumns = `id, player_id, from_club_id, to_club_id, transfer_window, status,
		fee_min_eur, fee_max_eur, summary, confidence, created_at, updated_at`

func scanRumour(row pgxScanner, r *models.Rumour) error {
	return row.Scan(&r.ID, &r.PlayerID, &r.FromClubID, &r.ToClubID, &r.TransferWindow, &r.Status,
		&r.FeeMinEUR, &r.FeeMaxEUR, &r.Summary, &r.Confidence, &r.CreatedAt, &r.UpdatedAt)
}

// pgxScanner is satisfied by both pgx.Row and pgx.Rows.
type pgxScanner interface {
	Scan(dest ...any) error
}

// ListRumours returns the most recently updated rumours. Player and club
// names are not yet joined in — see milestone 1.5 for feed enrichment.
func (s *Store) ListRumours(ctx context.Context, limit, offset int) ([]models.Rumour, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+rumourColumns+`
		FROM rumours
		ORDER BY updated_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rumours []models.Rumour
	for rows.Next() {
		var r models.Rumour
		if err := scanRumour(rows, &r); err != nil {
			return nil, err
		}
		rumours = append(rumours, r)
	}
	return rumours, rows.Err()
}

// GetRumourByID returns a single rumour and its full event timeline,
// oldest event first.
func (s *Store) GetRumourByID(ctx context.Context, id uuid.UUID) (*models.Rumour, []models.RumourEvent, error) {
	var r models.Rumour
	row := s.Pool.QueryRow(ctx, `SELECT `+rumourColumns+` FROM rumours WHERE id = $1`, id)
	if err := scanRumour(row, &r); err != nil {
		return nil, nil, err
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT id, rumour_id, article_id, source_id, status, fee_min_eur, fee_max_eur, summary, confidence, created_at
		FROM rumour_events
		WHERE rumour_id = $1
		ORDER BY created_at`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var events []models.RumourEvent
	for rows.Next() {
		var ev models.RumourEvent
		if err := rows.Scan(&ev.ID, &ev.RumourID, &ev.ArticleID, &ev.SourceID, &ev.Status,
			&ev.FeeMinEUR, &ev.FeeMaxEUR, &ev.Summary, &ev.Confidence, &ev.CreatedAt); err != nil {
			return nil, nil, err
		}
		events = append(events, ev)
	}
	return &r, events, rows.Err()
}
