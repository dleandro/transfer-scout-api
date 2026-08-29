// Package auth handles Google OAuth ID token verification and the
// Transfer-Scout-issued JWT that identifies a user on subsequent
// requests. See Milestone 3.1 (issue #45) for the full design.
package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

const googleIssuer = "https://accounts.google.com"

// GoogleClaims is the subset of a verified Google ID token's claims this
// app uses.
type GoogleClaims struct {
	Sub     string
	Email   string
	Name    string
	Picture string
}

// GoogleVerifier verifies Google-issued OIDC ID tokens against a specific
// OAuth client ID (the audience). Construct once at startup via
// NewGoogleVerifier — it fetches Google's discovery document and caches
// its signing keys — and reuse across requests; it's safe for concurrent
// use. Deliberately not a stateless per-call function: refetching the
// discovery document on every login would be wasteful and slow.
type GoogleVerifier struct {
	verifier *oidc.IDTokenVerifier
}

// NewGoogleVerifier fetches Google's OIDC discovery document. Call once at
// startup; treat a failure here the same as a DB connect failure — fail
// fast, don't start serving.
func NewGoogleVerifier(ctx context.Context, clientID string) (*GoogleVerifier, error) {
	provider, err := oidc.NewProvider(ctx, googleIssuer)
	if err != nil {
		return nil, fmt.Errorf("auth: fetch google oidc discovery document: %w", err)
	}
	return &GoogleVerifier{verifier: provider.Verifier(&oidc.Config{ClientID: clientID})}, nil
}

// Verify checks the token's signature, issuer, audience, and expiry, and
// extracts the claims this app needs.
func (g *GoogleVerifier) Verify(ctx context.Context, rawIDToken string) (*GoogleClaims, error) {
	idToken, err := g.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("auth: verify google id token: %w", err)
	}

	var extra struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := idToken.Claims(&extra); err != nil {
		return nil, fmt.Errorf("auth: parse google id token claims: %w", err)
	}

	return &GoogleClaims{
		Sub:     idToken.Subject,
		Email:   extra.Email,
		Name:    extra.Name,
		Picture: extra.Picture,
	}, nil
}
