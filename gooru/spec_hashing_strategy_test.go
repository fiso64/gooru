package gooru_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
	"gooru.local/types"
)

func TestSpecHashingStrategy_IsStoredAndUsedByDatabase(t *testing.T) {
	t.Parallel()

	for _, strategy := range []types.HashingStrategy{types.StrategyFull, types.StrategyPartial} {
		strategy := strategy
		t.Run(string(strategy), func(t *testing.T) {
			t.Parallel()
			dbPath := setupTestDBWithStrategy(t, strategy)
			client, err := gooru.New(dbPath, false)
			require.NoError(t, err)
			t.Cleanup(func() { client.Close() })

			dir := t.TempDir()
			file := createTestFile(t, dir, "file.txt", "content")
			_, err = client.TagFiles([]string{file}, []string{"tracked"}, nil, false)
			require.NoError(t, err)

			tags, status, err := client.GetTagsForFile(file, false)
			require.NoError(t, err)
			assert.Equal(t, types.StatusOK, status)
			assert.ElementsMatch(t, []string{"tracked"}, tags)
		})
	}
}
