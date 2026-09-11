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
		switch {
		case stagedExists && markerExists:
			if !os.SameFile(stagedInfo, markerInfo) {
				return errors.New("durable replacement recovery marker does not own staged upload")
			}
		case stagedExists:
			if err := os.Link(file.path, markerPath); err != nil {
				if errors.Is(err, os.ErrExist) {
					return prepareDurableReplacementRecoveryMarkers(files)
				}
				return fmt.Errorf("record durable replacement recovery ownership: %w", err)
			}
		case markerExists:
			// A previous activation attempt may have renamed the staged link and
			// then rolled back before checkpointing. Recreate the staged input from
			// the marker so the normal activation state machine can resume.
			if err := os.Link(markerPath, file.path); err != nil {
				if errors.Is(err, os.ErrExist) {
					return prepareDurableReplacementRecoveryMarkers(files)
				}
				return fmt.Errorf("restore durable replacement staging from recovery marker: %w", err)
			}
		default:
			return errors.New("durable replacement staging and recovery marker are missing")
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
				return restoreActivatedReplacementDestinations(files, activated)
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
