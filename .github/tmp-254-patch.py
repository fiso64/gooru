from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if new in text:
        return
    if old not in text:
        raise SystemExit(f"missing patch anchor in {path}: {old[:80]!r}")
    p.write_text(text.replace(old, new, 1))


replace(
    "internal/serve/config.go",
    '''type UploadsConfig struct {\n\tEnabled          bool           `yaml:"enabled"`\n\tTargets          []UploadTarget `yaml:"targets"`\n\tMaxFileSizeBytes int64          `yaml:"max_file_size_bytes"`\n\tConflictPolicy   string         `yaml:"conflict_policy"`\n}''',
    '''type UploadsConfig struct {\n\tEnabled          bool           `yaml:"enabled"`\n\tPreserveModTime  bool           `yaml:"preserve_modtime"`\n\tTargets          []UploadTarget `yaml:"targets"`\n\tMaxFileSizeBytes int64          `yaml:"max_file_size_bytes"`\n\tConflictPolicy   string         `yaml:"conflict_policy"`\n}''',
)
replace(
    "internal/serve/config.go",
    '''\t\tUploads: UploadsConfig{Enabled: false, ConflictPolicy: "rename"},''',
    '''\t\tUploads: UploadsConfig{Enabled: false, PreserveModTime: true, ConflictPolicy: "rename"},''',
)

replace(
    "internal/serve/uploads.go",
    '''\t"strings"\n''',
    '''\t"strings"\n\t"time"\n''',
)
replace(
    "internal/serve/uploads.go",
    '''type StagedUpload struct {\n\tName         string\n\tPath         string\n\tAnalysisPath string\n\tSize         int64\n\tTargetID     string\n\tStatus       string\n\tError        string\n}''',
    '''type StagedUpload struct {\n\tName          string\n\tPath          string\n\tAnalysisPath  string\n\tSize          int64\n\tSourceModTime time.Time\n\tTargetID      string\n\tStatus        string\n\tError         string\n}''',
)
replace(
    "internal/serve/uploads.go",
    '''\treplace         bool\n}''',
    '''\treplace         bool\n\tsourceModTime   time.Time\n}''',
)
replace(
    "internal/serve/uploads.go",
    '''out = append(out, StagedUpload{Name: file.name, Path: path, AnalysisPath: path, Size: file.size, TargetID: file.targetID, Status: file.status, Error: file.error})''',
    '''out = append(out, StagedUpload{Name: file.name, Path: path, AnalysisPath: path, Size: file.size, SourceModTime: file.sourceModTime, TargetID: file.targetID, Status: file.status, Error: file.error})''',
)

replace(
    "internal/serve/upload_stream.go",
    '''\t"path/filepath"\n)''',
    '''\t"path/filepath"\n\t"strconv"\n\t"time"\n)''',
)
replace(
    "internal/serve/upload_stream.go",
    '''\tvar targetID, conflictRequested string\n\tvar targetSeen, conflictSeen bool\n\ttagValues := make([]string, 0)''',
    '''\tvar targetID, conflictRequested string\n\tvar targetSeen, conflictSeen bool\n\ttagValues := make([]string, 0)\n\tsourceModTimeValues := make([]string, 0)''',
)
replace(
    "internal/serve/upload_stream.go",
    '''\t\t\tcase "tags":\n\t\t\t\ttagValues = append(tagValues, value)\n\t\t\t}''',
    '''\t\t\tcase "tags":\n\t\t\t\ttagValues = append(tagValues, value)\n\t\t\tcase "source_mod_time_ms":\n\t\t\t\tsourceModTimeValues = append(sourceModTimeValues, value)\n\t\t\t}''',
)
replace(
    "internal/serve/upload_stream.go",
    '''\tif err := os.MkdirAll(target.Path, 0700); err != nil {\n\t\treturn nil, saved, multipartUploadError{message: "failed to prepare upload directory", err: err}\n\t}\n\n\tfor i := range streamed {''',
    '''\tif err := os.MkdirAll(target.Path, 0700); err != nil {\n\t\treturn nil, saved, multipartUploadError{message: "failed to prepare upload directory", err: err}\n\t}\n\tsourceModTimes := uploadSourceModTimes(sourceModTimeValues, len(streamed))\n\n\tfor i := range streamed {''',
)
replace(
    "internal/serve/upload_stream.go",
    '''\t\tfile := &streamed[i]\n\t\tif file.status == "error" {\n\t\t\tsaved = append(saved, savedUpload{name: file.name, size: file.size, targetID: target.ID, status: "error", error: file.error})\n\t\t\tcontinue\n\t\t}\n\t\tfinalized, finalErr := finalizeStreamedUpload(target, *file, conflictPolicy)\n\t\tif finalErr == nil {\n\t\t\tfile.path = ""\n\t\t\tsaved = append(saved, finalized)''',
    '''\t\tfile := &streamed[i]\n\t\tsourceModTime := sourceModTimes[i]\n\t\tif file.status == "error" {\n\t\t\tsaved = append(saved, savedUpload{name: file.name, size: file.size, sourceModTime: sourceModTime, targetID: target.ID, status: "error", error: file.error})\n\t\t\tcontinue\n\t\t}\n\t\tfinalized, finalErr := finalizeStreamedUpload(target, *file, conflictPolicy)\n\t\tif finalErr == nil {\n\t\t\tfinalized.sourceModTime = sourceModTime\n\t\t\tif s.cfg.Uploads.PreserveModTime && finalized.status != "skipped" && !sourceModTime.IsZero() {\n\t\t\t\tif err := os.Chtimes(finalized.path, sourceModTime, sourceModTime); err != nil {\n\t\t\t\t\treturn nil, saved, uploadFileError{name: finalized.name, err: errors.New("failed to preserve uploaded file modification time")}\n\t\t\t\t}\n\t\t\t}\n\t\t\tfile.path = ""\n\t\t\tsaved = append(saved, finalized)''',
)
replace(
    "internal/serve/upload_stream.go",
    '''func readUploadField(part *multipart.Part) (string, error) {''',
    '''func uploadSourceModTimes(values []string, count int) []time.Time {\n\tout := make([]time.Time, count)\n\tfor i := 0; i < count && i < len(values); i++ {\n\t\tmillis, err := strconv.ParseInt(values[i], 10, 64)\n\t\tif err != nil || millis <= 0 {\n\t\t\tcontinue\n\t\t}\n\t\tout[i] = time.Unix(millis/1000, (millis%1000)*int64(time.Millisecond)).UTC()\n\t}\n\treturn out\n}\n\nfunc readUploadField(part *multipart.Part) (string, error) {''',
)

replace(
    "frontend/src/lib/api/client.ts",
    '''    const form = new FormData();\n    for (const file of files) form.append('files', file, file.name);''',
    '''    const form = new FormData();\n    for (const file of files) {\n      form.append('files', file, file.name);\n      const sourceModTime = Number.isFinite(file.lastModified) && file.lastModified > 0 ? Math.trunc(file.lastModified) : '';\n      form.append('source_mod_time_ms', String(sourceModTime));\n    }''',
)

replace(
    "docs/openapi.yaml",
    '''                target_id:\n                  type: string\n                  description: Configured upload target ID. Defaults to the first configured target.''',
    '''                source_mod_time_ms:\n                  type: array\n                  items:\n                    type: integer\n                    format: int64\n                  description: Source file modification timestamps in Unix milliseconds, one entry per files item in the same order.\n                target_id:\n                  type: string\n                  description: Configured upload target ID. Defaults to the first configured target.''',
)

replace(
    "docs/CONFIG.md",
    '''| `uploads.enabled` | `false` | Enable browser/API uploads. Enabling uploads requires at least one valid target. |\n| `uploads.targets` | empty list | Allowed upload destinations. Each target has `id`, `name`, and `path`. |''',
    '''| `uploads.enabled` | `false` | Enable browser/API uploads. Enabling uploads requires at least one valid target. |\n| `uploads.preserve_modtime` | `true` | Preserve the source modification timestamp reported by browser uploads on the managed destination file. Source timestamps are still carried with the upload when disabled so upload ordering policies can use them independently. |\n| `uploads.targets` | empty list | Allowed upload destinations. Each target has `id`, `name`, and `path`. |''',
)
replace(
    "docs/CONFIG.md",
    '''uploads:\n  enabled: true\n  targets:''',
    '''uploads:\n  enabled: true\n  preserve_modtime: true\n  targets:''',
)

Path("internal/serve/upload_modtime_test.go").write_text(r'''package serve

import (
    "bytes"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strconv"
    "testing"
    "time"
)

func TestUploadPreservesReportedSourceModTimeAndCarriesMetadata(t *testing.T) {
    dir := t.TempDir()
    library := &recordingUploadLibrary{}
    server := newUploadTestServer(t, dir, true, library)
    server.cfg.Uploads.PreserveModTime = true
    sourceModTime := time.Date(2001, time.February, 3, 4, 5, 6, 789000000, time.UTC)

    rec := httptest.NewRecorder()
    server.Handler().ServeHTTP(rec, uploadWithSourceModTimeRequest(t, "photo.jpg", []byte("payload"), sourceModTime))
    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
    }

    info, err := os.Stat(filepath.Join(dir, "photo.jpg"))
    if err != nil {
        t.Fatal(err)
    }
    if !info.ModTime().Equal(sourceModTime) {
        t.Fatalf("destination modtime=%s want=%s", info.ModTime(), sourceModTime)
    }
    if len(library.files) != 1 || !library.files[0].SourceModTime.Equal(sourceModTime) {
        t.Fatalf("source modtime metadata was not carried to importer: %+v", library.files)
    }
}

func TestUploadCarriesSourceModTimeWithoutChangingDestinationWhenDisabled(t *testing.T) {
    dir := t.TempDir()
    library := &recordingUploadLibrary{}
    server := newUploadTestServer(t, dir, true, library)
    server.cfg.Uploads.PreserveModTime = false
    sourceModTime := time.Date(2001, time.February, 3, 4, 5, 6, 789000000, time.UTC)

    rec := httptest.NewRecorder()
    server.Handler().ServeHTTP(rec, uploadWithSourceModTimeRequest(t, "photo.jpg", []byte("payload"), sourceModTime))
    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
    }

    info, err := os.Stat(filepath.Join(dir, "photo.jpg"))
    if err != nil {
        t.Fatal(err)
    }
    if info.ModTime().Equal(sourceModTime) {
        t.Fatalf("destination unexpectedly inherited source modtime while preserve_modtime=false")
    }
    if len(library.files) != 1 || !library.files[0].SourceModTime.Equal(sourceModTime) {
        t.Fatalf("source modtime metadata was not retained for later added_at policy: %+v", library.files)
    }
}

func uploadWithSourceModTimeRequest(t *testing.T, name string, data []byte, sourceModTime time.Time) *http.Request {
    t.Helper()
    var body bytes.Buffer
    writer := multipart.NewWriter(&body)
    part, err := writer.CreateFormFile("file", name)
    if err != nil {
        t.Fatal(err)
    }
    if _, err := part.Write(data); err != nil {
        t.Fatal(err)
    }
    if err := writer.WriteField("source_mod_time_ms", strconv.FormatInt(sourceModTime.UnixMilli(), 10)); err != nil {
        t.Fatal(err)
    }
    if err := writer.Close(); err != nil {
        t.Fatal(err)
    }
    req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body)
    req.Header.Set("Content-Type", writer.FormDataContentType())
    return req
}
''')

replace(
    "internal/serve/config_test.go",
    '''\tif cfg.Uploads.ConflictPolicy != "rename" {\n\t\tt.Fatalf("unexpected upload conflict policy default %q", cfg.Uploads.ConflictPolicy)\n\t}\n}''',
    '''\tif cfg.Uploads.ConflictPolicy != "rename" {\n\t\tt.Fatalf("unexpected upload conflict policy default %q", cfg.Uploads.ConflictPolicy)\n\t}\n\tif !cfg.Uploads.PreserveModTime {\n\t\tt.Fatal("uploads.preserve_modtime should default true")\n\t}\n}''',
)
