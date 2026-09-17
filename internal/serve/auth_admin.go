package serve

import (
	"context"
	"database/sql"
	"errors"
)

var ErrUserNotFound = errors.New("user not found")

// SetPasswordByUsername is a privileged local-management operation for
// declarative/provisioning callers. It keeps the existing user identity,
// replaces the password hash exactly once when invoked, and revokes all active
// sessions so a credential rotation takes effect immediately.
func (s *AuthStore) SetPasswordByUsername(ctx context.Context, username, password string) (User, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var user User
	err = tx.QueryRowContext(ctx, `
SELECT id, username, role, created_at, updated_at, disabled_at
FROM users
WHERE username = ?`, username).Scan(
		&user.ID, &user.Username, &user.Role, &user.CreatedAt, &user.UpdatedAt, &user.DisabledAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, err
	}
	if user.DisabledAt != nil {
		return User{}, ErrDisabledUser
	}

	hash, err := s.hashPassword(password)
	if err != nil {
		return User{}, err
	}
	now := s.now()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, now, user.ID); err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE sessions
SET revoked_at = ?
WHERE user_id = ? AND revoked_at IS NULL`, now, user.ID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	user.UpdatedAt = now
	return user, nil
}
