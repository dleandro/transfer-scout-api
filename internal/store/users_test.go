package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestIntegration_UpsertUser_InsertsThenUpdates proves UpsertUser creates
// a row on first sign-in and refreshes the profile fields (not the
// identity) on a repeat call with the same google_sub.
func TestIntegration_UpsertUser_InsertsThenUpdates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	googleSub := "google-sub-" + uuid.NewString()

	created, err := s.UpsertUser(ctx, googleSub, "first@example.com", "First Name", "https://example.com/first.png")
	if err != nil {
		t.Fatalf("UpsertUser (insert): %v", err)
	}
	if created.Email != "first@example.com" || created.DisplayName != "First Name" {
		t.Fatalf("created = %+v, want first@example.com / First Name", created)
	}

	updated, err := s.UpsertUser(ctx, googleSub, "second@example.com", "Second Name", "https://example.com/second.png")
	if err != nil {
		t.Fatalf("UpsertUser (update): %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("update produced a different user ID (%v) than the insert (%v) — should be the same row, keyed on google_sub", updated.ID, created.ID)
	}
	if updated.Email != "second@example.com" || updated.DisplayName != "Second Name" {
		t.Errorf("updated = %+v, want second@example.com / Second Name", updated)
	}
}
