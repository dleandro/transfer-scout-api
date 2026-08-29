package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/store"
)

// createTestRumourAndUser inserts the minimal fixtures CreateComment/
// ListComments need to FK against, returning their IDs.
func createTestRumourAndUser(t *testing.T, s *store.Store) (rumourID, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var playerID, clubID uuid.UUID
	if err := s.Pool.QueryRow(ctx, `INSERT INTO players (name) VALUES ($1) RETURNING id`,
		uniqueName("Comment Test Player")).Scan(&playerID); err != nil {
		t.Fatalf("insert test player: %v", err)
	}
	if err := s.Pool.QueryRow(ctx, `INSERT INTO clubs (name) VALUES ($1) RETURNING id`,
		uniqueName("Comment Test Club")).Scan(&clubID); err != nil {
		t.Fatalf("insert test club: %v", err)
	}
	if err := s.Pool.QueryRow(ctx, `
		INSERT INTO rumours (player_id, to_club_id, transfer_window, status)
		VALUES ($1, $2, $3, 'rumoured') RETURNING id`,
		playerID, clubID, uniqueName("comment-test-window")).Scan(&rumourID); err != nil {
		t.Fatalf("insert test rumour: %v", err)
	}

	user, err := s.UpsertUser(ctx, "google-sub-"+uuid.NewString(), "commenter@example.com", "Commenter", "")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	return rumourID, user.ID
}

func TestIntegration_CreateComment_JoinsAuthorAndRejectsUnknownRumour(t *testing.T) {
	s := newTestStore(t)
	rumourID, userID := createTestRumourAndUser(t, s)

	comment, err := s.CreateComment(context.Background(), rumourID, userID, "great signing")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if comment.RumourID != rumourID {
		t.Errorf("RumourID = %v, want %v", comment.RumourID, rumourID)
	}
	if comment.Author.ID != userID || comment.Author.DisplayName != "Commenter" {
		t.Errorf("Author = %+v, want id=%v display_name=Commenter", comment.Author, userID)
	}
	if comment.Body != "great signing" {
		t.Errorf("Body = %q, want %q", comment.Body, "great signing")
	}

	_, err = s.CreateComment(context.Background(), uuid.New(), userID, "orphaned comment")
	if !errors.Is(err, store.ErrRumourNotFound) {
		t.Errorf("CreateComment against a nonexistent rumour: err = %v, want ErrRumourNotFound", err)
	}
}

func TestIntegration_RumourExists(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rumourID, _ := createTestRumourAndUser(t, s)

	exists, err := s.RumourExists(ctx, rumourID)
	if err != nil {
		t.Fatalf("RumourExists (real rumour): %v", err)
	}
	if !exists {
		t.Error("expected true for a rumour that was just inserted")
	}

	exists, err = s.RumourExists(ctx, uuid.New())
	if err != nil {
		t.Fatalf("RumourExists (random id): %v", err)
	}
	if exists {
		t.Error("expected false for a random id that was never inserted")
	}
}

func TestIntegration_ListComments_HasMoreReflectsWhetherAFurtherPageExists(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rumourID, userID := createTestRumourAndUser(t, s)

	for i := 0; i < 3; i++ {
		if _, err := s.CreateComment(ctx, rumourID, userID, uniqueName("comment")); err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
	}

	all, hasMoreAll, err := s.ListComments(ctx, rumourID, 3, 0)
	if err != nil {
		t.Fatalf("ListComments (limit covering every row): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d comments, want 3", len(all))
	}
	if hasMoreAll {
		t.Error("expected has_more=false when limit covers every row")
	}
	if all[0].CreatedAt.After(all[1].CreatedAt) || all[1].CreatedAt.After(all[2].CreatedAt) {
		t.Error("expected comments ordered oldest first")
	}

	partial, hasMorePartial, err := s.ListComments(ctx, rumourID, 2, 0)
	if err != nil {
		t.Fatalf("ListComments (limit one short): %v", err)
	}
	if len(partial) != 2 {
		t.Fatalf("got %d comments, want 2", len(partial))
	}
	if !hasMorePartial {
		t.Error("expected has_more=true when one row remains beyond the requested page")
	}
}
