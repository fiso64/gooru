package gooru_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
	"gooru.local/types"
)

func TestClient_TagFilesModifiedSharedContentPreservesOrphanedTags(t *testing.T) {
	t.Parallel()

	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	tempDir := t.TempDir()
	file1 := createTestFile(t, tempDir, "file1.txt", "shared original content")
	file2 := createTestFile(t, tempDir, "file2.txt", "shared original content")
	_, err = client.TagFiles([]string{file1, file2}, []string{"initial_tag"}, nil, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(file1, []byte("modified content one"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("modified content two"), 0644))

	result, err := client.TagFiles([]string{file1, file2}, []string{"new_tag"}, nil, false)
	require.NoError(t, err)
	require.Len(t, result.Notifications, 2)

	byPath := make(map[string]types.Notification, len(result.Notifications))
	for _, notification := range result.Notifications {
		byPath[notification.OriginalPath] = notification
	}
	for _, path := range []string{file1, file2} {
		notification, ok := byPath[path]
		require.True(t, ok, "missing notification for %s", path)
		assert.Equal(t, types.NotificationKindModified, notification.Kind)
		assert.Equal(t, []string{"initial_tag"}, notification.OrphanedTags)
	}
}
