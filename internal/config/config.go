package config

import (
	"os"
	"path/filepath"
)

// GetDBPath returns the path to the SQLite database file.
// It creates the directory ~/.config/gooru if it doesn't exist.
func GetDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dbDir := filepath.Join(home, ".config", "gooru")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dbDir, "gooru.db"), nil
}