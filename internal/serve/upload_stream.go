package serve

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
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
	name   string
	path   string
	size   int64
	status string
	error  string
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

	var targetID, conflictRequested string
	var targetSeen, conflictSeen bool
	tagValues := make([]string, 0)
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
			}
			continue
		}

		if formName != "files" && formName != "file" {
			_ = part.Close()
			continue
		}
		fileCount++
		if fileCount > maxUploadFiles {
			_ = part.Close()
			return nil, saved, multipartUploadError{message: fmt.Sprintf("at most %d files are allowed per upload", maxUploadFiles), err: errors.New("too many upload files")}
		}
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
	target, err := s.uploadTarget(targetID)
	if err != nil {
		return nil, saved, multipartUploadError{code: "invalid_upload_target", message: err.Error(), err: err}
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
		finalized, finalErr := finalizeStreamedUpload(target, *file, conflictPolicy)
		if finalErr == nil {
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
	size, copyErr := copyUpload(dst, src, s.cfg.Uploads.MaxFileSizeBytes)
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

func finalizeStreamedUpload(target UploadTarget, file streamedUpload, conflictPolicy string) (savedUpload, error) {
	path, skipped, replace, err := chooseUploadDestination(target.Path, file.name, conflictPolicy)
	if err != nil {
		return savedUpload{}, uploadFileError{name: file.name, err: err}
	}
	if skipped {
		_ = os.Remove(file.path)
		return savedUpload{name: file.name, path: path, destinationPath: path, size: file.size, targetID: target.ID, status: "skipped"}, nil
	}
	stagedPath, err := moveStreamedUploadIntoDir(file.path, target.Path, file.name)
	if err != nil {
		return savedUpload{}, uploadFileError{name: file.name, err: err}
	}
	if replace {
		return savedUpload{name: filepath.Base(path), path: stagedPath, destinationPath: path, size: file.size, targetID: target.ID, replace: true}, nil
	}
	if err := commitUploadDestination(stagedPath, path); err != nil {
		_ = os.Remove(stagedPath)
		return savedUpload{}, uploadFileError{name: file.name, err: err}
	}
	return savedUpload{name: filepath.Base(path), path: path, destinationPath: path, size: file.size, targetID: target.ID}, nil
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
