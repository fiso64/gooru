package serve

import "crypto/subtle"

const csrfTokenDomain = "gooru-csrf-v1\x00"

// StableCSRF returns a deterministic CSRF token derived from the high-entropy
// session token. Exposing this one-way derivative to frontend code does not
// expose the HttpOnly session credential, and every tab sharing the session
// receives the same CSRF token instead of invalidating one another.
func (s *AuthStore) StableCSRF(auth AuthSession) (string, error) {
	if auth.Session.ID == "" || auth.User.ID == "" || auth.Token == "" {
		return "", ErrSessionNotFound
	}
	return stableCSRFToken(auth.Token), nil
}

// VerifyCSRFToken accepts the stable per-session token and the legacy random
// token still represented by csrf_token_hash. Keeping the legacy path during
// the transition means already-open tabs remain usable after an upgrade while
// new login and /auth/me responses converge on the stable token.
func (s *AuthStore) VerifyCSRFToken(auth AuthSession, token string) bool {
	if s.VerifyCSRF(auth, token) {
		return true
	}
	if auth.Token == "" || token == "" {
		return false
	}
	stable := stableCSRFToken(auth.Token)
	return len(stable) == len(token) && subtle.ConstantTimeCompare([]byte(stable), []byte(token)) == 1
}

func stableCSRFToken(sessionToken string) string {
	return hashToken(csrfTokenDomain + sessionToken)
}
