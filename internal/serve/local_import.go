package serve

import (
    "context"
    "errors"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "strings"
)

// ImportLocalFiles uses the same multipart staging, target conflict handling and
// protected-storage importer as the HTTP upload endpoint. Source files are read
// only; failed imports remove only upload-owned destinations.
func (s *Server) ImportLocalFiles(ctx context.Context, targetID string, paths []string) (UploadImportResponse, error) {
    if strings.TrimSpace(targetID) == "" {
        return UploadImportResponse{}, errors.New("an explicit upload target is required")
    }
    if _, err := s.uploadTarget(targetID); err != nil {
        return UploadImportResponse{}, fmt.Errorf("select import target: %w", err)
    }
    importer, ok := s.library.(UploadLibrary)
    if !ok || importer == nil {
        return UploadImportResponse{}, errors.New("upload import service is not configured")
    }
    if len(paths) == 0 {
        return UploadImportResponse{}, errors.New("at least one source file is required")
    }
    reader, writer := io.Pipe()
    produced := make(chan error, 1)
    form := multipart.NewWriter(writer)
    go func() {
        err := streamLocalImport(form, targetID, paths)
        if err == nil {
            err = form.Close()
        }
        _ = writer.CloseWithError(err)
        produced <- err
    }()
    request, err := http.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/uploads", reader)
    if err != nil {
        _ = reader.CloseWithError(err)
        <-produced
        return UploadImportResponse{}, err
    }
    request.Header.Set("Content-Type", form.FormDataContentType())
    tags, saved, stageErr := s.stageMultipartUpload(request)
    _ = reader.CloseWithError(stageErr)
    producerErr := <-produced
    if producerErr != nil {
        return UploadImportResponse{}, producerErr
    }
    if stageErr != nil {
        return UploadImportResponse{}, fmt.Errorf("stage import: %w", stageErr)
    }
    defer releaseSavedUploadOwnership(saved)
    if err := validateUploadTags(tags, saved); err != nil {
        removeSavedUploads(saved)
        return UploadImportResponse{}, err
    }
    if len(saved) == 1 && saved[0].status == "error" {
        removeSavedUploads(saved)
        return UploadImportResponse{}, fmt.Errorf("import %q: %s", saved[0].name, saved[0].error)
    }
    response, err := importer.ImportUploadedFiles(ctx, stagedUploads(saved), tags)
    if err != nil {
        removeSavedUploads(saved)
        return UploadImportResponse{}, fmt.Errorf("import staged files: %w", err)
    }
    return response, nil
}

func streamLocalImport(form *multipart.Writer, targetID string, paths []string) error {
    if err := form.WriteField("target_id", targetID); err != nil {
        return err
    }
    for _, path := range paths {
        info, err := os.Lstat(path)
        if err != nil {
            return fmt.Errorf("inspect import source %q: %w", path, err)
        }
        if !info.Mode().IsRegular() {
            return fmt.Errorf("import source %q is not a regular file", path)
        }
        source, err := os.Open(path)
        if err != nil {
            return fmt.Errorf("open import source %q: %w", path, err)
        }
        part, err := form.CreateFormFile("files", filepath.Base(path))
        if err != nil {
            _ = source.Close()
            return err
        }
        copied, copyErr := io.Copy(part, source)
        closeErr := source.Close()
        if copyErr != nil {
            return fmt.Errorf("copy import source %q: %w", path, copyErr)
        }
        if closeErr != nil {
            return fmt.Errorf("close import source %q: %w", path, closeErr)
        }
        if copied != info.Size() {
            return fmt.Errorf("import source %q changed while copying", path)
        }
        if err := form.WriteField("source_modtime_ms", strconv.FormatInt(info.ModTime().UnixMilli(), 10)); err != nil {
            return err
        }
    }
    return nil
}
