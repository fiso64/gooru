package serve

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ChangePasswordAndRevokeOtherSessions updates the user's password and revokes
// every other active session in the same transaction. The session performing
// the change is preserved so a successful password change does not force the
// current browser to log in again.
func (s *AuthStore) ChangePasswordAndRevokeOtherSessions(ctx context.Context, userID, currentSessionID, currentPassword, newPassword string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var passwordHash string
	var disabledAt *time.Time
	err = tx.QueryRowContext(ctx, `SELECT password_hash, disabled_at FROM users WHERE id = ?`, userID).Scan(&passwordHash, &disabledAt)
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
	now := s.now()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, newHash, now, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE sessions
SET revoked_at = ?
WHERE user_id = ? AND id <> ? AND revoked_at IS NULL`, now, userID, currentSessionID); err != nil {
		return err
	}
	return tx.Commit()
}
