package gooru_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
	"gooru.local/types"
)

// setupTestDB is a test helper that creates a temporary, initialized database for a test.
// It returns the path to the database file.
// The testing framework's `t.TempDir()` ensures the parent directory is cleaned up automatically.
func setupTestDB(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	err := gooru.Init(dbPath, types.StrategyPartial, false)
	require.NoError(t, err, "Failed to initialize test database")
	return dbPath
}

// createTestFile is a helper to create a file with specific content in a directory.
func createTestFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to create test file")
	return filePath
}