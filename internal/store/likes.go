package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// LikeRumour records userID's like of rumourID. Idempotent: liking an
// already-liked rumour is a no-op, and re-liking a previously unliked one
// restores the same row (via ON CONFLICT) rather than erroring on the
// (rumour_id, user_id) unique constraint.
func (s *Store) LikeRumour(ctx context.Context, rumourID, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO likes (rumour_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (rumour_id, user_id) DO UPDATE SET deleted_at = NULL`,
		rumourID, userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return ErrRumourNotFound
		}
		return fmt.Errorf("store: like rumour: %w", err)
	}
	return nil
}

// UnlikeRumour soft-deletes userID's like of rumourID, if any — a no-op
// (not an error) when the rumour was never liked or the like is already
// removed, matching DELETE's idempotency.
func (s *Store) UnlikeRumour(ctx context.Context, rumourID, userID uuid.UUID) error {
	exists, err := s.RumourExists(ctx, rumourID)
	if err != nil {
		return fmt.Errorf("store: unlike rumour: %w", err)
	}
	if !exists {
		return ErrRumourNotFound
	}

	if _, err := s.Pool.Exec(ctx, `
		UPDATE likes SET deleted_at = now()
		WHERE rumour_id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		rumourID, userID); err != nil {
		return fmt.Errorf("store: unlike rumour: %w", err)
	}
	return nil
}
