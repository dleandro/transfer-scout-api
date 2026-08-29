package store

import (
	"context"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

const userColumns = "id, email, display_name, avatar_url, created_at, updated_at"

func scanUser(row pgxScanner, u *models.User) error {
	return row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
}

// UpsertUser creates a user on first Google sign-in, or refreshes their
// profile fields (email/display name/avatar can all change on Google's
// side) on subsequent sign-ins — keyed on googleSub, Google's durable
// subject identifier, not email.
func (s *Store) UpsertUser(ctx context.Context, googleSub, email, displayName, avatarURL string) (*models.User, error) {
	var avatar *string
	if avatarURL != "" {
		avatar = &avatarURL
	}

	row := s.Pool.QueryRow(ctx, `
		INSERT INTO users (google_sub, email, display_name, avatar_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (google_sub) DO UPDATE SET
			email = EXCLUDED.email,
			display_name = EXCLUDED.display_name,
			avatar_url = EXCLUDED.avatar_url,
			updated_at = now()
		RETURNING `+userColumns, googleSub, email, displayName, avatar)

	var u models.User
	if err := scanUser(row, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
