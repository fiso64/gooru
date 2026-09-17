package serve

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const durableUploadStagingRootName = ".gooru-upload-staging"

func durableUploadStagingDir(root, operationID string) (string, error) {
	if !validDurableUploadOperationID(operationID) {
		return "", errors.New("invalid upload operation id")
	}
	return filepath.Join(root, durableUploadStagingRootName, operationID), nil
}

func prepareDurableUploadStagingDir(root, operationID string) (string, error) {
	operationDir, err := durableUploadStagingDir(root, operationID)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve upload target: %w", err)
	}
	resolvedRoot, ok := resolvedContainmentPath(filepath.Clean(rootAbs))
	if !ok {
		return "", errors.New("cannot safely resolve upload target")
	}
	operationAbs, err := filepath.Abs(operationDir)
	if err != nil {
		return "", fmt.Errorf("resolve upload staging directory: %w", err)
	}
	requireContained := func() error {
		resolvedOperationDir, ok := resolvedContainmentPath(filepath.Clean(operationAbs))
		if !ok {
			return errors.New("cannot safely resolve upload staging directory")
		}
		if !pathContainsOrEquals(resolvedRoot, resolvedOperationDir) {
			return errors.New("upload staging directory escapes upload target")
		}
		return nil
	}
	if err := requireContained(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(operationDir, 0700); err != nil {
		return "", err
	}
	if err := requireContained(); err != nil {
		return "", err
	}
	return operationDir, nil
}

func validDurableUploadOperationID(operationID string) bool {
	return strings.HasPrefix(operationID, "operation-") && filepath.Base(operationID) == operationID && !strings.ContainsAny(operationID, `/\\`)
}

func (s *Server) stageDurableMultipartUpload(r *http.Request, operationID string) (tags []string, saved []savedUpload, retErr error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, nil, multipartUploadError{message: fmt.Sprintf("multipart/form-data upload is required: %v", err), err: err}
	}
	stagingTarget, err := s.uploadTarget("")
	if err != nil {
		return nil, nil, multipartUploadError{code: "invalid_upload_target", message: err.Error(), err: err}
	}
	initialOperationDir, err := prepareDurableUploadStagingDir(stagingTarget.Path, operationID)
	if err != nil {
		return nil, nil, multipartUploadError{message: "failed to prepare upload staging directory", err: err}
	}
	initialDir, err := os.MkdirTemp(initialOperationDir, "request-")
	if err != nil {
		return nil, nil, multipartUploadError{message: "failed to prepare upload staging directory", err: err}
	}
	ownedStagingTarget := stagingTarget
	ownedStagingTarget.Path = initialDir
	stagingDirs := []string{initialDir}

	var targetID, conflictRequested, addedAtStrategyRequested string
	var queueFirstTimeValue, queueLastTimeValue string
	var targetSeen, conflictSeen, addedAtStrategySeen, queueFirstTimeSeen, queueLastTimeSeen bool
	tagValues := make([]string, 0)
	itemTagValues := make([]string, 0)
	sourceModTimeValues := make([]string, 0)
	queueTimeValues := make([]string, 0)
	queueIndexValues := make([]string, 0)
	queueTotalValues := make([]string, 0)
	streamed := make([]streamedUpload, 0)
	fileCount := 0
	defer func() {
		if retErr != nil {
			removeStreamedUploads(streamed)
			removeSavedUploads(saved)
			for _, dir := range stagingDirs {
				_ = os.RemoveAll(dir)
			}
		}
	}()

	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, saved, multipartUploadError{message: fmt.Sprintf("failed to read multipart upload: %v", nextErr), err: nextErr}
		}
		formName := part.FormName()
		fileName := part.FileName()
		if fileName == "" {
			value, fieldErr := readUploadField(part)
			_ = part.Close()
			if fieldErr != nil {
				return nil, saved, fieldErr
			}
			switch formName {
			case "target_id":
				if !targetSeen {
					targetID, targetSeen = value, true
				}
			case "conflict_policy":
				if !conflictSeen {
					conflictRequested, conflictSeen = value, true
				}
			case "tags":
				tagValues = append(tagValues, value)
			case "item_tags":
				itemTagValues = append(itemTagValues, value)
			case "source_modtime_ms":
				sourceModTimeValues = append(sourceModTimeValues, value)
			case "added_at_strategy":
				if !addedAtStrategySeen {
					addedAtStrategyRequested, addedAtStrategySeen = value, true
				}
			case "queue_time_ms":
				queueTimeValues = append(queueTimeValues, value)
			case "queue_first_time_ms":
				if !queueFirstTimeSeen {
					queueFirstTimeValue, queueFirstTimeSeen = value, true
				}
			case "queue_last_time_ms":
				if !queueLastTimeSeen {
					queueLastTimeValue, queueLastTimeSeen = value, true
				}
			case "queue_index":
				queueIndexValues = append(queueIndexValues, value)
			case "queue_total":
				queueTotalValues = append(queueTotalValues, value)
			}
			continue
		}
		if formName != "files" && formName != "file" {
			_ = part.Close()
			continue
		}
		fileCount++
		streamedFile, fileErr := s.streamUploadPart(ownedStagingTarget, fileName, part)
		_ = part.Close()
		if fileErr != nil {
			return nil, saved, fileErr
		}
		streamed = append(streamed, streamedFile)
	}

	if fileCount == 0 {
		return nil, saved, multipartUploadError{message: "at least one file is required", err: errors.New("missing upload file")}
	}
	for i := range streamed {
		if i >= len(sourceModTimeValues) {
			break
		}
		streamed[i].sourceModTime = parseUploadSourceModTime(sourceModTimeValues[i])
	}
	target, err := s.uploadTarget(targetID)
	if err != nil {
		return nil, saved, multipartUploadError{code: "invalid_upload_target", message: err.Error(), err: err}
	}
	if err := os.MkdirAll(target.Path, 0700); err != nil {
		return nil, saved, multipartUploadError{message: "failed to prepare upload directory", err: err}
	}
	targetDir := initialDir
	if filepath.Clean(stagingTarget.Path) != filepath.Clean(target.Path) {
		targetOperationDir, err := prepareDurableUploadStagingDir(target.Path, operationID)
		if err != nil {
			return nil, saved, multipartUploadError{message: "failed to prepare upload staging directory", err: err}
		}
		targetDir, err = os.MkdirTemp(targetOperationDir, "request-")
		if err != nil {
			return nil, saved, multipartUploadError{message: "failed to prepare upload staging directory", err: err}
		}
		stagingDirs = append(stagingDirs, targetDir)
	}
	addedAtStrategy, err := uploadAddedAtStrategy(addedAtStrategyRequested, target.AddedAtStrategy)
	if err != nil {
		return nil, saved, multipartUploadError{message: err.Error(), err: err}
	}
	queueFallback := time.Now().UTC()
	queueFirstTime := parseUploadSourceModTime(queueFirstTimeValue)
	queueLastTime := parseUploadSourceModTime(queueLastTimeValue)
	for i := range streamed {
		queueTime := queueFallback
		if i < len(queueTimeValues) {
			if parsed := parseUploadSourceModTime(queueTimeValues[i]); !parsed.IsZero() {
				queueTime = parsed
			}
		}
		queueIndex := parseUploadOrdinal(queueIndexValues, i, i)
		queueTotal := parseUploadOrdinal(queueTotalValues, i, len(streamed))
		streamed[i].addedAt = resolveUploadAddedAt(addedAtStrategy, streamed[i].sourceModTime, queueTime, queueFirstTime, queueLastTime, queueIndex, queueTotal)
		streamed[i].addedOrder = resolveUploadAddedOrder(addedAtStrategy, queueIndex, queueTotal)
	}
	conflictPolicy, err := uploadConflictPolicy(conflictRequested)
	if err != nil {
		return nil, saved, multipartUploadError{message: err.Error(), err: err}
	}

	reserved := make(map[string]struct{}, len(streamed))
	if segmentIndex, segmented, segmentErr := durableUploadSegmentIndex(r); segmentErr != nil {
		return nil, saved, multipartUploadError{message: segmentErr.Error(), err: segmentErr}
	} else if segmented {
		segmentStore, ok := any(s.backgroundOperations).(durableUploadSegmentStore)
		if !ok {
			return nil, saved, multipartUploadError{message: "segmented upload service is not configured", err: errors.New("segmented upload service is not configured")}
		}
		priorReserved, reserveErr := durableUploadPriorSegmentDestinations(segmentStore, operationID, segmentIndex)
		if reserveErr != nil {
			return nil, saved, multipartUploadError{message: "failed to load prior upload segment destinations", err: reserveErr}
		}
		for path := range priorReserved {
			reserved[path] = struct{}{}
		}
	}
	for i := range streamed {
		file := &streamed[i]
		if file.status == "error" {
			saved = append(saved, savedUpload{name: file.name, size: file.size, targetID: target.ID, status: "error", error: file.error})
			continue
		}
		path, destinationErr := chooseDurableUploadDestination(target.Path, file.name, conflictPolicy, reserved)
		if destinationErr != nil {
			return nil, saved, uploadFileError{name: file.name, err: destinationErr}
		}
		stagedPath, moveErr := moveStreamedUploadIntoDir(file.path, targetDir, file.name)
		if moveErr != nil {
			return nil, saved, uploadFileError{name: file.name, err: moveErr}
		}
		if err := applyUploadedSourceModTime(stagedPath, file.sourceModTime, s.cfg.Uploads.PreserveModTime); err != nil {
			_ = os.Remove(stagedPath)
			return nil, saved, uploadFileError{name: file.name, err: err}
		}
		file.path = ""
		saved = append(saved, savedUpload{name: file.name, path: stagedPath, destinationPath: path, size: file.size, targetID: target.ID, sourceModTime: file.sourceModTime, addedAt: file.addedAt, addedOrder: file.addedOrder, conflictPolicy: conflictPolicy})
		reserved[path] = struct{}{}
	}
	if err := attachUploadItemTags(saved, itemTagValues); err != nil {
		return nil, saved, err
	}
	if filepath.Clean(initialDir) != filepath.Clean(targetDir) {
		_ = os.Remove(initialDir)
	}
	if err := r.Context().Err(); err != nil {
		return nil, saved, errUploadReceivingCanceled
	}
	parsedTags := parseUploadTags(tagValues)
	if err := validateUploadTags(parsedTags, saved); err != nil {
		return nil, saved, multipartUploadError{message: err.Error(), err: err}
	}
	if len(saved) == 1 && saved[0].status == "error" {
		if saved[0].error == errUploadTooLarge.Error() {
			return parsedTags, saved, nil
		}
		return nil, saved, uploadFileError{name: saved[0].name, err: errors.New(saved[0].error)}
	}
	return parsedTags, saved, nil
}

func chooseDurableUploadDestination(dir, name, conflictPolicy string, reserved map[string]struct{}) (string, error) {
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	if base == "" {
		base = "upload"
	}
	for i := 0; i < 10_000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		path := filepath.Join(dir, candidate)
		_, planned := reserved[path]
		_, statErr := os.Stat(path)
		if statErr == nil || planned {
			if conflictPolicy == "error" {
				return "", errUploadConflict
			}
			continue
		}
		if errors.Is(statErr, os.ErrNotExist) {
			return path, nil
		}
		return "", errors.New("failed to inspect upload destination")
	}
	return "", errors.New("could not choose a non-conflicting upload filename")
}
