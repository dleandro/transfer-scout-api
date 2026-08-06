package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// errNoUserInContext should be unreachable in practice: KeyByUserID is
// only ever installed after RequireAuth in the middleware chain, which
// always populates the context (or stops the chain with a 401) before
// calling next. Kept as an explicit, sensible error rather than an
// unrelated stdlib sentinel, since httprate's default error handler
// writes this message straight into the HTTP response body.
var errNoUserInContext = errors.New("auth: no authenticated user in request context")

type contextKey int

const userIDContextKey contextKey = iota

// RequireAuth parses a Bearer token from the Authorization header,
// rejecting the request with 401 (plain-text, matching this repo's
// existing error convention) if it's missing, malformed, or invalid. On
// success, the authenticated user's ID is placed in the request context,
// readable via UserIDFromContext.
func RequireAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				http.Error(w, "missing or malformed authorization header", http.StatusUnauthorized)
				return
			}

			userID, err := ParseToken(token, secret)
			if err != nil {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext returns the authenticated user's ID, as placed by
// RequireAuth.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDContextKey).(uuid.UUID)
	return userID, ok
}

// KeyByUserID is an httprate.KeyFunc that rate-limits by the authenticated
// user's ID rather than IP — must run after RequireAuth in the middleware
// chain, since it reads the context value RequireAuth sets.
func KeyByUserID(r *http.Request) (string, error) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		return "", errNoUserInContext
	}
	return userID.String(), nil
}
