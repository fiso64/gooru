package service

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"gooru.local/gooru/internal/database"
	"gooru.local/gooru/internal/hashing"
	"gooru.local/gooru/internal/types"
)

// Service encapsulates the core business logic.
type Service struct {
	Store *database.Store
}

// NewService creates a new Service.
func NewService(store *database.Store) *Service {
	return &Service{Store: store}
}

// TagFile tags a single file with the given tags.
func (s *Service) TagFile(filePath string, tags []string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return err // File doesn't exist or is not accessible
	}

	hash, err := hashing.HashFile(absPath)
	if err != nil {
		return err
	}

	tx, err := s.Store.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // Rollback on error

	if err := s.Store.GetOrCreateContent(tx, hash); err != nil {
		return err
	}

	ext := filepath.Ext(absPath)
	// Use UpdateContentLocation to handle file moves correctly.
	if err := s.Store.UpdateContentLocation(tx, hash, absPath, info.Size(), info.ModTime().Unix(), ext); err != nil {
		return err
	}

	for _, tagName := range tags {
		tagID, err := s.Store.GetOrCreateTag(tx, tagName)
		if err != nil {
			return err
		}

		if err := s.Store.AssociateTag(tx, hash, tagID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// UntagFile untags a single file with the given tags.
func (s *Service) UntagFile(filePath string, tags []string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}

	hash, err := s.Store.FindContentHashByPath(absPath)
	if err != nil {
		return err
	}
	if hash == "" {
		return fmt.Errorf("file not found in database: %s", filePath)
	}

	tx, err := s.Store.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, tagName := range tags {
		tagID, err := s.Store.GetTagID(tagName)
		if err != nil {
			if err == sql.ErrNoRows {
				continue // Tag doesn't exist, so nothing to remove.
			}
			return err // A real error occurred.
		}

		if err := s.Store.DisassociateTag(tx, hash, tagID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetTagsForFile retrieves all tags for a given file.
func (s *Service) GetTagsForFile(filePath string) ([]string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	hash, err := s.Store.FindContentHashByPath(absPath)
	if err != nil {
		return nil, err
	}
	if hash == "" {
		// Return empty slice if file is not in DB, it's not an error.
		return []string{}, nil
	}

	return s.Store.GetTagsForContent(hash)
}

// ListAllFiles lists all files known to the system.
func (s *Service) ListAllFiles() ([]string, error) {
    return s.Store.ListAllFiles()
}

// ListFilesByTag lists all files associated with a given tag.
func (s *Service) ListFilesByTag(tag string) ([]string, error) {
    return s.Store.ListFilesByTag(tag)
}

// ListFilesByTagsAnd lists all files associated with a given set of tags (AND query).
func (s *Service) ListFilesByTagsAnd(tags []string) ([]string, error) {
    return s.Store.ListFilesByTagsAnd(tags)
}

// GetAllFilesInfo gets detailed info for all files known to the system.
func (s *Service) GetAllFilesInfo() ([]types.FileInfo, error) {
    return s.Store.GetAllFilesInfo()
}

// GetFilesInfoByTag gets detailed info for all files associated with a given tag.
func (s *Service) GetFilesInfoByTag(tag string) ([]types.FileInfo, error) {
    return s.Store.GetFilesInfoByTag(tag)
}

// GetFilesInfoByTagsAnd gets detailed info for all files associated with a given set of tags (AND query).
func (s *Service) GetFilesInfoByTagsAnd(tags []string) ([]types.FileInfo, error) {
    return s.Store.GetFilesInfoByTagsAnd(tags)
}

// NeedsRelink performs a fast check using filesystem metadata to see if a relink is necessary.
func (s *Service) NeedsRelink(dirs []string) (bool, error) {
	absDirs, err := toAbsolutePaths(dirs)
	if err != nil {
		return false, err
	}
	dbLocations, err := s.Store.GetLocationsForDirs(absDirs)
	if err != nil {
		return false, err
	}

	for path, info := range dbLocations {
		fsInfo, err := os.Stat(path)
		if os.IsNotExist(err) {
			return true, nil // File in DB is missing from disk.
		}
		if err != nil {
			return true, nil // Can't stat the file, something is wrong.
		}

		if fsInfo.Size() != info.Size || fsInfo.ModTime().Unix() != info.ModTime {
			return true, nil // Metadata mismatch, file has likely changed.
		}
	}

	return false, nil // Everything matches.
}

// Relink performs a high-performance concurrent scan of the given directories
// and updates the database with the findings.
func (s *Service) Relink(dirs []string) (types.RelinkStats, error) {
	absDirs, err := toAbsolutePaths(dirs)
	if err != nil {
		return types.RelinkStats{}, err
	}

	// 1. Get initial state from the database. This must happen first.
	dbLocations, err := s.Store.GetLocationsForDirs(absDirs)
	if err != nil {
		return types.RelinkStats{}, fmt.Errorf("could not get db locations: %w", err)
	}
	sizeToHashes, err := s.Store.GetSizeToHashesMap()
	if err != nil {
		return types.RelinkStats{}, fmt.Errorf("could not build size-to-hash map: %w", err)
	}
	knownHashes, err := s.Store.GetAllContentHashes()
	if err != nil {
		return types.RelinkStats{}, fmt.Errorf("could not get known hashes: %w", err)
	}
	hashToTagsCache, err := s.Store.GetHashToTagsCacheMap()
	if err != nil {
		return types.RelinkStats{}, fmt.Errorf("could not get tags cache: %w", err)
	}

	// 2. Perform the intelligent, targeted filesystem scan.
	fsLocations, filesScanned := s.scanDirsConcurrently(absDirs, sizeToHashes)

	// 3. Compute the difference ("diff") between the two states.
	toAdd := make(map[string]types.LocationInfo)
	toRemove := make([]string, 0)

	// Check for new or changed files on disk
	for path, fsInfo := range fsLocations {
		dbInfo, existsInDb := dbLocations[path]
		// Add if path is new, or if path exists but hash is different.
		if !existsInDb || dbInfo.Hash != fsInfo.Hash {
			if _, contentIsKnown := knownHashes[fsInfo.Hash]; contentIsKnown {
				// Content is known, so preserve its tags cache.
				fsInfo.TagsCache = hashToTagsCache[fsInfo.Hash]
				toAdd[path] = fsInfo
			}
		}
	}

	// Check for files that were removed from disk or whose content changed
	for path, dbInfo := range dbLocations {
		fsInfo, existsOnFs := fsLocations[path]
		// Remove if path no longer exists on disk, or if it exists but now has a different hash
		if !existsOnFs || fsInfo.Hash != dbInfo.Hash {
			toRemove = append(toRemove, path)
		}
	}

	// 4. Apply the changes transactionally.
	stats, err := s.Store.ApplyRelinkChanges(toAdd, toRemove)
	if err != nil {
		return types.RelinkStats{}, err
	}

	stats.FilesScanned = filesScanned
	return stats, nil
}

func toAbsolutePaths(paths []string) ([]string, error) {
	absPaths := make([]string, len(paths))
	for i, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		absPaths[i] = abs
	}
	return absPaths, nil
}