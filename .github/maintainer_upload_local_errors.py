from pathlib import Path

p = Path("internal/serve/uploads.go")
s = p.read_text()
replacements = [
    (
        'var errUploadTooLarge = errors.New("uploaded file exceeds max_file_size_bytes")\n',
        'var (\n\terrUploadTooLarge = errors.New("uploaded file exceeds max_file_size_bytes")\n\terrUploadConflict = errors.New("uploaded filename conflicts with an existing file")\n)\n',
    ),
    (
        '\t\tname, err := safeUploadName(header.Filename)\n\t\tif err != nil {\n\t\t\treturn nil, uploadFileError{name: header.Filename, err: err}\n\t\t}\n',
        '\t\tname, err := safeUploadName(header.Filename)\n\t\tif err != nil {\n\t\t\tif len(files) > 1 {\n\t\t\t\tsaved = append(saved, savedUpload{name: header.Filename, size: header.Size, targetID: target.ID, status: "error", error: err.Error()})\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\treturn nil, uploadFileError{name: header.Filename, err: err}\n\t\t}\n',
    ),
    (
        '\t\tsrc, err := header.Open()\n\t\tif err != nil {\n\t\t\tremoveSavedUploads(saved)\n\t\t\treturn nil, uploadFileError{name: name, err: fmt.Errorf("failed to read uploaded file")}\n\t\t}\n',
        '\t\tsrc, err := header.Open()\n\t\tif err != nil {\n\t\t\tfileErr := fmt.Errorf("failed to read uploaded file")\n\t\t\tif len(files) > 1 {\n\t\t\t\tsaved = append(saved, savedUpload{name: name, size: header.Size, targetID: target.ID, status: "error", error: fileErr.Error()})\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tremoveSavedUploads(saved)\n\t\t\treturn nil, uploadFileError{name: name, err: fileErr}\n\t\t}\n',
    ),
    (
        '\t\tdst, path, tmpPath, skipped, err := createUploadDestination(target.Path, name, conflictPolicy)\n\t\tif err != nil {\n\t\t\t_ = src.Close()\n\t\t\tremoveSavedUploads(saved)\n\t\t\treturn nil, uploadFileError{name: name, err: err}\n\t\t}\n',
        '\t\tdst, path, tmpPath, skipped, err := createUploadDestination(target.Path, name, conflictPolicy)\n\t\tif err != nil {\n\t\t\t_ = src.Close()\n\t\t\tif len(files) > 1 && errors.Is(err, errUploadConflict) {\n\t\t\t\tsaved = append(saved, savedUpload{name: name, size: header.Size, targetID: target.ID, status: "error", error: err.Error()})\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tremoveSavedUploads(saved)\n\t\t\treturn nil, uploadFileError{name: name, err: err}\n\t\t}\n',
    ),
    (
        '\t\tif err := commitUploadDestination(tmpPath, path); err != nil {\n\t\t\t_ = os.Remove(tmpPath)\n\t\t\tremoveSavedUploads(saved)\n\t\t\treturn nil, uploadFileError{name: name, err: err}\n\t\t}\n',
        '\t\tif err := commitUploadDestination(tmpPath, path); err != nil {\n\t\t\t_ = os.Remove(tmpPath)\n\t\t\tif len(files) > 1 && errors.Is(err, errUploadConflict) {\n\t\t\t\tsaved = append(saved, savedUpload{name: name, size: header.Size, targetID: target.ID, status: "error", error: err.Error()})\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tremoveSavedUploads(saved)\n\t\t\treturn nil, uploadFileError{name: name, err: err}\n\t\t}\n',
    ),
    (
        '\t\t\tcase "error":\n\t\t\t\treturn nil, "", "", false, errors.New("uploaded filename conflicts with an existing file")\n',
        '\t\t\tcase "error":\n\t\t\t\treturn nil, "", "", false, errUploadConflict\n',
    ),
    (
        '\t\tif errors.Is(err, os.ErrExist) {\n\t\t\treturn errors.New("uploaded filename conflicts with an existing file")\n\t\t}\n',
        '\t\tif errors.Is(err, os.ErrExist) {\n\t\t\treturn errUploadConflict\n\t\t}\n',
    ),
]
for old, new in replacements:
    if old not in s:
        raise SystemExit(f"missing uploads.go replacement:\n{old}")
    s = s.replace(old, new, 1)
p.write_text(s)

p = Path("internal/serve/uploads_test.go")
s = p.read_text()
anchor = "func TestUploadRequestedConflictPolicySkipReturnsSkippedFile(t *testing.T) {\n"
test = '''func TestUploadConflictPolicyErrorIsolatesExistingNameWithinBatch(t *testing.T) {
\tdir := t.TempDir()
\tif err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("existing"), 0600); err != nil {
\t\tt.Fatalf("write existing file: %v", err)
\t}
\tserver := newUploadTestServer(t, dir, true, &recordingUploadLibrary{})
\tserver.cfg.Uploads.ConflictPolicy = "error"
\trec := httptest.NewRecorder()

\tserver.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{
\t\t"a.txt": "valid",
\t\t"b.txt": "replacement",
\t}, nil))

\tif rec.Code != http.StatusOK {
\t\tt.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
\t}
\tvar response UploadImportResponse
\tif err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
\t\tt.Fatalf("decode response: %v", err)
\t}
\tbyName := make(map[string]UploadedFileDTO, len(response.Files))
\tfor _, file := range response.Files {
\t\tbyName[file.Name] = file
\t}
\tif byName["a.txt"].Status != "imported" {
\t\tt.Fatalf("valid sibling should import, got %+v", byName)
\t}
\tif byName["b.txt"].Status != "error" || byName["b.txt"].Error != errUploadConflict.Error() {
\t\tt.Fatalf("conflicting member should fail independently, got %+v", byName)
\t}
\tif got := string(mustReadFile(t, filepath.Join(dir, "a.txt"))); got != "valid" {
\t\tt.Fatalf("valid sibling was not retained: %q", got)
\t}
\tif got := string(mustReadFile(t, filepath.Join(dir, "b.txt"))); got != "existing" {
\t\tt.Fatalf("existing file was changed: %q", got)
\t}
}

'''
if anchor not in s:
    raise SystemExit("missing uploads_test.go anchor")
s = s.replace(anchor, test + anchor, 1)
p.write_text(s)
