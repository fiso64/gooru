package serve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const durableUploadActivatedMarkerSuffix = ".activated"

func durableUploadNeedsActivation(file savedUpload) bool {
	return file.status != "error" && file.status != "skipped" && file.destinationPath != "" && filepath.Clean(file.path) != filepath.Clean(file.destinationPath)
}

func activateSavedDurableUploads(files []savedUpload) error {
	for _, file := range files {
		if !durableUploadNeedsActivation(file) {
			continue
		}
		if err := activateDurableUploadDestination(file.path, file.destinationPath); err != nil {
			rollbackErr := restoreDurableNonreplacementActivations(files)
			if rollbackErr != nil {
				return uploadFileError{name: file.name, err: fmt.Errorf("%w; durable activation rollback failed: %v", err, rollbackErr)}
			}
			return uploadFileError{name: file.name, err: err}
		}
	}
	if err := finalizeDurableNonreplacementActivations(files); err != nil {
		rollbackErr := restoreDurableNonreplacementActivations(files)
		if rollbackErr != nil {
			return errors.Join(fmt.Errorf("finalize durable upload activation: %w", err), fmt.Errorf("durable activation rollback failed: %w", rollbackErr))
		}
		return fmt.Errorf("finalize durable upload activation: %w", err)
	}
	return nil
}

func activateDurableUploadDestination(stagedPath, destinationPath string) error {
	markerPath := stagedPath + durableUploadActivatedMarkerSuffix
	markerExists, err := durableUploadPathExists(markerPath)
	if err != nil {
		return errors.New("failed to inspect durable upload activation")
	}
	stagedExists, err := durableUploadPathExists(stagedPath)
	if err != nil {
		return errors.New("failed to inspect staged upload")
	}
	destinationExists, err := durableUploadPathExists(destinationPath)
	if err != nil {
		return errors.New("failed to inspect upload destination")
	}

	if markerExists && stagedExists {
		same, sameErr := durableUploadSameFile(markerPath, stagedPath)
		if sameErr != nil {
			return errors.New("failed to inspect durable upload activation ownership")
		}
		if !same {
			// Upgrade the old empty-marker format only while the staged inode is
			// still available to prove which destination, if any, belongs to us.
			if destinationExists {
				destinationSame, destinationErr := durableUploadSameFile(stagedPath, destinationPath)
				if destinationErr != nil {
					return errors.New("failed to inspect upload destination ownership")
				}
				if !destinationSame {
					_ = os.Remove(markerPath)
					return errUploadConflict
				}
			}
			if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.New("failed to replace durable upload activation marker")
			}
			if err := os.Link(stagedPath, markerPath); err != nil {
				return errors.New("failed to record durable upload activation ownership")
			}
			markerExists = true
		}
	}

	if markerExists && !stagedExists {
		if !destinationExists {
			return errors.New("durable upload activation state is incomplete")
		}
		same, sameErr := durableUploadSameFile(markerPath, destinationPath)
		if sameErr != nil {
			return errors.New("failed to inspect durable upload activation ownership")
		}
		if !same {
			// A legacy/stale marker cannot prove ownership after the staged inode
			// is gone. Prefer a conflict over deleting or importing a racing file.
			_ = os.Remove(markerPath)
			return errUploadConflict
		}
		return nil
	}

	if !markerExists {
		if !stagedExists {
			return errors.New("durable staged upload is missing")
		}
		if destinationExists {
			return errUploadConflict
		}
		if err := os.Link(stagedPath, markerPath); err != nil {
			if errors.Is(err, os.ErrExist) {
				return activateDurableUploadDestination(stagedPath, destinationPath)
			}
			return errors.New("failed to record durable upload activation ownership")
		}
		markerExists = true
	}

	if destinationExists {
		same, sameErr := durableUploadSameFile(markerPath, destinationPath)
		if sameErr != nil {
			return errors.New("failed to inspect upload destination ownership")
		}
		if !same {
			_ = os.Remove(markerPath)
			return errUploadConflict
		}
		return nil
	}
	if err := os.Link(markerPath, destinationPath); err != nil {
		_ = os.Remove(markerPath)
		if errors.Is(err, os.ErrExist) {
			return errUploadConflict
		}
		return errors.New("failed to activate durable upload")
	}
	return nil
}

func finalizeDurableNonreplacementActivations(files []savedUpload) error {
	failures := 0
	for _, file := range files {
		if !durableUploadNeedsActivation(file) {
			continue
		}
		markerPath := file.path + durableUploadActivatedMarkerSuffix
		markerExists, markerErr := durableUploadPathExists(markerPath)
		stagedExists, stagedErr := durableUploadPathExists(file.path)
		if markerErr != nil || stagedErr != nil || !markerExists {
			failures++
			continue
		}
		if !stagedExists {
			continue
		}
		same, sameErr := durableUploadSameFile(markerPath, file.path)
		if sameErr != nil || !same {
			failures++
			continue
		}
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("finalize %d durable upload staging links", failures)
	}
	return nil
}

// restoreDurableNonreplacementActivations returns a failed activation batch to
// its staged state. The hard-link marker lets this avoid deleting a destination
// that another actor replaced after activation.
func restoreDurableNonreplacementActivations(files []savedUpload) error {
	failures := 0
	for _, file := range files {
		if !durableUploadNeedsActivation(file) {
			continue
		}
		markerPath := file.path + durableUploadActivatedMarkerSuffix
		markerExists, err := durableUploadPathExists(markerPath)
		if err != nil {
			failures++
			continue
		}
		if !markerExists {
			continue
		}

		stagedExists, stagedErr := durableUploadPathExists(file.path)
		if stagedErr != nil {
			failures++
			continue
		}
		if stagedExists {
			same, sameErr := durableUploadSameFile(markerPath, file.path)
			if sameErr != nil || !same {
				failures++
				continue
			}
		} else if err := os.Link(markerPath, file.path); err != nil {
			failures++
			continue
		}

		destinationExists, destinationErr := durableUploadPathExists(file.destinationPath)
		if destinationErr != nil {
			failures++
			continue
		}
		if destinationExists {
			same, sameErr := durableUploadSameFile(markerPath, file.destinationPath)
			if sameErr != nil {
				failures++
				continue
			}
			if same {
				if err := os.Remove(file.destinationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					failures++
					continue
				}
			}
		}
		if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("restore %d durable upload activation artifacts", failures)
	}
	return nil
}

func rollbackDurableNonreplacementActivations(files []savedUpload) error {
	failures := 0
	for _, file := range files {
		if !durableUploadNeedsActivation(file) {
			continue
		}
		markerPath := file.path + durableUploadActivatedMarkerSuffix
		markerExists, err := durableUploadPathExists(markerPath)
		if err != nil {
			failures++
			continue
		}
		if markerExists {
			destinationExists, destinationErr := durableUploadPathExists(file.destinationPath)
			if destinationErr != nil {
				failures++
				continue
			}
			if destinationExists {
				same, sameErr := durableUploadSameFile(markerPath, file.destinationPath)
				if sameErr != nil {
					failures++
					continue
				}
				if same {
					if err := os.Remove(file.destinationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
						failures++
						continue
					}
				}
			}
			if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures++
				continue
			}
		}
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures++
		}
		cleanupDurableUploadStagingParents(file.path)
	}
	if failures > 0 {
		return fmt.Errorf("rollback %d durable upload activation artifacts", failures)
	}
	return nil
}

func settleDurableNonreplacementActivations(files []savedUpload) error {
	failures := 0
	for _, file := range files {
		if !durableUploadNeedsActivation(file) {
			continue
		}
		markerPath := file.path + durableUploadActivatedMarkerSuffix
		if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures++
		}
		cleanupDurableUploadStagingParents(file.path)
	}
	var result error
	if failures > 0 {
		result = fmt.Errorf("settle %d durable upload activation artifacts", failures)
	}
	return result
}

func durableStagedUploads(files []savedUpload) []StagedUpload {
	staged := stagedUploads(files)
	for i, file := range files {
		if durableUploadNeedsActivation(file) {
			staged[i].Path = file.destinationPath
			staged[i].AnalysisPath = file.destinationPath
		}
	}
	return staged
}

func cleanupDurableUploadStagingParents(stagedPath string) {
	dir := filepath.Dir(stagedPath)
	_ = os.Remove(dir)
	if filepath.Base(filepath.Dir(dir)) == durableUploadStagingRootName {
		_ = os.Remove(filepath.Dir(dir))
	}
}

func durableUploadPathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func durableUploadSameFile(left, right string) (bool, error) {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false, err
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false, err
	}
	return os.SameFile(leftInfo, rightInfo), nil
}
