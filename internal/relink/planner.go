package relink

import (
	"sort"
	"strings"

	"gooru.local/types"
)

// PlanInput is the database/filesystem snapshot used to produce a relink dry run.
type PlanInput struct {
	DBLocations      map[string]types.LocationInfo
	FSLocations      map[string]types.LocationInfo
	KnownPathsByHash map[string][]string
	MissingKnownPath map[string]bool
	FilesScanned     int
}

// Plan classifies a filesystem scan against known database state without reading
// the filesystem or database itself.
func Plan(input PlanInput) types.RelinkResult {
	result := types.RelinkResult{}
	result.Stats.FilesScanned = input.FilesScanned

	fsByHash := locationsByHash(input.FSLocations)
	handledFSPaths := make(map[string]bool)
	usedOldMovePaths := make(map[string]bool)

	for _, dbPath := range sortedLocationKeys(input.DBLocations) {
		dbInfo := input.DBLocations[dbPath]
		fsInfo, pathExistsOnFS := input.FSLocations[dbPath]

		if pathExistsOnFS {
			if fsInfo.Hash == dbInfo.Hash {
				handledFSPaths[dbPath] = true
			} else {
				result.ProposedDeletes = append(result.ProposedDeletes, fileInfo(dbPath, dbInfo))
			}
			continue
		}

		newLocation, ok := firstMoveTarget(fsByHash[dbInfo.Hash], input.DBLocations, handledFSPaths)
		if ok {
			result.ProposedMoves = append(result.ProposedMoves, types.MoveInfo{
				OldPath:     dbPath,
				NewLocation: newLocation,
			})
			handledFSPaths[newLocation.Path] = true
			usedOldMovePaths[dbPath] = true
			continue
		}

		result.ProposedDeletes = append(result.ProposedDeletes, fileInfo(dbPath, dbInfo))
	}

	for _, fsPath := range sortedLocationKeys(input.FSLocations) {
		if handledFSPaths[fsPath] {
			continue
		}

		fsInfo := input.FSLocations[fsPath]
		oldPaths, contentWasKnown := input.KnownPathsByHash[fsInfo.Hash]
		if !contentWasKnown {
			continue
		}

		oldPath, isMove := firstMissingOldPath(oldPaths, input.DBLocations, input.MissingKnownPath, usedOldMovePaths)
		if isMove {
			result.ProposedMoves = append(result.ProposedMoves, types.MoveInfo{
				OldPath:     oldPath,
				NewLocation: fsInfo,
			})
			usedOldMovePaths[oldPath] = true
			continue
		}

		result.ProposedAdds = append(result.ProposedAdds, fsInfo)
	}

	return result
}

func locationsByHash(locations map[string]types.LocationInfo) map[string][]types.LocationInfo {
	byHash := make(map[string][]types.LocationInfo)
	for _, info := range locations {
		byHash[info.Hash] = append(byHash[info.Hash], info)
	}
	for hash := range byHash {
		sort.Slice(byHash[hash], func(i, j int) bool {
			return byHash[hash][i].Path < byHash[hash][j].Path
		})
	}
	return byHash
}

func firstMoveTarget(candidates []types.LocationInfo, dbLocations map[string]types.LocationInfo, handledFSPaths map[string]bool) (types.LocationInfo, bool) {
	for _, candidate := range candidates {
		if handledFSPaths[candidate.Path] {
			continue
		}
		if dbInfo, tracked := dbLocations[candidate.Path]; tracked && dbInfo.Hash == candidate.Hash {
			continue
		}
		return candidate, true
	}
	return types.LocationInfo{}, false
}

func firstMissingOldPath(oldPaths []string, dbLocations map[string]types.LocationInfo, missingKnownPath map[string]bool, usedOldMovePaths map[string]bool) (string, bool) {
	sorted := append([]string(nil), oldPaths...)
	sort.Strings(sorted)

	for _, oldPath := range sorted {
		if usedOldMovePaths[oldPath] {
			continue
		}
		if _, inScope := dbLocations[oldPath]; inScope {
			continue
		}
		if missingKnownPath[oldPath] {
			return oldPath, true
		}
	}
	return "", false
}

func sortedLocationKeys(locations map[string]types.LocationInfo) []string {
	paths := make([]string, 0, len(locations))
	for path := range locations {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func fileInfo(path string, loc types.LocationInfo) types.FileInfo {
	info := types.FileInfo{Path: path, Hash: loc.Hash, Size: loc.Size, ModTime: loc.ModTime}
	if loc.TagsCache != "" {
		info.Tags = strings.Split(loc.TagsCache, " ")
	}
	return info
}
