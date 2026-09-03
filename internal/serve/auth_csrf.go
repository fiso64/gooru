package serve

import (
	"context"
	"crypto/subtle"
	"errors"
)

const csrfTokenDomain = "gooru-csrf-v1\x00"

// StableCSRF returns a deterministic CSRF token derived from the high-entropy
// session token. Exposing this one-way derivative to frontend code does not
// expose the HttpOnly session credential, and every tab sharing the session
// receives the same CSRF token instead of invalidating one another.
//
// Existing sessions may still have a randomly generated CSRF hash. The first
// stable-token request migrates that hash in place so deployments do not need
// a schema migration or forced logout.
func (s *AuthStore) StableCSRF(ctx context.Context, auth AuthSession) (string, error) {
	if auth.Session.ID == "" || auth.User.ID == "" || auth.Token == "" {
		return "", ErrSessionNotFound
	}
	token := hashToken(csrfTokenDomain + auth.Token)
	desiredHash := hashToken(token)
	if len(auth.Session.CSRFHash) == len(desiredHash) && subtle.ConstantTimeCompare([]byte(auth.Session.CSRFHash), []byte(desiredHash)) == 1 {
		return token, nil
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE sessions
SET csrf_token_hash = ?
WHERE id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?`, desiredHash, auth.Session.ID, auth.User.ID, s.now())
	if err != nil {
		return "", err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if updated != 1 {
		return "", errors.New("session became unavailable while issuing CSRF token")
	}
	return token, nil
}
