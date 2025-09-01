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
	configDir := filepath.Join(home, ".config", "gooru")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(configDir, "gooru.db"), nil
}

// GetRunDirPath returns the path to the runtime directory.
// It creates the directory ~/.config/gooru/run if it doesn't exist.
func GetRunDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	runDir := filepath.Join(home, ".config", "gooru", "run")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return "", err
	}
	return runDir, nil
}