package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrClubNotFound is returned by FollowClub/UnfollowClub when club_id
// doesn't reference a real club, so the handler can map it to a 404
// instead of a generic 500.
var ErrClubNotFound = errors.New("store: club not found")

// FollowClub records userID's follow of clubID. Idempotent: following an
// already-followed club is a no-op, and re-following a previously
// unfollowed one restores the same row (via ON CONFLICT) rather than
// erroring on the (user_id, club_id) unique constraint. Mirrors LikeRumour.
func (s *Store) FollowClub(ctx context.Context, userID, clubID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO follows (user_id, club_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, club_id) DO UPDATE SET deleted_at = NULL`,
		userID, clubID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return ErrClubNotFound
		}
		return fmt.Errorf("store: follow club: %w", err)
	}
	return nil
}

// UnfollowClub soft-deletes userID's follow of clubID, if any — a no-op
// (not an error) when the club was never followed or the follow is
// already removed, matching DELETE's idempotency. Mirrors UnlikeRumour.
func (s *Store) UnfollowClub(ctx context.Context, userID, clubID uuid.UUID) error {
	exists, err := s.ClubExists(ctx, clubID)
	if err != nil {
		return fmt.Errorf("store: unfollow club: %w", err)
	}
	if !exists {
		return ErrClubNotFound
	}

	if _, err := s.Pool.Exec(ctx, `
		UPDATE follows SET deleted_at = now()
		WHERE user_id = $1 AND club_id = $2 AND deleted_at IS NULL`,
		userID, clubID); err != nil {
		return fmt.Errorf("store: unfollow club: %w", err)
	}
	return nil
}
