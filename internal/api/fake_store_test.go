package api

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// fakeStore implements Store for tests, without a real Postgres
// connection — mirrors the fake-backed pattern used in internal/ingest.
type fakeStore struct {
	rumours []store.RumourFeedItem
	hasMore bool
	listErr error

	rumour *store.RumourFeedItem
	events []store.RumourEventItem
	getErr error

	pingErr error

	user          *models.User
	upsertUserErr error

	rumourExists    bool
	rumourExistsErr error

	comments         []models.Comment
	commentsHasMore  bool
	listCommentsErr  error
	createdComment   *models.Comment
	createCommentErr error

	clubs      []models.Club
	clubsErr   error
	players    []models.Player
	playersErr error

	// captured args, so tests can assert what the handler actually passed
	// through to the store (e.g. clamped pagination values).
	gotLimit, gotOffset                 int
	gotFilter                           store.RumourFilter
	gotID                               uuid.UUID
	gotCommentBody                      string
	gotCommentUserID                    uuid.UUID
	gotCommentsLimit, gotCommentsOffset int
}

var errStoreUnavailable = errors.New("store: unavailable")

func (f *fakeStore) ListRumours(ctx context.Context, limit, offset int, filter store.RumourFilter) ([]store.RumourFeedItem, bool, error) {
	f.gotLimit, f.gotOffset, f.gotFilter = limit, offset, filter
	if f.listErr != nil {
		return nil, false, f.listErr
	}
	return f.rumours, f.hasMore, nil
}

func (f *fakeStore) ListClubs(ctx context.Context) ([]models.Club, error) {
	if f.clubsErr != nil {
		return nil, f.clubsErr
	}
	return f.clubs, nil
}

func (f *fakeStore) ListPlayers(ctx context.Context) ([]models.Player, error) {
	if f.playersErr != nil {
		return nil, f.playersErr
	}
	return f.players, nil
}

func (f *fakeStore) GetRumourByID(ctx context.Context, id uuid.UUID) (*store.RumourFeedItem, []store.RumourEventItem, error) {
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

func (f *fakeStore) RumourExists(ctx context.Context, id uuid.UUID) (bool, error) {
	if f.rumourExistsErr != nil {
		return false, f.rumourExistsErr
	}
	return f.rumourExists, nil
}

func (f *fakeStore) CreateComment(ctx context.Context, rumourID, userID uuid.UUID, body string) (*models.Comment, error) {
	f.gotID, f.gotCommentUserID, f.gotCommentBody = rumourID, userID, body
	if f.createCommentErr != nil {
		return nil, f.createCommentErr
	}
	return f.createdComment, nil
}

func (f *fakeStore) ListComments(ctx context.Context, rumourID uuid.UUID, limit, offset int) ([]models.Comment, bool, error) {
	f.gotID, f.gotCommentsLimit, f.gotCommentsOffset = rumourID, limit, offset
	if f.listCommentsErr != nil {
		return nil, false, f.listCommentsErr
	}
	return f.comments, f.commentsHasMore, nil
}
