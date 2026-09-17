package serve

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var ErrUserNotFound = errors.New("user not found")

func scanManagedUser(row interface{ Scan(...any) error }, passwordHash *string) (User, error) {
	var user User
	args := []any{&user.ID, &user.Username}
	if passwordHash != nil {
		args = append(args, passwordHash)
	}
	args = append(args, &user.Role, &user.CreatedAt, &user.UpdatedAt, &user.DisabledAt)
	if err := row.Scan(args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	return user, nil
}

func (s *AuthStore) userByID(ctx context.Context, userID string, passwordHash *string) (User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return User{}, ErrUserNotFound
	}
	columns := "id, username, role, created_at, updated_at, disabled_at"
	if passwordHash != nil {
		columns = "id, username, password_hash, role, created_at, updated_at, disabled_at"
	}
	return scanManagedUser(s.db.QueryRowContext(ctx, "SELECT "+columns+" FROM users WHERE id = ?", userID), passwordHash)
}

func (s *AuthStore) userByUsername(ctx context.Context, username string, passwordHash *string) (User, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	columns := "id, username, role, created_at, updated_at, disabled_at"
	if passwordHash != nil {
		columns = "id, username, password_hash, role, created_at, updated_at, disabled_at"
	}
	return scanManagedUser(s.db.QueryRowContext(ctx, "SELECT "+columns+" FROM users WHERE username = ?", username), passwordHash)
}

// SetPasswordByUsername is a privileged local-management operation for
// provisioning callers. It keeps the existing user identity and revokes all
// active sessions so a credential rotation takes effect immediately.
func (s *AuthStore) SetPasswordByUsername(ctx context.Context, username, password string) (User, error) {
	user, err := s.userByUsername(ctx, username, nil)
	if err != nil {
		return User{}, err
	}
	return s.setPassword(ctx, user, password)
}

func (s *AuthStore) setPassword(ctx context.Context, user User, password string) (User, error) {
	if user.DisabledAt != nil {
		return User{}, ErrDisabledUser
	}
	hash, err := s.hashPassword(password)
	if err != nil {
		return User{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()
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

// ReconcileAdmin applies declarative username/password state to one stable
// admin identity. If userID is empty, an existing user with desiredUsername is
// adopted, or a new admin is created. Once a userID is persisted by the caller,
// a missing ID is an error rather than permission to adopt a different user.
func (s *AuthStore) ReconcileAdmin(ctx context.Context, userID, desiredUsername, password string) (User, error) {
	desiredUsername, err := normalizeUsername(desiredUsername)
	if err != nil {
		return User{}, err
	}

	var passwordHash string
	var user User
	if strings.TrimSpace(userID) == "" {
		user, err = s.userByUsername(ctx, desiredUsername, &passwordHash)
		if errors.Is(err, ErrUserNotFound) {
			return s.CreateAdmin(ctx, desiredUsername, password)
		}
	} else {
		user, err = s.userByID(ctx, userID, &passwordHash)
	}
	if err != nil {
		return User{}, err
	}
	if user.DisabledAt != nil {
		return User{}, ErrDisabledUser
	}
	if user.Role != adminRole {
		return User{}, errors.New("declarative admin identity is not an admin")
	}

	matches, err := s.verifyPassword(passwordHash, password)
	if err != nil {
		return User{}, err
	}
	var newHash string
	if !matches {
		newHash, err = s.hashPassword(password)
		if err != nil {
			return User{}, err
		}
	}
	if matches && user.Username == desiredUsername {
		return user, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := s.now()
	if user.Username != desiredUsername {
		result, err := tx.ExecContext(ctx, `UPDATE users SET username = ?, updated_at = ? WHERE id = ?`, desiredUsername, now, user.ID)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return User{}, ErrDuplicateUsername
			}
			return User{}, err
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			if err != nil {
				return User{}, err
			}
			return User{}, ErrUserNotFound
		}
		user.Username = desiredUsername
	}
	if !matches {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, newHash, now, user.ID); err != nil {
			return User{}, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE sessions
SET revoked_at = ?
WHERE user_id = ? AND revoked_at IS NULL`, now, user.ID); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	user.UpdatedAt = now
	return user, nil
}
