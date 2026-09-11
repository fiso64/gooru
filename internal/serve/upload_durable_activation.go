package serve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const durableUploadActivatedMarkerSuffix = ".activated"

func durableUploadNeedsActivation(file savedUpload) bool {
	return !file.replace && file.status != "error" && file.status != "skipped" && file.destinationPath != "" && filepath.Clean(file.path) != filepath.Clean(file.destinationPath)
}

func activateSavedDurableUploads(files []savedUpload) ([]activatedSavedReplacement, error) {
	for _, file := range files {
		if !durableUploadNeedsActivation(file) {
			continue
		}
		if err := activateDurableUploadDestination(file.path, file.destinationPath); err != nil {
			return nil, uploadFileError{name: file.name, err: err}
		}
	}
	return activateSavedReplacements(files)
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

	if !markerExists {
		if !stagedExists {
			return errors.New("durable staged upload is missing")
		}
		if destinationExists {
			return errUploadConflict
		}
		marker, markerErr := os.OpenFile(markerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if markerErr != nil {
			if errors.Is(markerErr, os.ErrExist) {
				return activateDurableUploadDestination(stagedPath, destinationPath)
			}
			return errors.New("failed to record durable upload activation")
		}
		if closeErr := marker.Close(); closeErr != nil {
			_ = os.Remove(markerPath)
			return errors.New("failed to record durable upload activation")
		}
		markerExists = true
	}

	if !stagedExists {
		if destinationExists && markerExists {
			return nil
		}
		return errors.New("durable upload activation state is incomplete")
	}
	if destinationExists {
		stagedInfo, stagedErr := os.Stat(stagedPath)
		destinationInfo, destinationErr := os.Stat(destinationPath)
		if stagedErr != nil || destinationErr != nil || !os.SameFile(stagedInfo, destinationInfo) {
			_ = os.Remove(markerPath)
			return errUploadConflict
		}
	} else if err := os.Link(stagedPath, destinationPath); err != nil {
		_ = os.Remove(markerPath)
		if errors.Is(err, os.ErrExist) {
			return errUploadConflict
		}
		return errors.New("failed to activate durable upload")
	}
	if err := os.Remove(stagedPath); err != nil {
		_ = os.Remove(destinationPath)
		_ = os.Remove(markerPath)
		return errors.New("failed to finalize durable upload activation")
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
		if exists, err := durableUploadPathExists(markerPath); err != nil {
			failures++
			continue
		} else if exists {
			if err := os.Remove(file.destinationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures++
			}
			if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures++
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
	if failures > 0 {
		return fmt.Errorf("settle %d durable upload activation artifacts", failures)
	}
	return nil
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
