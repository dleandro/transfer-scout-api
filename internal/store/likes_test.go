package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/store"
)

func TestIntegration_LikeRumour_IsIdempotentAndRestoresAfterUnlike(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rumourID, userID := createTestRumourAndUser(t, s)

	if err := s.LikeRumour(ctx, rumourID, userID); err != nil {
		t.Fatalf("LikeRumour: %v", err)
	}
	// Re-liking an already-liked rumour is a no-op, not an error.
	if err := s.LikeRumour(ctx, rumourID, userID); err != nil {
		t.Fatalf("LikeRumour (re-like): %v", err)
	}

	item, _, err := s.GetRumourByID(ctx, rumourID, &userID)
	if err != nil {
		t.Fatalf("GetRumourByID: %v", err)
	}
	if item.LikeCount != 1 {
		t.Errorf("LikeCount = %d, want 1 (re-liking should not double-count)", item.LikeCount)
	}
	if !item.LikedByMe {
		t.Error("LikedByMe = false, want true after LikeRumour")
	}

	if err := s.UnlikeRumour(ctx, rumourID, userID); err != nil {
		t.Fatalf("UnlikeRumour: %v", err)
	}
	item, _, err = s.GetRumourByID(ctx, rumourID, &userID)
	if err != nil {
		t.Fatalf("GetRumourByID after unlike: %v", err)
	}
	if item.LikeCount != 0 {
		t.Errorf("LikeCount = %d, want 0 after unlike", item.LikeCount)
	}
	if item.LikedByMe {
		t.Error("LikedByMe = true, want false after UnlikeRumour")
	}

	// Re-liking after an unlike restores the same row rather than erroring
	// on the (rumour_id, user_id) unique constraint.
	if err := s.LikeRumour(ctx, rumourID, userID); err != nil {
		t.Fatalf("LikeRumour (after unlike): %v", err)
	}
	item, _, err = s.GetRumourByID(ctx, rumourID, &userID)
	if err != nil {
		t.Fatalf("GetRumourByID after re-like: %v", err)
	}
	if item.LikeCount != 1 || !item.LikedByMe {
		t.Errorf("after re-like: LikeCount=%d LikedByMe=%v, want 1/true", item.LikeCount, item.LikedByMe)
	}
}

func TestIntegration_UnlikeRumour_NeverLikedIsANoOp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rumourID, userID := createTestRumourAndUser(t, s)

	if err := s.UnlikeRumour(ctx, rumourID, userID); err != nil {
		t.Fatalf("UnlikeRumour (never liked): %v", err)
	}
}

func TestIntegration_LikeRumour_UnknownRumourReturnsErrRumourNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, userID := createTestRumourAndUser(t, s)

	if err := s.LikeRumour(ctx, uuid.New(), userID); !errors.Is(err, store.ErrRumourNotFound) {
		t.Errorf("LikeRumour against unknown rumour: err = %v, want ErrRumourNotFound", err)
	}
}

func TestIntegration_UnlikeRumour_UnknownRumourReturnsErrRumourNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, userID := createTestRumourAndUser(t, s)

	if err := s.UnlikeRumour(ctx, uuid.New(), userID); !errors.Is(err, store.ErrRumourNotFound) {
		t.Errorf("UnlikeRumour against unknown rumour: err = %v, want ErrRumourNotFound", err)
	}
}

func TestIntegration_LikeCount_CountsDistinctActiveLikersOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rumourID, userID1 := createTestRumourAndUser(t, s)
	user2, err := s.UpsertUser(ctx, "google-sub-"+uuid.NewString(), "second@example.com", "Second", "")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	if err := s.LikeRumour(ctx, rumourID, userID1); err != nil {
		t.Fatalf("LikeRumour user1: %v", err)
	}
	if err := s.LikeRumour(ctx, rumourID, user2.ID); err != nil {
		t.Fatalf("LikeRumour user2: %v", err)
	}

	items, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{}, nil)
	if err != nil {
		t.Fatalf("ListRumours: %v", err)
	}
	var found *store.RumourFeedItem
	for i := range items {
		if items[i].ID == rumourID {
			found = &items[i]
		}
	}
	if found == nil {
		t.Fatalf("rumour %s not found in ListRumours results", rumourID)
	}
	if found.LikeCount != 2 {
		t.Errorf("LikeCount = %d, want 2", found.LikeCount)
	}
	// Anonymous viewer (nil viewerID) never sees liked_by_me=true.
	if found.LikedByMe {
		t.Error("LikedByMe = true for a nil (anonymous) viewer, want false")
	}
}

func TestIntegration_ListRumours_LikedByMeReflectsOnlyTheGivenViewer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rumourID, userID1 := createTestRumourAndUser(t, s)
	user2, err := s.UpsertUser(ctx, "google-sub-"+uuid.NewString(), "third@example.com", "Third", "")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	if err := s.LikeRumour(ctx, rumourID, userID1); err != nil {
		t.Fatalf("LikeRumour: %v", err)
	}

	itemsAsLiker, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{}, &userID1)
	if err != nil {
		t.Fatalf("ListRumours (as liker): %v", err)
	}
	if !mustFind(t, itemsAsLiker, rumourID).LikedByMe {
		t.Error("LikedByMe = false for the user who liked it, want true")
	}

	itemsAsOther, _, err := s.ListRumours(ctx, 100, 0, store.RumourFilter{}, &user2.ID)
	if err != nil {
		t.Fatalf("ListRumours (as other user): %v", err)
	}
	if mustFind(t, itemsAsOther, rumourID).LikedByMe {
		t.Error("LikedByMe = true for a different user, want false")
	}
}

func mustFind(t *testing.T, items []store.RumourFeedItem, id uuid.UUID) store.RumourFeedItem {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("rumour %s not found", id)
	return store.RumourFeedItem{}
}
