package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// createTestClubAndUser inserts the minimal fixtures FollowClub/
// UnfollowClub/ListClubs need to FK against, returning their IDs.
func createTestClubAndUser(t *testing.T, s *store.Store) (clubID, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	if err := s.Pool.QueryRow(ctx, `INSERT INTO clubs (name) VALUES ($1) RETURNING id`,
		uniqueName("Follow Test Club")).Scan(&clubID); err != nil {
		t.Fatalf("insert test club: %v", err)
	}

	user, err := s.UpsertUser(ctx, "google-sub-"+uuid.NewString(), "follower@example.com", "Follower", "")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	return clubID, user.ID
}

func TestIntegration_FollowClub_IsIdempotentAndRestoresAfterUnfollow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	clubID, userID := createTestClubAndUser(t, s)

	if err := s.FollowClub(ctx, userID, clubID); err != nil {
		t.Fatalf("FollowClub: %v", err)
	}
	// Re-following an already-followed club is a no-op, not an error.
	if err := s.FollowClub(ctx, userID, clubID); err != nil {
		t.Fatalf("FollowClub (re-follow): %v", err)
	}

	clubs, err := s.ListClubs(ctx, &userID, nil)
	if err != nil {
		t.Fatalf("ListClubs: %v", err)
	}
	if !mustFindClub(t, clubs, clubID).FollowedByMe {
		t.Error("FollowedByMe = false, want true after FollowClub")
	}

	if err := s.UnfollowClub(ctx, userID, clubID); err != nil {
		t.Fatalf("UnfollowClub: %v", err)
	}
	clubs, err = s.ListClubs(ctx, &userID, nil)
	if err != nil {
		t.Fatalf("ListClubs after unfollow: %v", err)
	}
	if mustFindClub(t, clubs, clubID).FollowedByMe {
		t.Error("FollowedByMe = true, want false after UnfollowClub")
	}

	// Re-following after an unfollow restores the same row rather than
	// erroring on the (user_id, club_id) unique constraint.
	if err := s.FollowClub(ctx, userID, clubID); err != nil {
		t.Fatalf("FollowClub (after unfollow): %v", err)
	}
	clubs, err = s.ListClubs(ctx, &userID, nil)
	if err != nil {
		t.Fatalf("ListClubs after re-follow: %v", err)
	}
	if !mustFindClub(t, clubs, clubID).FollowedByMe {
		t.Error("FollowedByMe = false, want true after re-following")
	}
}

func TestIntegration_UnfollowClub_NeverFollowedIsANoOp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	clubID, userID := createTestClubAndUser(t, s)

	if err := s.UnfollowClub(ctx, userID, clubID); err != nil {
		t.Fatalf("UnfollowClub (never followed): %v", err)
	}
}

func TestIntegration_FollowClub_UnknownClubReturnsErrClubNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, userID := createTestClubAndUser(t, s)

	if err := s.FollowClub(ctx, userID, uuid.New()); !errors.Is(err, store.ErrClubNotFound) {
		t.Errorf("FollowClub against unknown club: err = %v, want ErrClubNotFound", err)
	}
}

func TestIntegration_UnfollowClub_UnknownClubReturnsErrClubNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, userID := createTestClubAndUser(t, s)

	if err := s.UnfollowClub(ctx, userID, uuid.New()); !errors.Is(err, store.ErrClubNotFound) {
		t.Errorf("UnfollowClub against unknown club: err = %v, want ErrClubNotFound", err)
	}
}

func TestIntegration_ListClubs_FollowedByMeReflectsOnlyTheGivenViewerAndIsFalseForAnonymous(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	clubID, userID1 := createTestClubAndUser(t, s)
	user2, err := s.UpsertUser(ctx, "google-sub-"+uuid.NewString(), "other-follower@example.com", "OtherFollower", "")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	if err := s.FollowClub(ctx, userID1, clubID); err != nil {
		t.Fatalf("FollowClub: %v", err)
	}

	clubsAsFollower, err := s.ListClubs(ctx, &userID1, nil)
	if err != nil {
		t.Fatalf("ListClubs (as follower): %v", err)
	}
	if !mustFindClub(t, clubsAsFollower, clubID).FollowedByMe {
		t.Error("FollowedByMe = false for the user who followed it, want true")
	}

	clubsAsOther, err := s.ListClubs(ctx, &user2.ID, nil)
	if err != nil {
		t.Fatalf("ListClubs (as other user): %v", err)
	}
	if mustFindClub(t, clubsAsOther, clubID).FollowedByMe {
		t.Error("FollowedByMe = true for a different user, want false")
	}

	clubsAnonymous, err := s.ListClubs(ctx, nil, nil)
	if err != nil {
		t.Fatalf("ListClubs (anonymous): %v", err)
	}
	if mustFindClub(t, clubsAnonymous, clubID).FollowedByMe {
		t.Error("FollowedByMe = true for a nil (anonymous) viewer, want false")
	}
}

func mustFindClub(t *testing.T, clubs []store.ClubFeedItem, id uuid.UUID) store.ClubFeedItem {
	t.Helper()
	for _, c := range clubs {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("club %s not found", id)
	return store.ClubFeedItem{}
}

// TestIntegration_ListRumours_FollowingFiltersToRumoursTouchingAFollowedClub
// proves the Following filter matches a rumour on either side of the deal
// (mirroring ClubID's to/from matching), only for the viewer who actually
// follows the club, and matches nothing for an anonymous (nil) viewer even
// though the query param would otherwise request it.
func TestIntegration_ListRumours_FollowingFiltersToRumoursTouchingAFollowedClub(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	player, err := s.GetOrCreatePlayer(ctx, uniqueName("Following Filter Player"))
	if err != nil {
		t.Fatalf("get or create player: %v", err)
	}
	followedClub, err := s.GetOrCreateClub(ctx, uniqueName("Following Filter Followed Club"))
	if err != nil {
		t.Fatalf("get or create followed club: %v", err)
	}
	otherClub, err := s.GetOrCreateClub(ctx, uniqueName("Following Filter Other Club"))
	if err != nil {
		t.Fatalf("get or create other club: %v", err)
	}
	unrelatedClub, err := s.GetOrCreateClub(ctx, uniqueName("Following Filter Unrelated Club"))
	if err != nil {
		t.Fatalf("get or create unrelated club: %v", err)
	}
	user, err := s.UpsertUser(ctx, "google-sub-"+uuid.NewString(), "following-filter@example.com", "FollowingFilter", "")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := s.FollowClub(ctx, user.ID, followedClub); err != nil {
		t.Fatalf("FollowClub: %v", err)
	}

	// A: -> followedClub (matches on the "to" side).
	rumourA, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: player, ToClubID: followedClub, TransferWindow: uniqueName("window"),
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour A: %v", err)
	}
	// B: followedClub -> otherClub (matches on the "from" side).
	rumourB, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: player, FromClubID: &followedClub, ToClubID: otherClub, TransferWindow: uniqueName("window"),
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour B: %v", err)
	}
	// C: unrelatedClub, no relation to followedClub at all.
	rumourC, _, err := s.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID: player, ToClubID: unrelatedClub, TransferWindow: uniqueName("window"),
		Status: models.StatusRumoured,
	})
	if err != nil {
		t.Fatalf("upsert rumour C: %v", err)
	}

	t.Run("matches rumours touching the followed club on either side, for the following viewer", func(t *testing.T) {
		items, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{Following: true}, &user.ID)
		if err != nil {
			t.Fatalf("list rumours: %v", err)
		}
		if !containsID(items, rumourA.ID) || !containsID(items, rumourB.ID) {
			t.Errorf("expected A and B (both touch the followed club), got %d rumours", len(items))
		}
		if containsID(items, rumourC.ID) {
			t.Error("rumour C doesn't touch the followed club, should not match")
		}
	})

	t.Run("matches nothing for a viewer who doesn't follow the club", func(t *testing.T) {
		otherUser, err := s.UpsertUser(ctx, "google-sub-"+uuid.NewString(), "non-follower@example.com", "NonFollower", "")
		if err != nil {
			t.Fatalf("UpsertUser: %v", err)
		}
		items, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{Following: true}, &otherUser.ID)
		if err != nil {
			t.Fatalf("list rumours: %v", err)
		}
		if containsID(items, rumourA.ID) || containsID(items, rumourB.ID) || containsID(items, rumourC.ID) {
			t.Error("expected no rumours for a viewer following nothing")
		}
	})

	t.Run("matches nothing for an anonymous (nil) viewer", func(t *testing.T) {
		items, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{Following: true}, nil)
		if err != nil {
			t.Fatalf("list rumours: %v", err)
		}
		if containsID(items, rumourA.ID) || containsID(items, rumourB.ID) || containsID(items, rumourC.ID) {
			t.Error("expected no rumours for an anonymous viewer requesting following=true")
		}
	})
}
