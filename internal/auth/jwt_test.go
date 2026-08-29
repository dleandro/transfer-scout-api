package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIssueAndParseToken_RoundTrip(t *testing.T) {
	userID := uuid.New()
	token, err := IssueToken(userID, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	got, err := ParseToken(token, "test-secret")
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if got != userID {
		t.Errorf("got %v, want %v", got, userID)
	}
}

func TestParseToken_WrongSecretRejected(t *testing.T) {
	token, err := IssueToken(uuid.New(), "correct-secret", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	if _, err := ParseToken(token, "wrong-secret"); err == nil {
		t.Fatal("expected an error for a token signed with a different secret, got none")
	}
}

func TestParseToken_ExpiredRejected(t *testing.T) {
	token, err := IssueToken(uuid.New(), "test-secret", -time.Hour) // already expired
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	if _, err := ParseToken(token, "test-secret"); err == nil {
		t.Fatal("expected an error for an expired token, got none")
	}
}

func TestParseToken_MalformedRejected(t *testing.T) {
	if _, err := ParseToken("not-a-jwt", "test-secret"); err == nil {
		t.Fatal("expected an error for a malformed token, got none")
	}
}
