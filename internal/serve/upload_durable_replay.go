package serve

import (
	"errors"
	"os"
)

// restoreDurableNonreplacementDestinations reconstructs the logical import
// path for an already-activated non-replacement upload. Protected imports move
// that path into opaque storage before the metadata transaction commits; after
// a process crash startup orphan cleanup may remove the uncommitted opaque link.
// The retained activation marker is an ownership hard link, so replay can
// safely recreate a missing destination without overwriting a racing file.
func restoreDurableNonreplacementDestinations(files []savedUpload) error {
	for _, file := range files {
		if !durableUploadNeedsActivation(file) {
			continue
		}
		markerPath := file.path + durableUploadActivatedMarkerSuffix
		markerExists, err := durableUploadPathExists(markerPath)
		if err != nil {
			return uploadFileError{name: file.name, err: errors.New("failed to inspect durable upload activation marker")}
		}
		if !markerExists {
			return uploadFileError{name: file.name, err: errors.New("durable upload activation marker is missing")}
		}

		destinationExists, err := durableUploadPathExists(file.destinationPath)
		if err != nil {
			return uploadFileError{name: file.name, err: errors.New("failed to inspect durable upload destination")}
		}
		if destinationExists {
			same, sameErr := durableUploadSameFile(markerPath, file.destinationPath)
			if sameErr != nil {
				return uploadFileError{name: file.name, err: errors.New("failed to inspect durable upload destination ownership")}
			}
			if !same {
				return uploadFileError{name: file.name, err: errUploadConflict}
			}
			continue
		}

		if err := os.Link(markerPath, file.destinationPath); err != nil {
			if errors.Is(err, os.ErrExist) {
				same, sameErr := durableUploadSameFile(markerPath, file.destinationPath)
				if sameErr != nil {
					return uploadFileError{name: file.name, err: errors.New("failed to inspect durable upload destination ownership")}
				}
				if same {
					continue
				}
				return uploadFileError{name: file.name, err: errUploadConflict}
			}
			return uploadFileError{name: file.name, err: errors.New("failed to restore durable upload destination")}
		}
	}
	return nil
}
