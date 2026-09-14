package serve

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxUploadFieldBytes = 1 << 20

type multipartUploadError struct {
	code    string
	message string
	err     error
}

func (e multipartUploadError) Error() string {
	if e.message != "" {
		return e.message
	}
	return e.err.Error()
}

func (e multipartUploadError) Unwrap() error { return e.err }

func writeMultipartUploadError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) || errors.Is(err, errUploadTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), uploadErrorDetails(err))
		return
	}
	var requestErr multipartUploadError
	if errors.As(err, &requestErr) {
		code := requestErr.code
		if code == "" {
			code = "invalid_request"
		}
		writeError(w, http.StatusBadRequest, code, requestErr.Error(), uploadErrorDetails(err))
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), uploadErrorDetails(err))
}

type streamedUpload struct {
	name          string
	path          string
	size          int64
	status        string
	error         string
	sourceModTime time.Time
	addedAt       time.Time
}

func (s *Server) stageMultipartUpload(r *http.Request) (tags []string, saved []savedUpload, retErr error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, nil, multipartUploadError{message: fmt.Sprintf("multipart/form-data upload is required: %v", err), err: err}
	}
	stagingTarget, err := s.uploadTarget("")
	if err != nil {
		return nil, nil, multipartUploadError{code: "invalid_upload_target", message: err.Error(), err: err}
	}
	if err := os.MkdirAll(stagingTarget.Path, 0700); err != nil {
		return nil, nil, multipartUploadError{message: "failed to prepare upload staging directory", err: err}
	}

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
		streamedFile, fileErr := s.streamUploadPart(stagingTarget, fileName, part)
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
		// The resolved value is copied into saved/staged state before async job submission.
		streamed[i].addedAt = resolveUploadAddedAt(addedAtStrategy, streamed[i].sourceModTime, queueTime, queueFirstTime, queueLastTime, queueIndex, queueTotal)
	}
	conflictPolicy, err := uploadConflictPolicy(conflictRequested, s.cfg.Uploads.ConflictPolicy)
	if err != nil {
		return nil, saved, multipartUploadError{message: err.Error(), err: err}
	}
	if err := os.MkdirAll(target.Path, 0700); err != nil {
		return nil, saved, multipartUploadError{message: "failed to prepare upload directory", err: err}
	}

	for i := range streamed {
		file := &streamed[i]
		if file.status == "error" {
			saved = append(saved, savedUpload{name: file.name, size: file.size, targetID: target.ID, status: "error", error: file.error})
			continue
		}
		finalized, finalErr := finalizeStreamedUpload(target, *file, conflictPolicy, s.cfg.Uploads.PreserveModTime)
		if finalErr == nil {
			finalized.addedAt = file.addedAt
			finalized.conflictPolicy = conflictPolicy
			file.path = ""
			saved = append(saved, finalized)
			continue
		}
		var fileScoped uploadFileError
		if errors.As(finalErr, &fileScoped) && errors.Is(finalErr, errUploadConflict) {
			_ = os.Remove(file.path)
			file.path = ""
			saved = append(saved, savedUpload{name: fileScoped.name, size: file.size, targetID: target.ID, status: "error", error: fileScoped.err.Error()})
			continue
		}
		return nil, saved, finalErr
	}
	if err := attachUploadItemTags(saved, itemTagValues); err != nil {
		return nil, saved, err
	}

	if len(saved) == 1 && saved[0].status == "error" {
		if saved[0].error == errUploadTooLarge.Error() {
			return parseUploadTags(tagValues), saved, nil
		}
		return nil, saved, uploadFileError{name: saved[0].name, err: errors.New(saved[0].error)}
	}
	return parseUploadTags(tagValues), saved, nil
}

func readUploadField(part *multipart.Part) (string, error) {
	limited := io.LimitReader(part, maxUploadFieldBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", multipartUploadError{message: fmt.Sprintf("failed to read multipart field: %v", err), err: err}
	}
	if len(data) > maxUploadFieldBytes {
		return "", multipartUploadError{message: "multipart field is too large", err: errors.New("multipart field exceeds limit")}
	}
	return string(data), nil
}

func (s *Server) streamUploadPart(target UploadTarget, originalName string, src io.Reader) (streamedUpload, error) {
	name, err := safeUploadName(originalName)
	if err != nil {
		return streamedUpload{name: originalName, status: "error", error: err.Error()}, nil
	}
	dst, err := os.CreateTemp(target.Path, ".gooru-upload-*")
	if err != nil {
		return streamedUpload{}, uploadFileError{name: name, err: errors.New("failed to create upload staging file")}
	}
	path := dst.Name()
	size, copyErr := s.persistUploadedFile(dst, src)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if errors.Is(copyErr, errUploadTooLarge) {
			return streamedUpload{name: name, size: size, status: "error", error: errUploadTooLarge.Error()}, nil
		}
		if copyErr != nil {
			return streamedUpload{}, uploadFileError{name: name, err: copyErr}
		}
		return streamedUpload{}, uploadFileError{name: name, err: errors.New("failed to write upload staging file")}
	}
	return streamedUpload{name: name, path: path, size: size}, nil
}

func finalizeStreamedUpload(target UploadTarget, file streamedUpload, conflictPolicy string, preserveModTime bool) (savedUpload, error) {
	path, skipped, replace, err := chooseUploadDestination(target.Path, file.name, conflictPolicy)
	if err != nil {
		return savedUpload{}, uploadFileError{name: file.name, err: err}
	}
	if skipped {
		_ = os.Remove(file.path)
		return savedUpload{name: file.name, path: path, destinationPath: path, size: file.size, targetID: target.ID, status: "skipped", sourceModTime: file.sourceModTime}, nil
	}
	stagedPath, err := moveStreamedUploadIntoDir(file.path, target.Path, file.name)
	if err != nil {
		return savedUpload{}, uploadFileError{name: file.name, err: err}
	}
	if replace {
		if err := applyUploadedSourceModTime(stagedPath, file.sourceModTime, preserveModTime); err != nil {
			_ = os.Remove(stagedPath)
			return savedUpload{}, uploadFileError{name: file.name, err: err}
		}
		return savedUpload{name: filepath.Base(path), path: stagedPath, destinationPath: path, size: file.size, targetID: target.ID, replace: true, sourceModTime: file.sourceModTime}, nil
	}
	if err := commitUploadDestination(stagedPath, path); err != nil {
		_ = os.Remove(stagedPath)
		return savedUpload{}, uploadFileError{name: file.name, err: err}
	}
	if err := applyUploadedSourceModTime(path, file.sourceModTime, preserveModTime); err != nil {
		_ = os.Remove(path)
		return savedUpload{}, uploadFileError{name: file.name, err: err}
	}
	return savedUpload{name: filepath.Base(path), path: path, destinationPath: path, size: file.size, targetID: target.ID, sourceModTime: file.sourceModTime}, nil
}

func parseUploadOrdinal(values []string, index int, fallback int) int {
	if index < len(values) {
		value, err := strconv.Atoi(strings.TrimSpace(values[index]))
		if err == nil && value >= 0 {
			return value
		}
	}
	return fallback
}

func resolveUploadAddedAt(strategy string, sourceModTime, queueTime, queueFirstTime, queueLastTime time.Time, queueIndex, queueTotal int) time.Time {
	if queueTime.IsZero() {
		queueTime = time.Now().UTC()
	}
	if queueIndex < 0 {
		queueIndex = 0
	}
	if queueTotal <= 0 {
		queueTotal = queueIndex + 1
	}
	if queueIndex >= queueTotal {
		queueIndex = queueTotal - 1
	}
	queueOffset := queueTotal - 1 - queueIndex
	if strategy == "modtime" && !sourceModTime.IsZero() {
		return sourceModTime.UTC()
	}
	if strategy == "reverse_queue" {
		queueOffset = queueIndex
	} else if !queueFirstTime.IsZero() && !queueLastTime.IsZero() && !queueLastTime.Before(queueFirstTime) && !queueTime.Before(queueFirstTime) && !queueTime.After(queueLastTime) {
		// Normal newest-first browsing should preserve queue admission order even when
		// one-file workers finish independently. Reflect the original queue time over
		// the shared batch envelope, then use the descending ordinal as the seconds tie-breaker.
		queueTime = queueFirstTime.Add(queueLastTime.Sub(queueTime))
	}
	// locations.added_at is second-granularity. The default queue strategy makes the
	// first admitted item newest; reverse_queue deliberately keeps later queue times newer.
	return queueTime.UTC().Truncate(time.Second).Add(time.Duration(queueOffset) * time.Second)
}

func parseUploadSourceModTime(value string) time.Time {
	millis, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || millis <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(millis).UTC()
}

func applyUploadedSourceModTime(path string, sourceModTime time.Time, preserve bool) error {
	if !preserve || sourceModTime.IsZero() {
		return nil
	}
	if err := os.Chtimes(path, sourceModTime, sourceModTime); err != nil {
		return errors.New("failed to preserve uploaded file modification time")
	}
	return nil
}

func chooseUploadDestination(dir, name, conflictPolicy string) (path string, skipped bool, replace bool, err error) {
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
		path = filepath.Join(dir, candidate)
		_, statErr := os.Stat(path)
		if statErr == nil {
			switch conflictPolicy {
			case "skip":
				return path, true, false, nil
			case "error":
				return "", false, false, errUploadConflict
			case "replace":
				return path, false, true, nil
			}
			continue
		}
		if errors.Is(statErr, os.ErrNotExist) {
			return path, false, false, nil
		}
		return "", false, false, errors.New("failed to inspect upload destination")
	}
	return "", false, false, errors.New("could not choose a non-conflicting upload filename")
}

func moveStreamedUploadIntoDir(srcPath, dir, name string) (string, error) {
	if filepath.Clean(filepath.Dir(srcPath)) == filepath.Clean(dir) {
		return srcPath, nil
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return "", errors.New("failed to read staged upload")
	}
	defer src.Close()
	dst, err := os.CreateTemp(dir, "."+filepath.Base(name)+".tmp-*")
	if err != nil {
		return "", errors.New("failed to create target staging file")
	}
	dstPath := dst.Name()
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(dstPath)
		return "", errors.New("failed to move staged upload into target")
	}
	if err := os.Remove(srcPath); err != nil {
		_ = os.Remove(dstPath)
		return "", errors.New("failed to finalize staged upload move")
	}
	return dstPath, nil
}

func removeStreamedUploads(files []streamedUpload) {
	for _, file := range files {
		if file.path != "" {
			_ = os.Remove(file.path)
		}
	}
}
