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

	clubs      []store.ClubFeedItem
	clubsErr   error
	players    []models.Player
	playersErr error
	leagues    []models.League
	leaguesErr error

	likeRumourErr   error
	unlikeRumourErr error

	followClubErr   error
	unfollowClubErr error

	// captured args, so tests can assert what the handler actually passed
	// through to the store (e.g. clamped pagination values).
	gotLimit, gotOffset                  int
	gotFilter                            store.RumourFilter
	gotViewerID                          *uuid.UUID
	gotClubsViewerID                     *uuid.UUID
	gotClubsLeagueID                     *uuid.UUID
	gotID                                uuid.UUID
	gotCommentBody                       string
	gotCommentUserID                     uuid.UUID
	gotCommentsLimit, gotCommentsOffset  int
	gotLikeRumourID, gotLikeUserID       uuid.UUID
	gotUnlikeRumourID, gotUnlikeUserID   uuid.UUID
	gotFollowClubID, gotFollowUserID     uuid.UUID
	gotUnfollowClubID, gotUnfollowUserID uuid.UUID
}

var errStoreUnavailable = errors.New("store: unavailable")

func (f *fakeStore) ListRumours(ctx context.Context, limit, offset int, filter store.RumourFilter, viewerID *uuid.UUID) ([]store.RumourFeedItem, bool, error) {
	f.gotLimit, f.gotOffset, f.gotFilter, f.gotViewerID = limit, offset, filter, viewerID
	if f.listErr != nil {
		return nil, false, f.listErr
	}
	return f.rumours, f.hasMore, nil
}

func (f *fakeStore) ListClubs(ctx context.Context, viewerID, leagueID *uuid.UUID) ([]store.ClubFeedItem, error) {
	f.gotClubsViewerID, f.gotClubsLeagueID = viewerID, leagueID
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

func (f *fakeStore) ListLeagues(ctx context.Context) ([]models.League, error) {
	if f.leaguesErr != nil {
		return nil, f.leaguesErr
	}
	return f.leagues, nil
}

func (f *fakeStore) GetRumourByID(ctx context.Context, id uuid.UUID, viewerID *uuid.UUID) (*store.RumourFeedItem, []store.RumourEventItem, error) {
	f.gotID, f.gotViewerID = id, viewerID
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

func (f *fakeStore) LikeRumour(ctx context.Context, rumourID, userID uuid.UUID) error {
	f.gotLikeRumourID, f.gotLikeUserID = rumourID, userID
	return f.likeRumourErr
}

func (f *fakeStore) UnlikeRumour(ctx context.Context, rumourID, userID uuid.UUID) error {
	f.gotUnlikeRumourID, f.gotUnlikeUserID = rumourID, userID
	return f.unlikeRumourErr
}

func (f *fakeStore) FollowClub(ctx context.Context, userID, clubID uuid.UUID) error {
	f.gotFollowUserID, f.gotFollowClubID = userID, clubID
	return f.followClubErr
}

func (f *fakeStore) UnfollowClub(ctx context.Context, userID, clubID uuid.UUID) error {
	f.gotUnfollowUserID, f.gotUnfollowClubID = userID, clubID
	return f.unfollowClubErr
}
