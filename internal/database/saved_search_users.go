package database

import (
	"database/sql"
	"strings"
)

// GetUserIDByUsername resolves a DB-backed user by its human-facing username.
func (s *Store) GetUserIDByUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", sql.ErrNoRows
	}
	var id string
	err := s.QueryRow("SELECT id FROM users WHERE username = ? COLLATE NOCASE", username).Scan(&id)
	return id, err
}
