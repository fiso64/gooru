from pathlib import Path


def replace(path, old, new, count=1):
    p = Path(path)
    s = p.read_text()
    actual = s.count(old)
    if actual != count:
        raise SystemExit(f"{path}: expected {count} occurrences, found {actual}: {old[:80]!r}")
    p.write_text(s.replace(old, new, count))


replace('internal/serve/config.go', 'Uploads: UploadsConfig{Enabled: false, ConflictPolicy: "skip", PreserveModTime: true}', 'Uploads: UploadsConfig{Enabled: false, ConflictPolicy: "rename", PreserveModTime: true}')
replace('internal/serve/uploads.go', 'import (\n\t"context"', 'import (\n\t"context"\n\t"database/sql"')
replace('internal/serve/uploads.go', '\tAddedAt       time.Time\n}', '\tAddedAt       time.Time\n\tConflictPolicy string\n}')
replace('internal/serve/uploads.go', '\tif fallback == "" {\n\t\treturn "skip", nil\n\t}', '\tif fallback == "" {\n\t\treturn "rename", nil\n\t}')
replace('internal/serve/uploads.go', 'response, err := importer.ImportUploadedFiles(ctx, stagedUploads(saved), tags)', 'response, err := importer.ImportUploadedFiles(ctx, stagedUploads(saved, conflictPolicy), tags)')
replace('internal/serve/uploads.go', 'func stagedUploads(files []savedUpload) []StagedUpload {', 'func stagedUploads(files []savedUpload, conflictPolicy string) []StagedUpload {')
replace('internal/serve/uploads.go', 'SourceModTime: file.sourceModTime, AddedAt: file.addedAt})', 'SourceModTime: file.sourceModTime, AddedAt: file.addedAt, ConflictPolicy: conflictPolicy})')
needle = '''\t\tif exists || status == types.StatusUntrackedContent || status == types.StatusOK {\n\t\t\tdto.Status = "duplicate_existing"\n\t\t\tremoveRejectedStagedUpload(file)\n\t\t\tresponse.Files = append(response.Files, dto)\n\t\t\tcontinue\n\t\t}\n\t\tstoragePath := ""\n'''
insert = '''\t\tif exists || status == types.StatusUntrackedContent || status == types.StatusOK {\n\t\t\tdto.Status = "duplicate_existing"\n\t\t\tremoveRejectedStagedUpload(file)\n\t\t\tresponse.Files = append(response.Files, dto)\n\t\t\tcontinue\n\t\t}\n\t\tif file.ConflictPolicy == "rename" && l.encryption.Enabled && IsManagedUploadPath(l.managedTargets, file.Path) {\n\t\t\tresolvedPath, err := l.resolveProtectedUploadRename(file.Path)\n\t\t\tif err != nil {\n\t\t\t\tdto.Status = "error"\n\t\t\t\tdto.Error = err.Error()\n\t\t\t\tremoveRejectedStagedUpload(file)\n\t\t\t\tresponse.Files = append(response.Files, dto)\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tfile.Path = resolvedPath\n\t\t\tdto.Name = filepath.Base(resolvedPath)\n\t\t}\n\t\tstoragePath := ""\n'''
replace('internal/serve/uploads.go', needle, insert)
marker = 'func removeRejectedStagedUpload(file StagedUpload) {\n'
helper = '''func (l *GooruLibrary) resolveProtectedUploadRename(path string) (string, error) {\n\text := filepath.Ext(path)\n\tbase := strings.TrimSuffix(filepath.Base(path), ext)\n\tif base == "" {\n\t\tbase = "upload"\n\t}\n\tdir := filepath.Dir(path)\n\tfor i := 0; i < 10_000; i++ {\n\t\tname := filepath.Base(path)\n\t\tif i > 0 {\n\t\t\tname = fmt.Sprintf("%s-%d%s", base, i, ext)\n\t\t}\n\t\tcandidate := filepath.Join(dir, name)\n\t\t_, err := l.client.GetFileInfoByPath(candidate)\n\t\tif err == nil {\n\t\t\tcontinue\n\t\t}\n\t\tif !errors.Is(err, sql.ErrNoRows) {\n\t\t\treturn "", fmt.Errorf("check protected upload name: %w", err)\n\t\t}\n\t\tif candidate == path {\n\t\t\treturn path, nil\n\t\t}\n\t\tif err := commitUploadDestination(path, candidate); err != nil {\n\t\t\tif errors.Is(err, errUploadConflict) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\treturn "", err\n\t\t}\n\t\treturn candidate, nil\n\t}\n\treturn "", errors.New("could not choose a non-conflicting protected upload filename")\n}\n\n'''
replace('internal/serve/uploads.go', marker, helper + marker)

replace('internal/serve/config_upload_policy_test.go', 'func TestDefaultUploadConflictPolicyIsSkip(t *testing.T) {', 'func TestDefaultUploadConflictPolicyIsRename(t *testing.T) {')
replace('internal/serve/config_upload_policy_test.go', 'cfg.Uploads.ConflictPolicy != "skip"', 'cfg.Uploads.ConflictPolicy != "rename"')
replace('internal/serve/config_upload_policy_test.go', 'default upload conflict policy = %q, want skip', 'default upload conflict policy = %q, want rename')
replace('internal/serve/config_upload_policy_test.go', 'policy != "skip"', 'policy != "rename"')
replace('internal/serve/config_upload_policy_test.go', 'empty upload conflict policy = %q, want skip', 'empty upload conflict policy = %q, want rename')

p = Path('internal/serve/uploads_test.go')
s = p.read_text()
start = s.index('func TestUploadOmittedConflictPolicySkipsExistingName')
end = s.index('\nfunc TestUploadRequestedConflictPolicySkipReturnsSkippedFile', start)
replacement = '''func TestUploadOmittedConflictPolicyRenamesExistingName(t *testing.T) {\n\tdir := t.TempDir()\n\tif err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("existing"), 0600); err != nil {\n\t\tt.Fatalf("write existing file: %v", err)\n\t}\n\tlibrary := &recordingUploadLibrary{}\n\tserver := newUploadTestServer(t, dir, true, library)\n\trec := httptest.NewRecorder()\n\n\tserver.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"a.txt": "uploaded"}, nil))\n\n\tif rec.Code != http.StatusOK {\n\t\tt.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())\n\t}\n\tif got := string(mustReadFile(t, filepath.Join(dir, "a.txt"))); got != "existing" {\n\t\tt.Fatalf("existing file was overwritten: %q", got)\n\t}\n\tif got := string(mustReadFile(t, filepath.Join(dir, "a-1.txt"))); got != "uploaded" {\n\t\tt.Fatalf("renamed upload contents = %q", got)\n\t}\n\tif len(library.paths) != 1 || filepath.Base(library.paths[0]) != "a-1.txt" {\n\t\tt.Fatalf("unexpected imported paths: %+v", library.paths)\n\t}\n}\n'''
p.write_text(s[:start] + replacement + s[end:])

replace('frontend/src/lib/state/uploadWorkflow.svelte.ts', "let conflictPolicy = $state('skip');", "let conflictPolicy = $state('rename');")
replace('frontend/src/lib/state/uploadWorkflow.svelte.ts', "conflictPolicy = 'skip';", "conflictPolicy = 'rename';")
replace('frontend/tests/upload-conflict-default.spec.ts', "test('WebUI hides conflict policy choices and uploads with skip'", "test('WebUI hides conflict policy choices and uploads with rename'")
replace('frontend/tests/upload-conflict-default.spec.ts', '/name="conflict_policy"\\r?\\n\\r?\\nskip/', '/name="conflict_policy"\\r?\\n\\r?\\nrename/')
replace('frontend/tests/upload-conflict-default.spec.ts', '/name="conflict_policy"\\r?\\n\\r?\\n(rename|replace)/', '/name="conflict_policy"\\r?\\n\\r?\\n(skip|replace)/')

p = Path('internal/serve/upload_encryption_e2e_test.go')
s = p.read_text()
if '\t"encoding/json"\n' not in s:
    s = s.replace('\t"image/color"\n', '\t"encoding/json"\n\t"image/color"\n', 1)
test = r'''

func TestEncryptedUploadDefaultConflictRenamesLogicalPathAfterHashDedup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	uploadDir := filepath.Join(dir, "uploads")
	cfg := DefaultConfig(filepath.Join(dir, "serve.db"))
	cfg.Auth.Enabled = false
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x33}, securekey.Size)
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: uploadDir}}
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))

	upload := func(data []byte) UploadImportResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, uploadBinaryRequestWithSourceModTime(t, "same.png", data, time.Time{}))
		if rec.Code != http.StatusOK {
			t.Fatalf("upload status = %d: %s", rec.Code, rec.Body.String())
		}
		var response UploadImportResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(response.Files) != 1 {
			t.Fatalf("upload response = %+v", response.Files)
		}
		return response
	}

	firstBytes := tinyPNG(t, 2, 2, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	secondBytes := tinyPNG(t, 3, 2, color.RGBA{R: 40, G: 50, B: 60, A: 255})
	first := upload(firstBytes)
	if first.Files[0].Status != "imported" || first.Files[0].Name != "same.png" {
		t.Fatalf("first upload = %+v", first.Files[0])
	}
	duplicate := upload(firstBytes)
	if duplicate.Files[0].Status != "duplicate_existing" {
		t.Fatalf("exact duplicate = %+v", duplicate.Files[0])
	}
	renamed := upload(secondBytes)
	if renamed.Files[0].Status != "imported" || renamed.Files[0].Name != "same-1.png" {
		t.Fatalf("same-name different-content upload = %+v", renamed.Files[0])
	}

	for _, name := range []string{"same.png", "same-1.png"} {
		logicalPath := filepath.Join(uploadDir, name)
		if _, err := os.Stat(logicalPath); !os.IsNotExist(err) {
			t.Fatalf("protected logical path %q leaked on disk: %v", logicalPath, err)
		}
		file, err := client.GetFileInfoByPath(logicalPath)
		if err != nil {
			t.Fatalf("logical path %q not tracked: %v", logicalPath, err)
		}
		resolved, err := client.ResolveManagedStorage(file)
		if err != nil {
			t.Fatalf("resolve storage for %q: %v", logicalPath, err)
		}
		if resolved.StoragePath == "" || filepath.Ext(resolved.StoragePath) != "" {
			t.Fatalf("protected storage for %q is not opaque: %q", logicalPath, resolved.StoragePath)
		}
	}
}
'''
if 'func TestEncryptedUploadDefaultConflictRenamesLogicalPathAfterHashDedup' in s:
    raise SystemExit('protected regression already present')
p.write_text(s + test)
