package config

import (
	"gooru.local/internal/config"
)

// GetDBPath returns the path to the SQLite database file.
func GetDBPath() (string, error) {
	return config.GetDBPath()
}

// GetRunDirPath returns the path to the runtime directory.
func GetRunDirPath() (string, error) {
	return config.GetRunDirPath()
}