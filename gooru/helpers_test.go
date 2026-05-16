package gooru_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
	"gooru.local/types"
)

func setupTestDB(t *testing.T) string {
	t.Helper()
	return setupTestDBWithStrategy(t, types.StrategyPartial)
}

func setupTestDBWithStrategy(t *testing.T, strategy types.HashingStrategy) string {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	require.NoError(t, gooru.Init(dbPath, strategy, false))
	return dbPath
}

func createTestFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
	return filePath
}

func overwriteFilePreservingMetadata(t *testing.T, path string, content string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	mtime := time.Unix(info.ModTime().Unix(), 0)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	require.NoError(t, os.Chtimes(path, mtime, mtime))
}
