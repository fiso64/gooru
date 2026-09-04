from pathlib import Path


def rep(path, old, new):
    p=Path(path); t=p.read_text()
    if new in t: return
    if old not in t: raise SystemExit(f'missing anchor {path}: {old[:80]!r}')
    p.write_text(t.replace(old,new,1))

rep('types/types.go', '''type LocationInfo struct {\n\tPath      string // The absolute path of the file\n\tHash      string\n\tSize      int64\n\tModTime   int64 // Unix time\n\tExtension string\n\tTagsCache string\n}''', '''type LocationInfo struct {\n\tPath      string // The absolute path of the file\n\tHash      string\n\tSize      int64\n\tModTime   int64 // Unix time\n\tAddedAt   int64 // Unix time; zero lets the database assign insertion time\n\tExtension string\n\tTagsCache string\n}''')

rep('internal/database/database.go', '''\tconst columns = 5 // content_hash, path, size_bytes, mod_time, extension''', '''\tconst columns = 6 // content_hash, path, size_bytes, mod_time, added_at, extension''')
rep('internal/database/database.go', '''\t\t\tplaceholders = append(placeholders, "('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?)")\n\t\t\targs = append(args, loc.Hash, loc.Path, loc.Size, loc.ModTime, loc.Extension)''', '''\t\t\tplaceholders = append(placeholders, "('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?, ?)")\n\t\t\targs = append(args, loc.Hash, loc.Path, loc.Size, loc.ModTime, loc.AddedAt, loc.Extension)''')
rep('internal/database/database.go', '''\t\tquery := `INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES ` +''', '''\t\tquery := `INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, added_at, extension) VALUES ` +''')

rep('internal/serve/config.go', '''type UploadTarget struct {\n\tID   string `yaml:"id"`\n\tName string `yaml:"name"`\n\tPath string `yaml:"path"`\n}''', '''type UploadTarget struct {\n\tID              string `yaml:"id"`\n\tName            string `yaml:"name"`\n\tPath            string `yaml:"path"`\n\tAddedAtStrategy string `yaml:"added_at_strategy"`\n}''')

rep('internal/serve/uploads.go', '''type StagedUpload struct {\n\tName          string\n\tPath          string\n\tAnalysisPath  string\n\tSize          int64\n\tTargetID      string\n\tStatus        string\n\tError         string\n\tSourceModTime time.Time\n}''', '''type StagedUpload struct {\n\tName          string\n\tPath          string\n\tAnalysisPath  string\n\tSize          int64\n\tTargetID      string\n\tStatus        string\n\tError         string\n\tSourceModTime time.Time\n\tAddedAt       time.Time\n}''')
rep('internal/serve/uploads.go', '''type UploadTargetDTO struct {\n\tID   string `json:"id"`\n\tName string `json:"name"`\n}''', '''type UploadTargetDTO struct {\n\tID              string `json:"id"`\n\tName            string `json:"name"`\n\tAddedAtStrategy string `json:"added_at_strategy"`\n}''')
rep('internal/serve/uploads.go', '''\t\titems = append(items, UploadTargetDTO{ID: target.ID, Name: target.Name})''', '''\t\tstrategy, err := uploadAddedAtStrategy("", target.AddedAtStrategy)\n\t\tif err != nil {\n\t\t\tcontinue\n\t\t}\n\t\titems = append(items, UploadTargetDTO{ID: target.ID, Name: target.Name, AddedAtStrategy: strategy})''')
rep('internal/serve/uploads.go', '''\tsourceModTime   time.Time\n}''', '''\tsourceModTime   time.Time\n\taddedAt         time.Time\n}''')
rep('internal/serve/uploads.go', '''func uploadConflictPolicy(requested string, fallback string) (string, error) {''', '''func uploadAddedAtStrategy(requested string, fallback string) (string, error) {\n\tstrategy := strings.TrimSpace(requested)\n\tif strategy == "" {\n\t\tstrategy = strings.TrimSpace(fallback)\n\t}\n\tif strategy == "" {\n\t\treturn "queue", nil\n\t}\n\tswitch strategy {\n\tcase "queue", "reverse_queue", "modtime":\n\t\treturn strategy, nil\n\tdefault:\n\t\tif strings.TrimSpace(requested) != "" {\n\t\t\treturn "", errors.New("added_at_strategy must be one of: queue, reverse_queue, modtime")\n\t\t}\n\t\treturn "", errors.New("configured upload target added_at_strategy is invalid")\n\t}\n}\n\nfunc uploadConflictPolicy(requested string, fallback string) (string, error) {''')
rep('internal/serve/uploads.go', '''\t\tout = append(out, StagedUpload{Name: file.name, Path: path, AnalysisPath: path, Size: file.size, TargetID: file.targetID, Status: file.status, Error: file.error, SourceModTime: file.sourceModTime})''', '''\t\tout = append(out, StagedUpload{Name: file.name, Path: path, AnalysisPath: path, Size: file.size, TargetID: file.targetID, Status: file.status, Error: file.error, SourceModTime: file.sourceModTime, AddedAt: file.addedAt})''')
rep('internal/serve/uploads.go', '''\t\timportLocations = append(importLocations, types.LocationInfo{\n\t\t\tPath:      file.Path,\n\t\t\tHash:      info.Hash,\n\t\t\tSize:      info.Size,\n\t\t\tModTime:   info.ModTime,\n\t\t\tExtension: filepath.Ext(file.Path),\n\t\t})''', '''\t\taddedAt := int64(0)\n\t\tif !file.AddedAt.IsZero() {\n\t\t\taddedAt = file.AddedAt.Unix()\n\t\t}\n\t\timportLocations = append(importLocations, types.LocationInfo{\n\t\t\tPath:      file.Path,\n\t\t\tHash:      info.Hash,\n\t\t\tSize:      info.Size,\n\t\t\tModTime:   info.ModTime,\n\t\t\tAddedAt:   addedAt,\n\t\t\tExtension: filepath.Ext(file.Path),\n\t\t})''')

rep('internal/serve/upload_stream.go', '''\tvar targetID, conflictRequested string\n\tvar targetSeen, conflictSeen bool\n\ttagValues := make([]string, 0)\n\tsourceModTimeValues := make([]string, 0)''', '''\tvar targetID, conflictRequested, addedAtStrategyRequested string\n\tvar targetSeen, conflictSeen, addedAtStrategySeen bool\n\ttagValues := make([]string, 0)\n\tsourceModTimeValues := make([]string, 0)\n\tqueueTimeValues := make([]string, 0)\n\tqueueIndexValues := make([]string, 0)\n\tqueueTotalValues := make([]string, 0)''')
rep('internal/serve/upload_stream.go', '''\t\t\tcase "source_modtime_ms":\n\t\t\t\tsourceModTimeValues = append(sourceModTimeValues, value)''', '''\t\t\tcase "source_modtime_ms":\n\t\t\t\tsourceModTimeValues = append(sourceModTimeValues, value)\n\t\t\tcase "added_at_strategy":\n\t\t\t\tif !addedAtStrategySeen {\n\t\t\t\t\taddedAtStrategyRequested, addedAtStrategySeen = value, true\n\t\t\t\t}\n\t\t\tcase "queue_time_ms":\n\t\t\t\tqueueTimeValues = append(queueTimeValues, value)\n\t\t\tcase "queue_index":\n\t\t\t\tqueueIndexValues = append(queueIndexValues, value)\n\t\t\tcase "queue_total":\n\t\t\t\tqueueTotalValues = append(queueTotalValues, value)''')
rep('internal/serve/upload_stream.go', '''\ttarget, err := s.uploadTarget(targetID)\n\tif err != nil {\n\t\treturn nil, saved, multipartUploadError{code: "invalid_upload_target", message: err.Error(), err: err}\n\t}\n\tconflictPolicy, err := uploadConflictPolicy(conflictRequested, s.cfg.Uploads.ConflictPolicy)''', '''\ttarget, err := s.uploadTarget(targetID)\n\tif err != nil {\n\t\treturn nil, saved, multipartUploadError{code: "invalid_upload_target", message: err.Error(), err: err}\n\t}\n\taddedAtStrategy, err := uploadAddedAtStrategy(addedAtStrategyRequested, target.AddedAtStrategy)\n\tif err != nil {\n\t\treturn nil, saved, multipartUploadError{message: err.Error(), err: err}\n\t}\n\tqueueFallback := time.Now().UTC()\n\tfor i := range streamed {\n\t\tqueueTime := queueFallback\n\t\tif i < len(queueTimeValues) {\n\t\t\tif parsed := parseUploadSourceModTime(queueTimeValues[i]); !parsed.IsZero() {\n\t\t\t\tqueueTime = parsed\n\t\t\t}\n\t\t}\n\t\tqueueIndex := parseUploadOrdinal(queueIndexValues, i, i)\n\t\tqueueTotal := parseUploadOrdinal(queueTotalValues, i, len(streamed))\n\t\tstreamed[i].sourceModTime = firstNonZeroTime(streamed[i].sourceModTime, time.Time{})\n\t\t// The resolved value is copied into saved/staged state before async job submission.\n\t\tstreamed[i].addedAt = resolveUploadAddedAt(addedAtStrategy, streamed[i].sourceModTime, queueTime, queueIndex, queueTotal)\n\t}\n\tconflictPolicy, err := uploadConflictPolicy(conflictRequested, s.cfg.Uploads.ConflictPolicy)''')
rep('internal/serve/upload_stream.go', '''type streamedUpload struct {\n\tname          string\n\tpath          string\n\tsize          int64\n\tstatus        string\n\terror         string\n\tsourceModTime time.Time\n}''', '''type streamedUpload struct {\n\tname          string\n\tpath          string\n\tsize          int64\n\tstatus        string\n\terror         string\n\tsourceModTime time.Time\n\taddedAt       time.Time\n}''')
rep('internal/serve/upload_stream.go', '''\t\t\tfinalized, finalErr := finalizeStreamedUpload(target, *file, conflictPolicy, s.cfg.Uploads.PreserveModTime)\n\t\tif finalErr == nil {''', '''\t\tfinalized, finalErr := finalizeStreamedUpload(target, *file, conflictPolicy, s.cfg.Uploads.PreserveModTime)\n\t\tif finalErr == nil {''')
# Ensure finalized results carry the already-resolved added-at metadata.
rep('internal/serve/upload_stream.go', '''\t\tif finalErr == nil {\n\t\t\tfile.path = ""\n\t\t\tsaved = append(saved, finalized)''', '''\t\tif finalErr == nil {\n\t\t\tfinalized.addedAt = file.addedAt\n\t\t\tfile.path = ""\n\t\t\tsaved = append(saved, finalized)''')
rep('internal/serve/upload_stream.go', '''func parseUploadSourceModTime(value string) time.Time {''', '''func parseUploadOrdinal(values []string, index int, fallback int) int {\n\tif index < len(values) {\n\t\tvalue, err := strconv.Atoi(strings.TrimSpace(values[index]))\n\t\tif err == nil && value >= 0 {\n\t\t\treturn value\n\t\t}\n\t}\n\treturn fallback\n}\n\nfunc firstNonZeroTime(value time.Time, fallback time.Time) time.Time {\n\tif value.IsZero() {\n\t\treturn fallback\n\t}\n\treturn value\n}\n\nfunc resolveUploadAddedAt(strategy string, sourceModTime, queueTime time.Time, queueIndex, queueTotal int) time.Time {\n\tif queueTime.IsZero() {\n\t\tqueueTime = time.Now().UTC()\n\t}\n\tif queueIndex < 0 {\n\t\tqueueIndex = 0\n\t}\n\tif queueTotal <= 0 {\n\t\tqueueTotal = queueIndex + 1\n\t}\n\tif queueIndex >= queueTotal {\n\t\tqueueIndex = queueTotal - 1\n\t}\n\tqueueOffset := queueIndex\n\tif strategy == "reverse_queue" {\n\t\tqueueOffset = queueTotal - 1 - queueIndex\n\t}\n\tif strategy == "modtime" && !sourceModTime.IsZero() {\n\t\treturn sourceModTime.UTC()\n\t}\n\t// locations.added_at is second-granularity; offset equal-time batch items by one\n\t// second so async worker completion order cannot affect stable queue ordering.\n\treturn queueTime.UTC().Truncate(time.Second).Add(time.Duration(queueOffset) * time.Second)\n}\n\nfunc parseUploadSourceModTime(value string) time.Time {''')

rep('docs/CONFIG.md', '''Each entry in `uploads.targets` supports:\n''', '''Each entry in `uploads.targets` supports `id`, `name`, `path`, and optional `added_at_strategy`. The strategy defaults to `queue` and accepts `queue`, `reverse_queue`, or `modtime`.\n\n''')
# OpenAPI additions are anchored on known upload target fields and multipart source metadata.
rep('docs/openapi.yaml', '''                source_modtime_ms:\n                  type: array''', '''                added_at_strategy:\n                  type: string\n                  enum: [queue, reverse_queue, modtime]\n                  description: Override the selected upload target's added-at strategy for this submission.\n                queue_time_ms:\n                  type: array\n                  items: { type: integer, format: int64 }\n                  description: Client-captured queue timestamps in Unix milliseconds, one per file.\n                queue_index:\n                  type: array\n                  items: { type: integer, minimum: 0 }\n                  description: Stable queue indexes, one per file.\n                queue_total:\n                  type: array\n                  items: { type: integer, minimum: 1 }\n                  description: Queue batch size, one per file.\n                source_modtime_ms:\n                  type: array''')
# Add server boundary tests.
Path('internal/serve/upload_added_at_strategy_test.go').write_text(r'''package serve

import (
    "bytes"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "strconv"
    "testing"
    "time"
)

func TestUploadAddedAtStrategyUsesTargetDefaultAndRequestOverride(t *testing.T) {
    base := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
    source := time.Date(2020, 7, 8, 9, 10, 11, 0, time.UTC)
    for _, tc := range []struct{name, targetStrategy, requestStrategy string; want time.Time}{
        {"target reverse", "reverse_queue", "", base.Add(2*time.Second)},
        {"request modtime", "reverse_queue", "modtime", source},
    } {
        t.Run(tc.name, func(t *testing.T) {
            dir := t.TempDir()
            library := &recordingUploadLibrary{}
            server := newUploadTestServer(t, dir, true, library)
            server.cfg.Uploads.Targets[0].AddedAtStrategy = tc.targetStrategy
            rec := httptest.NewRecorder()
            req := uploadAddedAtRequest(t, base, source, 0, 3, tc.requestStrategy)
            server.Handler().ServeHTTP(rec, req)
            if rec.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()) }
            if len(library.files) != 1 || !library.files[0].AddedAt.Equal(tc.want) {
                t.Fatalf("added_at=%v want=%v files=%+v", library.files[0].AddedAt, tc.want, library.files)
            }
        })
    }
}

func TestUploadAddedAtStrategyModtimeFallsBackToQueue(t *testing.T) {
    base := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
    got := resolveUploadAddedAt("modtime", time.Time{}, base, 1, 3)
    if want := base.Add(time.Second); !got.Equal(want) { t.Fatalf("got=%v want=%v", got, want) }
}

func TestUploadAddedAtStrategyRejectsInvalidOverride(t *testing.T) {
    dir := t.TempDir(); library := &recordingUploadLibrary{}; server := newUploadTestServer(t, dir, true, library)
    rec := httptest.NewRecorder(); server.Handler().ServeHTTP(rec, uploadAddedAtRequest(t, time.Now(), time.Time{}, 0, 1, "random"))
    if rec.Code != http.StatusBadRequest { t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()) }
}

func uploadAddedAtRequest(t *testing.T, queue, source time.Time, index, total int, strategy string) *http.Request {
    t.Helper(); var body bytes.Buffer; w := multipart.NewWriter(&body)
    part, err := w.CreateFormFile("files", "a.txt"); if err != nil { t.Fatal(err) }; _, _ = part.Write([]byte("hello"))
    if !source.IsZero() { _ = w.WriteField("source_modtime_ms", strconv.FormatInt(source.UnixMilli(),10)) }
    _ = w.WriteField("queue_time_ms", strconv.FormatInt(queue.UnixMilli(),10)); _ = w.WriteField("queue_index", strconv.Itoa(index)); _ = w.WriteField("queue_total", strconv.Itoa(total))
    if strategy != "" { _ = w.WriteField("added_at_strategy", strategy) }
    if err := w.Close(); err != nil { t.Fatal(err) }
    req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body); req.Header.Set("Content-Type", w.FormDataContentType()); return req
}
''')
