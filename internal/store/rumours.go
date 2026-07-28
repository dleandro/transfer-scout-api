package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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

// RumourFeedItem is a rumour joined with the player/club names and crests
// needed to render it without N+1 lookups per row.
type RumourFeedItem struct {
	models.Rumour
	PlayerName    string
	ToClubName    string
	ToClubCrest   *string
	FromClubName  *string
	FromClubCrest *string
}

const rumourFeedSelect = `
	SELECT r.id, r.player_id, r.from_club_id, r.to_club_id, r.transfer_window, r.status,
	       r.fee_min_eur, r.fee_max_eur, r.summary, r.confidence, r.created_at, r.updated_at,
	       p.name, tc.name, tc.crest_url, fc.name, fc.crest_url
	FROM rumours r
	JOIN players p ON p.id = r.player_id
	JOIN clubs tc ON tc.id = r.to_club_id
	LEFT JOIN clubs fc ON fc.id = r.from_club_id`

func scanRumourFeedItem(row pgxScanner, item *RumourFeedItem) error {
	return row.Scan(&item.ID, &item.PlayerID, &item.FromClubID, &item.ToClubID, &item.TransferWindow, &item.Status,
		&item.FeeMinEUR, &item.FeeMaxEUR, &item.Summary, &item.Confidence, &item.CreatedAt, &item.UpdatedAt,
		&item.PlayerName, &item.ToClubName, &item.ToClubCrest, &item.FromClubName, &item.FromClubCrest)
}

// ListRumours returns the most recently updated rumours, enriched with
// player/club names and crests.
func (s *Store) ListRumours(ctx context.Context, limit, offset int) ([]RumourFeedItem, error) {
	rows, err := s.Pool.Query(ctx, rumourFeedSelect+`
		ORDER BY r.updated_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []RumourFeedItem
	for rows.Next() {
		var item RumourFeedItem
		if err := scanRumourFeedItem(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// RumourEventItem is a rumour_event joined with the source name and
// article URL/title needed to render a timeline entry.
type RumourEventItem struct {
	models.RumourEvent
	SourceName   string
	ArticleURL   string
	ArticleTitle string
}

// GetRumourByID returns a single rumour (enriched with player/club
// names and crests) and its full event timeline, oldest first, each
// event enriched with its source name and article URL/title.
func (s *Store) GetRumourByID(ctx context.Context, id uuid.UUID) (*RumourFeedItem, []RumourEventItem, error) {
	var item RumourFeedItem
	if err := scanRumourFeedItem(s.Pool.QueryRow(ctx, rumourFeedSelect+` WHERE r.id = $1`, id), &item); err != nil {
		return nil, nil, err
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT e.id, e.rumour_id, e.article_id, e.source_id, e.status,
		       e.fee_min_eur, e.fee_max_eur, e.summary, e.confidence, e.created_at,
		       s.name, a.url, a.title
		FROM rumour_events e
		JOIN sources s ON s.id = e.source_id
		JOIN articles a ON a.id = e.article_id
		WHERE e.rumour_id = $1
		ORDER BY e.created_at`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var events []RumourEventItem
	for rows.Next() {
		var ev RumourEventItem
		if err := rows.Scan(&ev.ID, &ev.RumourID, &ev.ArticleID, &ev.SourceID, &ev.Status,
			&ev.FeeMinEUR, &ev.FeeMaxEUR, &ev.Summary, &ev.Confidence, &ev.CreatedAt,
			&ev.SourceName, &ev.ArticleURL, &ev.ArticleTitle); err != nil {
			return nil, nil, err
		}
		events = append(events, ev)
	}
	return &item, events, rows.Err()
}

// UpsertRumourParams are the inputs to UpsertRumour, derived from a single
// article's extraction result.
type UpsertRumourParams struct {
	PlayerID       uuid.UUID
	FromClubID     *uuid.UUID
	ToClubID       uuid.UUID
	TransferWindow string
	Status         models.RumourStatus
	FeeMinEUR      *float64
	FeeMaxEUR      *float64
	Summary        *string
	Confidence     *float64
}

// UpsertRumour creates or updates the rumour thread for
// (player_id, to_club_id, transfer_window). Status only ever moves forward
// (models.RumourStatus.IsForwardTransition — a later "talks" report never
// rolls back an "advanced" rumour); the fee range widens to span every
// reported figure (LEAST/GREATEST, which Postgres treats NULL as "ignore"
// for); summary/confidence take the latest report.
//
// Not wrapped in a transaction: safe under cmd/extract's current
// single-process, sequential-batch execution model (articles are
// processed one at a time in a loop), but a second extraction worker
// running concurrently against the same player/club/window could race on
// the read-then-decide-status step. Add a `SELECT ... FOR UPDATE`
// transaction here first if extraction is ever parallelized.
func (s *Store) UpsertRumour(ctx context.Context, p UpsertRumourParams) (models.Rumour, error) {
	var existing models.Rumour
	err := scanRumour(s.Pool.QueryRow(ctx, `
		SELECT `+rumourColumns+` FROM rumours
		WHERE player_id = $1 AND to_club_id = $2 AND transfer_window = $3`,
		p.PlayerID, p.ToClubID, p.TransferWindow), &existing)

	finalStatus := p.Status
	switch {
	case err == nil:
		if !existing.Status.IsForwardTransition(p.Status) {
			finalStatus = existing.Status
		}
	case errors.Is(err, pgx.ErrNoRows):
		// No existing rumour — nothing to compare against.
	default:
		return models.Rumour{}, err
	}

	var r models.Rumour
	err = scanRumour(s.Pool.QueryRow(ctx, `
		INSERT INTO rumours (player_id, from_club_id, to_club_id, transfer_window, status, fee_min_eur, fee_max_eur, summary, confidence)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (player_id, to_club_id, transfer_window) DO UPDATE SET
			from_club_id = COALESCE(EXCLUDED.from_club_id, rumours.from_club_id),
			status = $5,
			fee_min_eur = LEAST(rumours.fee_min_eur, EXCLUDED.fee_min_eur),
			fee_max_eur = GREATEST(rumours.fee_max_eur, EXCLUDED.fee_max_eur),
			summary = EXCLUDED.summary,
			confidence = EXCLUDED.confidence,
			updated_at = now()
		RETURNING `+rumourColumns,
		p.PlayerID, p.FromClubID, p.ToClubID, p.TransferWindow, finalStatus,
		p.FeeMinEUR, p.FeeMaxEUR, p.Summary, p.Confidence), &r)
	if err != nil {
		return models.Rumour{}, err
	}
	return r, nil
}

// RumourEventParams are the inputs to InsertRumourEvent.
type RumourEventParams struct {
	RumourID   uuid.UUID
	ArticleID  uuid.UUID
	SourceID   uuid.UUID
	Status     models.RumourStatus
	FeeMinEUR  *float64
	FeeMaxEUR  *float64
	Summary    *string
	Confidence *float64
}

// InsertRumourEvent appends a timeline event. Idempotent per
// (rumour_id, article_id) — re-running extraction for the same article
// does not duplicate its event.
func (s *Store) InsertRumourEvent(ctx context.Context, p RumourEventParams) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO rumour_events (rumour_id, article_id, source_id, status, fee_min_eur, fee_max_eur, summary, confidence)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (rumour_id, article_id) DO NOTHING`,
		p.RumourID, p.ArticleID, p.SourceID, p.Status, p.FeeMinEUR, p.FeeMaxEUR, p.Summary, p.Confidence)
	return err
}
