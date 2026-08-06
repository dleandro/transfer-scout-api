package api

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// fakeStore implements Store for tests, without a real Postgres
// connection — mirrors the fake-backed pattern used in internal/ingest.
type fakeStore struct {
	rumours []models.Rumour
	hasMore bool
	listErr error

	rumour *models.Rumour
	events []models.RumourEvent
	getErr error

	pingErr error

	user          *models.User
	upsertUserErr error

	// captured args, so tests can assert what the handler actually passed
	// through to the store (e.g. clamped pagination values).
	gotLimit, gotOffset int
	gotID               uuid.UUID
}

var errStoreUnavailable = errors.New("store: unavailable")

func (f *fakeStore) ListRumours(ctx context.Context, limit, offset int) ([]models.Rumour, bool, error) {
	f.gotLimit, f.gotOffset = limit, offset
	if f.listErr != nil {
		return nil, false, f.listErr
	}
	return f.rumours, f.hasMore, nil
}

func (f *fakeStore) GetRumourByID(ctx context.Context, id uuid.UUID) (*models.Rumour, []models.RumourEvent, error) {
	f.gotID = id
	if f.getErr != nil {
		return nil, nil, f.getErr
	}
	return f.rumour, f.events, nil
}

func (f *fakeStore) Ping(ctx context.Context) error {
	return f.pingErr
}

func (f *fakeStore) UpsertUser(ctx context.Context, googleSub, email, displayName, avatarURL string) (*models.User, error) {
	if f.upsertUserErr != nil {
		return nil, f.upsertUserErr
	}
	return f.user, nil
}
