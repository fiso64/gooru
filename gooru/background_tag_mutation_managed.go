package gooru

import (
	"fmt"
	"path/filepath"

	"gooru.local/types"
)

// ExecuteBackgroundTagMutationFiles executes a snapshotted file-ID mutation
// against the current file state. Ordinary files keep the existing path-based
// reanalysis semantics. Managed files are re-read through StoragePath while
// retaining their logical Path as the database identity, so opaque protected
// storage never requires the logical pathname to exist on disk.
func (c *Client) ExecuteBackgroundTagMutationFiles(operationID string, files []types.FileInfo) error {
	state, found, err := c.GetBackgroundTagMutation(operationID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("background tag mutation is missing")
	}
	if state.ResultReady {
		return nil
	}
	if state.TargetKind != BackgroundTagMutationTargetFileID {
		return fmt.Errorf("background tag mutation %q does not use file-id targets", operationID)
	}
	if len(files) != state.MatchedFiles {
		return fmt.Errorf("background tag mutation target count changed: expected %d files, got %d", state.MatchedFiles, len(files))
	}
	kind, err := backgroundTagMutationKind(state.Mutation, state.Tags)
	if err != nil {
		return err
	}

	analysis := &fileStateAnalysis{
		locationsToUpsert: make(map[string]types.LocationInfo, len(files)),
		potentialMoves:    make(map[string]string),
	}
	externalPaths := make([]string, 0, len(files))
	for _, file := range files {
		if file.StoragePath == "" {
			externalPaths = append(externalPaths, file.Path)
			continue
		}

		metadata, err := c.hasher.FileMetadata(file.StoragePath)
		if err != nil {
			return fmt.Errorf("read managed file metadata for %q: %w", file.Path, err)
		}
		hash := file.Hash
		metadataChanged := metadata.Size != file.Size || metadata.ModTime.Unix() != file.ModTime
		if hash == "" || metadataChanged {
			hash, err = c.hasher.HashFile(file.StoragePath)
			if err != nil {
				return fmt.Errorf("hash managed file %q: %w", file.Path, err)
			}
		}
		isModification := hash != file.Hash
		previousHash := ""
		if isModification {
			previousHash = file.Hash
		}
		location := types.LocationInfo{
			Path:        file.Path,
			StoragePath: file.StoragePath,
			Hash:        hash,
			Size:        metadata.Size,
			ModTime:     metadata.ModTime.Unix(),
			AddedAt:     file.AddedAt,
			Extension:   filepath.Ext(file.Path),
		}
		analysis.allFileData = append(analysis.allFileData, fileData{
			path:         file.Path,
			info:         location,
			wasModified:  isModification,
			previousHash: previousHash,
		})
		analysis.allHashes = append(analysis.allHashes, hash)
		if file.Hash == "" || metadataChanged {
			analysis.locationsToUpsert[file.Path] = location
		}
	}

	if err := c.populateOrphanedTags(analysis.allFileData); err != nil {
		return fmt.Errorf("read tags for modified managed files: %w", err)
	}

	if len(externalPaths) > 0 {
		externalAnalysis, err := c.analyzeFileStates(externalPaths, nil, true)
		if err != nil {
			return err
		}
		analysis.allFileData = append(analysis.allFileData, externalAnalysis.allFileData...)
		analysis.allHashes = append(analysis.allHashes, externalAnalysis.allHashes...)
		for path, location := range externalAnalysis.locationsToUpsert {
			analysis.locationsToUpsert[path] = location
		}
		for hash, path := range externalAnalysis.potentialMoves {
			analysis.potentialMoves[hash] = path
		}
	}
	if len(analysis.allFileData) != len(files) {
		return fmt.Errorf("background tag mutation could not analyze every snapshotted file")
	}

	_, _, err = c.executeTaggingTransaction(analysis, state.Tags, kind, nil, nil, func(tx *databaseTx, affectedCount int64, movesHandled map[string]string) error {
		result := backgroundTagOperationResult(analysis, int(affectedCount), movesHandled)
		return c.persistBackgroundTagMutationResultTx(tx, operationID, result)
	})
	if err == nil {
		c.notifyBackgroundOperationChange()
	}
	return err
}
