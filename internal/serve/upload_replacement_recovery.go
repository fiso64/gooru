package serve

import (
	"errors"
	"fmt"
	"os"
)

// prepareDurableReplacementRecoveryMarkers keeps a hard link to each durable
// replacement's newly uploaded inode before activation renames the staged path.
// The marker is deliberately retained until the operation reaches a terminal
// state. If protected import later moves the logical destination into random
// opaque storage and the process dies before the DB transaction commits, the
// startup orphan sweep may remove that opaque directory entry without deleting
// the upload data: this marker still owns the inode for worker replay.
func prepareDurableReplacementRecoveryMarkers(files []savedUpload) error {
	for _, file := range files {
		if !file.replace || file.status == "error" || file.status == "skipped" {
			continue
		}
		markerPath := file.path + durableUploadActivatedMarkerSuffix
		stagedInfo, stagedExists, err := durableReplacementRegularFile(file.path)
		if err != nil {
			return fmt.Errorf("inspect durable replacement staging: %w", err)
		}
		markerInfo, markerExists, err := durableReplacementRegularFile(markerPath)
		if err != nil {
			return fmt.Errorf("inspect durable replacement recovery marker: %w", err)
		}

		if stagedExists {
			if markerExists {
				if !os.SameFile(stagedInfo, markerInfo) {
					return errors.New("durable replacement recovery marker does not own staged upload")
				}
				continue
			}
			if err := os.Link(file.path, markerPath); err != nil {
				if errors.Is(err, os.ErrExist) {
					return errors.New("durable replacement recovery marker changed during activation")
				}
				return fmt.Errorf("record durable replacement recovery ownership: %w", err)
			}
			continue
		}
		if !markerExists {
			return errors.New("durable replacement staging and recovery marker are missing")
		}

		// A crash can leave a staged checkpoint after activation has already
		// moved the upload to its destination. When the destination is still our
		// inode, leave the staged path absent so activateReplacement can resume its
		// existing backup/no-original state. Otherwise recreate staged input from
		// the marker so a rolled-back or missing destination can activate again.
		finalInfo, finalExists, err := durableReplacementRegularFile(file.destinationPath)
		if err != nil {
			return fmt.Errorf("inspect durable replacement destination: %w", err)
		}
		if finalExists && os.SameFile(markerInfo, finalInfo) {
			continue
		}
		if err := os.Link(markerPath, file.path); err != nil {
			if errors.Is(err, os.ErrExist) {
				return errors.New("durable replacement staging changed during recovery")
			}
			return fmt.Errorf("restore durable replacement staging from recovery marker: %w", err)
		}
	}
	return nil
}

// restoreActivatedReplacementDestinations reconstructs logical replacement
// destinations from their durable marker before an activated task is retried.
// Existing destinations must be the same inode; a different file wins as a
// conflict rather than being overwritten or deleted.
func restoreActivatedReplacementDestinations(files []savedUpload, activated []activatedSavedReplacement) error {
	for _, item := range activated {
		if item.index < 0 || item.index >= len(files) {
			return errors.New("activated replacement index is out of range")
		}
		file := files[item.index]
		markerPath := file.path + durableUploadActivatedMarkerSuffix
		markerInfo, markerExists, err := durableReplacementRegularFile(markerPath)
		if err != nil {
			return fmt.Errorf("inspect durable replacement recovery marker: %w", err)
		}
		finalInfo, finalExists, err := durableReplacementRegularFile(item.replacement.finalPath)
		if err != nil {
			return fmt.Errorf("inspect activated replacement destination: %w", err)
		}
		if !markerExists {
			// Checkpoints created before durable replacement markers were added are
			// still replayable while their activated destination remains present.
			if finalExists {
				continue
			}
			return errors.New("activated replacement data and recovery marker are missing")
		}
		if finalExists {
			if !os.SameFile(markerInfo, finalInfo) {
				return errUploadConflict
			}
			continue
		}
		if err := os.Link(markerPath, item.replacement.finalPath); err != nil {
			if errors.Is(err, os.ErrExist) {
				return errors.New("activated replacement destination changed during recovery")
			}
			return fmt.Errorf("restore activated replacement destination: %w", err)
		}
	}
	return nil
}

func settleDurableReplacementRecoveryMarkers(files []savedUpload) error {
	failures := 0
	for _, file := range files {
		if !file.replace || file.status == "error" || file.status == "skipped" {
			continue
		}
		markerPath := file.path + durableUploadActivatedMarkerSuffix
		if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures++
			continue
		}
		cleanupDurableUploadStagingParents(file.path)
	}
	if failures > 0 {
		return fmt.Errorf("settle %d durable replacement recovery markers", failures)
	}
	return nil
}

func durableReplacementRegularFile(path string) (os.FileInfo, bool, error) {
	if path == "" {
		return nil, false, errors.New("durable replacement recovery path is empty")
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, errors.New("durable replacement recovery path is not a regular file")
	}
	return info, true, nil
}
