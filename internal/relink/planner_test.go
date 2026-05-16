package relink_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/internal/relink"
	"gooru.local/types"
)

func TestPlanTreatsDeletedDuplicateAsDeleteWhenTrackedDuplicateRemains(t *testing.T) {
	result := relink.Plan(relink.PlanInput{
		DBLocations: map[string]types.LocationInfo{
			"/old.txt":     loc("/old.txt", "hash", "oldtag"),
			"/tracked.txt": loc("/tracked.txt", "hash", "oldtag"),
		},
		FSLocations: map[string]types.LocationInfo{
			"/tracked.txt": loc("/tracked.txt", "hash", ""),
		},
		KnownPathsByHash: map[string][]string{"hash": {"/old.txt", "/tracked.txt"}},
	})

	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedAdds)
	require.Len(t, result.ProposedDeletes, 1)
	assert.Equal(t, "/old.txt", result.ProposedDeletes[0].Path)
	assert.ElementsMatch(t, []string{"oldtag"}, result.ProposedDeletes[0].Tags)
}

func TestPlanMovesMissingPathToUntrackedSameHashInScope(t *testing.T) {
	result := relink.Plan(relink.PlanInput{
		DBLocations: map[string]types.LocationInfo{
			"/old.txt": loc("/old.txt", "hash", ""),
		},
		FSLocations: map[string]types.LocationInfo{
			"/new.txt": loc("/new.txt", "hash", ""),
		},
		KnownPathsByHash: map[string][]string{"hash": {"/old.txt"}},
	})

	require.Len(t, result.ProposedMoves, 1)
	assert.Equal(t, "/old.txt", result.ProposedMoves[0].OldPath)
	assert.Equal(t, "/new.txt", result.ProposedMoves[0].NewLocation.Path)
	assert.Empty(t, result.ProposedAdds)
	assert.Empty(t, result.ProposedDeletes)
}

func TestPlanMovesKnownContentFromMissingPathOutsideScope(t *testing.T) {
	result := relink.Plan(relink.PlanInput{
		DBLocations: map[string]types.LocationInfo{},
		FSLocations: map[string]types.LocationInfo{
			"/new.txt": loc("/new.txt", "hash", ""),
		},
		KnownPathsByHash: map[string][]string{"hash": {"/old-outside.txt"}},
		MissingKnownPath: map[string]bool{"/old-outside.txt": true},
	})

	require.Len(t, result.ProposedMoves, 1)
	assert.Equal(t, "/old-outside.txt", result.ProposedMoves[0].OldPath)
	assert.Equal(t, "/new.txt", result.ProposedMoves[0].NewLocation.Path)
	assert.Empty(t, result.ProposedAdds)
	assert.Empty(t, result.ProposedDeletes)
}

func TestPlanAddsKnownContentWhenOldPathStillExists(t *testing.T) {
	result := relink.Plan(relink.PlanInput{
		DBLocations: map[string]types.LocationInfo{},
		FSLocations: map[string]types.LocationInfo{
			"/duplicate.txt": loc("/duplicate.txt", "hash", ""),
		},
		KnownPathsByHash: map[string][]string{"hash": {"/existing-outside.txt"}},
		MissingKnownPath: map[string]bool{"/existing-outside.txt": false},
	})

	assert.Empty(t, result.ProposedMoves)
	require.Len(t, result.ProposedAdds, 1)
	assert.Equal(t, "/duplicate.txt", result.ProposedAdds[0].Path)
	assert.Empty(t, result.ProposedDeletes)
}

func TestPlanIgnoresUnknownSameSizeContent(t *testing.T) {
	result := relink.Plan(relink.PlanInput{
		DBLocations: map[string]types.LocationInfo{},
		FSLocations: map[string]types.LocationInfo{
			"/unknown.txt": loc("/unknown.txt", "unknown", ""),
		},
		KnownPathsByHash: map[string][]string{},
	})

	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedAdds)
	assert.Empty(t, result.ProposedDeletes)
}

func TestPlanAddsEveryUnhandledDuplicateLocation(t *testing.T) {
	result := relink.Plan(relink.PlanInput{
		DBLocations: map[string]types.LocationInfo{
			"/tracked.txt": loc("/tracked.txt", "hash", ""),
		},
		FSLocations: map[string]types.LocationInfo{
			"/tracked.txt": loc("/tracked.txt", "hash", ""),
			"/a.txt":       loc("/a.txt", "hash", ""),
			"/b.txt":       loc("/b.txt", "hash", ""),
		},
		KnownPathsByHash: map[string][]string{"hash": {"/tracked.txt"}},
	})

	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedDeletes)
	require.Len(t, result.ProposedAdds, 2)
	assert.ElementsMatch(t, []string{"/a.txt", "/b.txt"}, []string{result.ProposedAdds[0].Path, result.ProposedAdds[1].Path})
}

func TestPlanKeepsModifiedKnownContentAsDeleteAndAddAtSamePath(t *testing.T) {
	result := relink.Plan(relink.PlanInput{
		DBLocations: map[string]types.LocationInfo{
			"/changed.txt": loc("/changed.txt", "old-hash", "oldtag"),
			"/known.txt":   loc("/known.txt", "new-hash", "newtag"),
		},
		FSLocations: map[string]types.LocationInfo{
			"/changed.txt": loc("/changed.txt", "new-hash", ""),
			"/known.txt":   loc("/known.txt", "new-hash", ""),
		},
		KnownPathsByHash: map[string][]string{"new-hash": {"/known.txt"}},
	})

	assert.Empty(t, result.ProposedMoves)
	require.Len(t, result.ProposedDeletes, 1)
	assert.Equal(t, "/changed.txt", result.ProposedDeletes[0].Path)
	require.Len(t, result.ProposedAdds, 1)
	assert.Equal(t, "/changed.txt", result.ProposedAdds[0].Path)
}

func loc(path, hash, tags string) types.LocationInfo {
	return types.LocationInfo{Path: path, Hash: hash, Size: 1, ModTime: 1, Extension: ".txt", TagsCache: tags}
}
