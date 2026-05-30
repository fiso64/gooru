package serve

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const adminRole = "admin"

var (
	ErrDuplicateUsername  = errors.New("username already exists")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrDisabledUser       = errors.New("user is disabled")
	ErrSessionNotFound    = errors.New("session not found")
)

type AuthStore struct {
	db              *sql.DB
	sessionTTL      time.Duration
	lastSeenTimeout time.Duration
	now             func() time.Time
}

type User struct {
	ID         string     `json:"id"`
	Username   string     `json:"username"`
	Role       string     `json:"role"`
	CreatedAt  time.Time  `json:"-"`
	UpdatedAt  time.Time  `json:"-"`
	DisabledAt *time.Time `json:"-"`
}

type Session struct {
	ID         string
	UserID     string
	TokenHash  string
	CSRFHash   string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	RevokedAt  *time.Time
}

type AuthSession struct {
	User      User
	Session   Session
	Token     string
	CSRFToken string
}

func NewAuthStore(db *sql.DB, sessionTTL time.Duration) *AuthStore {
	return &AuthStore{
		db:              db,
		sessionTTL:      sessionTTL,
		lastSeenTimeout: 5 * time.Minute,
		now:             func() time.Time { return time.Now().UTC() },
	}
}

func (s *AuthStore) CreateAdmin(ctx context.Context, username, password string) (User, error) {
	return s.createUser(ctx, username, password, adminRole)
}

func (s *AuthStore) createUser(ctx context.Context, username, password, role string) (User, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if role == "" {
		role = adminRole
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	now := s.now()
	id, err := newPublicID("usr")
	if err != nil {
		return User{}, err
	}
	user := User{
		ID:        id,
		Username:  username,
		Role:      role,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO users (id, username, password_hash, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`, user.ID, user.Username, hash, user.Role, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return User{}, ErrDuplicateUsername
		}
		return User{}, err
	}
	return user, nil
}

func (s *AuthStore) Login(ctx context.Context, username, password string) (AuthSession, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return AuthSession{}, ErrInvalidCredentials
	}
	var user User
	var passwordHash string
	err = s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, role, created_at, updated_at, disabled_at
FROM users
WHERE username = ?`, username).Scan(&user.ID, &user.Username, &passwordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt, &user.DisabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthSession{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthSession{}, err
	}
	if user.DisabledAt != nil {
		return AuthSession{}, ErrDisabledUser
	}
	ok, err := VerifyPassword(passwordHash, password)
	if err != nil || !ok {
		return AuthSession{}, ErrInvalidCredentials
	}
	return s.createSession(ctx, user)
}

func (s *AuthStore) LookupSession(ctx context.Context, token string) (AuthSession, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AuthSession{}, ErrSessionNotFound
	}
	tokenHash := hashToken(token)
	var auth AuthSession
	err := s.db.QueryRowContext(ctx, `
SELECT s.id, s.user_id, s.token_hash, s.csrf_token_hash, s.created_at, s.expires_at, s.last_seen_at, s.revoked_at,
       u.id, u.username, u.role, u.created_at, u.updated_at, u.disabled_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = ?`, tokenHash).Scan(
		&auth.Session.ID, &auth.Session.UserID, &auth.Session.TokenHash, &auth.Session.CSRFHash,
		&auth.Session.CreatedAt, &auth.Session.ExpiresAt, &auth.Session.LastSeenAt, &auth.Session.RevokedAt,
		&auth.User.ID, &auth.User.Username, &auth.User.Role, &auth.User.CreatedAt, &auth.User.UpdatedAt, &auth.User.DisabledAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthSession{}, ErrSessionNotFound
	}
	if err != nil {
		return AuthSession{}, err
	}
	now := s.now()
	if auth.User.DisabledAt != nil || auth.Session.RevokedAt != nil || !auth.Session.ExpiresAt.After(now) {
		return AuthSession{}, ErrSessionNotFound
	}
	if now.Sub(auth.Session.LastSeenAt) >= s.lastSeenTimeout {
		if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE id = ?`, now, auth.Session.ID); err == nil {
			auth.Session.LastSeenAt = now
		}
	}
	auth.Token = token
	return auth, nil
}

func (s *AuthStore) RevokeSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, s.now(), sessionID)
	return err
}

func (s *AuthStore) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	var passwordHash string
	var disabledAt *time.Time
	err := s.db.QueryRowContext(ctx, `SELECT password_hash, disabled_at FROM users WHERE id = ?`, userID).Scan(&passwordHash, &disabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidCredentials
	}
	if err != nil {
		return err
	}
	if disabledAt != nil {
		return ErrDisabledUser
	}
	ok, err := VerifyPassword(passwordHash, currentPassword)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, newHash, s.now(), userID)
	return err
}

func (s *AuthStore) RotateCSRF(ctx context.Context, sessionID string) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE sessions SET csrf_token_hash = ? WHERE id = ? AND revoked_at IS NULL`, hashToken(token), sessionID)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *AuthStore) CleanupExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ? OR revoked_at IS NOT NULL`, s.now())
	return err
}

func (s *AuthStore) VerifyCSRF(auth AuthSession, token string) bool {
	return subtleTokenCompare(auth.Session.CSRFHash, token)
}

func (s *AuthStore) createSession(ctx context.Context, user User) (AuthSession, error) {
	token, err := randomToken(32)
	if err != nil {
		return AuthSession{}, err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return AuthSession{}, err
	}
	now := s.now()
	id, err := newPublicID("ses")
	if err != nil {
		return AuthSession{}, err
	}
	session := Session{
		ID:         id,
		UserID:     user.ID,
		TokenHash:  hashToken(token),
		CSRFHash:   hashToken(csrfToken),
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.sessionTTL),
		LastSeenAt: now,
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, token_hash, csrf_token_hash, created_at, expires_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, session.ID, session.UserID, session.TokenHash, session.CSRFHash, session.CreatedAt, session.ExpiresAt, session.LastSeenAt)
	if err != nil {
		return AuthSession{}, err
	}
	return AuthSession{User: user, Session: session, Token: token, CSRFToken: csrfToken}, nil
}

func normalizeUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}
	if len(username) > 64 {
		return "", errors.New("username must be 64 characters or fewer")
	}
	for _, r := range username {
		if !(r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return "", errors.New("username may contain only letters, numbers, dots, dashes, and underscores")
		}
	}
	return username, nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newPublicID(prefix string) (string, error) {
	token, err := randomToken(18)
	if err != nil {
		return "", err
	}
	return prefix + "_" + token, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func subtleTokenCompare(expectedHash, token string) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	got := hashToken(token)
	return len(expectedHash) == len(got) && subtle.ConstantTimeCompare([]byte(expectedHash), []byte(got)) == 1
}
