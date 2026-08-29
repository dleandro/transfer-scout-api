package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// pgForeignKeyViolation is the Postgres error code for a foreign key
// constraint violation. See https://www.postgresql.org/docs/current/errcodes-appendix.html
const pgForeignKeyViolation = "23503"

// ErrRumourNotFound is returned by CreateComment when rumour_id doesn't
// reference a real rumour, so the handler can map it to a 404 instead of
// a generic 500.
var ErrRumourNotFound = errors.New("store: rumour not found")

const commentColumns = "comments.id, comments.rumour_id, comments.body, comments.created_at, comments.updated_at, users.id, users.display_name, users.avatar_url"

func scanComment(row pgxScanner, c *models.Comment) error {
	return row.Scan(&c.ID, &c.RumourID, &c.Body, &c.CreatedAt, &c.UpdatedAt,
		&c.Author.ID, &c.Author.DisplayName, &c.Author.AvatarURL)
}

// CreateComment inserts a comment and returns it with its author's public
// profile joined in.
func (s *Store) CreateComment(ctx context.Context, rumourID, userID uuid.UUID, body string) (*models.Comment, error) {
	row := s.Pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO comments (rumour_id, user_id, body)
			VALUES ($1, $2, $3)
			RETURNING id, rumour_id, user_id, body, created_at, updated_at
		)
		SELECT `+commentColumns+`
		FROM inserted AS comments
		JOIN users ON users.id = comments.user_id`, rumourID, userID, body)

	var c models.Comment
	if err := scanComment(row, &c); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return nil, ErrRumourNotFound
		}
		return nil, fmt.Errorf("store: create comment: %w", err)
	}
	return &c, nil
}

// ListComments returns a rumour's comments, oldest first (chronological
// reading order for a discussion thread), one page at a time. hasMore
// uses the same limit+1-and-trim trick as ListRumours.
func (s *Store) ListComments(ctx context.Context, rumourID uuid.UUID, limit, offset int) (comments []models.Comment, hasMore bool, err error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+commentColumns+`
		FROM comments
		JOIN users ON users.id = comments.user_id
		WHERE comments.rumour_id = $1
		ORDER BY comments.created_at ASC
		LIMIT $2 OFFSET $3`, rumourID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	for rows.Next() {
		var c models.Comment
		if err := scanComment(rows, &c); err != nil {
			return nil, false, err
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	if len(comments) > limit {
		comments = comments[:limit]
		hasMore = true
	}
	return comments, hasMore, nil
}
