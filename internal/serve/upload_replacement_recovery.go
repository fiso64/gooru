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

// activateSavedDurableReplacements preserves the existing replacement
// activation semantics, but uses durable ownership-aware rollback if a later
// replacement in the same batch fails to activate.
func activateSavedDurableReplacements(files []savedUpload) ([]activatedSavedReplacement, error) {
	activated := make([]activatedSavedReplacement, 0)
	for i, file := range files {
		if !file.replace {
			continue
		}
		replacement, err := activateReplacement(file.path, file.destinationPath)
		if err != nil {
			rollbackErr := rollbackDurableSavedReplacements(files, activated)
			if rollbackErr != nil {
				return nil, fmt.Errorf("%w; durable replacement rollback failed: %v", err, rollbackErr)
			}
			return nil, err
		}
		activated = append(activated, activatedSavedReplacement{index: i, replacement: replacement})
	}
	return activated, nil
}

func settleDurableSavedReplacements(files []savedUpload, activated []activatedSavedReplacement, response UploadImportResponse) error {
	var errs []error
	for _, item := range activated {
		if item.index < 0 || item.index >= len(files) {
			errs = append(errs, errors.New("activated replacement index is out of range"))
			continue
		}
		if item.index < len(response.Files) && response.Files[item.index].Status == "imported" {
			commitReplacement(item.replacement)
			continue
		}
		if err := rollbackDurableReplacement(files[item.index], item.replacement); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func rollbackDurableSavedReplacements(files []savedUpload, activated []activatedSavedReplacement) error {
	var errs []error
	for i := len(activated) - 1; i >= 0; i-- {
		item := activated[i]
		if item.index < 0 || item.index >= len(files) {
			errs = append(errs, errors.New("activated replacement index is out of range"))
			continue
		}
		if err := rollbackDurableReplacement(files[item.index], item.replacement); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// rollbackDurableReplacement only removes a destination when the durable marker
// proves it is the uploaded inode. A different destination is treated as a
// conflict and preserved, with the original backup left available for recovery.
func rollbackDurableReplacement(file savedUpload, replacement activatedReplacement) error {
	markerInfo, markerExists, err := durableReplacementRegularFile(file.path + durableUploadActivatedMarkerSuffix)
	if err != nil {
		return fmt.Errorf("inspect durable replacement recovery marker: %w", err)
	}
	backupInfo, backupExists, err := durableReplacementRegularFile(replacement.backupPath)
	if err != nil {
		return fmt.Errorf("inspect preserved original before replacement rollback: %w", err)
	}
	finalInfo, finalExists, err := durableReplacementRegularFile(replacement.finalPath)
	if err != nil {
		return fmt.Errorf("inspect replacement rollback destination: %w", err)
	}
	if !markerExists {
		return rollbackDurableReplacementWithoutMarker(replacement, backupInfo, backupExists, finalInfo, finalExists)
	}

	if replacement.hadOriginal && !backupExists {
		if !finalExists {
			return errors.New("failed to restore original file after replacement failure")
		}
		if os.SameFile(markerInfo, finalInfo) {
			return errors.New("preserved original is missing during durable replacement rollback")
		}
		// A previous rollback already restored the original (or a later actor
		// replaced it). With no backup left, never delete the current path.
		return nil
	}

	if replacement.hadOriginal && backupExists && finalExists && os.SameFile(backupInfo, finalInfo) {
		if err := os.Remove(replacement.backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("failed to finalize original file restoration")
		}
		return nil
	}

	if finalExists {
		if !os.SameFile(markerInfo, finalInfo) {
			return errUploadConflict
		}
		if err := os.Remove(replacement.finalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("failed to remove rejected replacement")
		}
	}

	if replacement.hadOriginal {
		if !backupExists {
			return errors.New("failed to restore original file after replacement failure")
		}
		if err := os.Link(replacement.backupPath, replacement.finalPath); err != nil {
			if errors.Is(err, os.ErrExist) {
				return errUploadConflict
			}
			return errors.New("failed to restore original file after replacement failure")
		}
		if err := os.Remove(replacement.backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("failed to finalize original file restoration")
		}
		return nil
	}

	_ = os.Remove(replacement.backupPath)
	if err := os.Remove(replacement.noOriginalMarkerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("failed to clear replacement state after rollback")
	}
	return nil
}

// rollbackDurableReplacementWithoutMarker handles checkpoints created before
// durable ownership markers existed. Without inode proof it never removes an
// existing destination; it can only restore a missing destination from the
// preserved original or finish an already-restored rollback.
func rollbackDurableReplacementWithoutMarker(replacement activatedReplacement, backupInfo os.FileInfo, backupExists bool, finalInfo os.FileInfo, finalExists bool) error {
	if replacement.hadOriginal {
		if !backupExists {
			if finalExists {
				return nil
			}
			return errors.New("failed to restore original file after replacement failure")
		}
		if finalExists {
			if os.SameFile(backupInfo, finalInfo) {
				if err := os.Remove(replacement.backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					return errors.New("failed to finalize original file restoration")
				}
				return nil
			}
			return errUploadConflict
		}
		if err := os.Link(replacement.backupPath, replacement.finalPath); err != nil {
			if errors.Is(err, os.ErrExist) {
				return errUploadConflict
			}
			return errors.New("failed to restore original file after replacement failure")
		}
		if err := os.Remove(replacement.backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("failed to finalize original file restoration")
		}
		return nil
	}
	if finalExists {
		return errUploadConflict
	}
	_ = os.Remove(replacement.backupPath)
	if err := os.Remove(replacement.noOriginalMarkerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("failed to clear replacement state after rollback")
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
