from pathlib import Path


def replace(path, old, new, count=1):
    p = Path(path)
    text = p.read_text()
    actual = text.count(old)
    if actual != count:
        raise SystemExit(f"{path}: expected {count} occurrences, found {actual}: {old!r}")
    p.write_text(text.replace(old, new))

# Core location model + DB persistence.
replace(
    "types/types.go",
    "\tModTime   int64 // Unix time\n\tExtension string",
    "\tModTime   int64 // Unix time\n\tAddedAt   int64 // Unix time; zero keeps the existing/default database value\n\tExtension string",
)
replace(
    "internal/database/database.go",
    "\tconst columns = 5 // content_hash, path, size_bytes, mod_time, extension",
    "\tconst columns = 9 // content_hash, path, size_bytes, mod_time, extension, plus explicit added_at insert/update guards",
)
replace(
    "internal/database/database.go",
    "\t\tfor _, loc := range batch {\n\t\t\tplaceholders = append(placeholders, \"('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?)\")\n\t\t\targs = append(args, loc.Hash, loc.Path, loc.Size, loc.ModTime, loc.Extension)\n\t\t}\n\t\tquery := `INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES ` +\n\t\t\tstrings.Join(placeholders, \",\") +\n\t\t\t` ON CONFLICT(path) DO UPDATE SET\n\t\t\t\tcontent_hash=excluded.content_hash,\n\t\t\t\tsize_bytes=excluded.size_bytes,\n\t\t\t\tmod_time=excluded.mod_time,\n\t\t\t\textension=excluded.extension`",
    "\t\tfor _, loc := range batch {\n\t\t\tplaceholders = append(placeholders, \"('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?, CASE WHEN ? > 0 THEN ? ELSE unixepoch() END)\")\n\t\t\targs = append(args, loc.Hash, loc.Path, loc.Size, loc.ModTime, loc.Extension, loc.AddedAt, loc.AddedAt)\n\t\t}\n\t\tquery := `INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension, added_at) VALUES ` +\n\t\t\tstrings.Join(placeholders, \",\") +\n\t\t\t` ON CONFLICT(path) DO UPDATE SET\n\t\t\t\tcontent_hash=excluded.content_hash,\n\t\t\t\tsize_bytes=excluded.size_bytes,\n\t\t\t\tmod_time=excluded.mod_time,\n\t\t\t\textension=excluded.extension,\n\t\t\t\tadded_at=CASE WHEN ? > 0 THEN ? ELSE locations.added_at END`",
)
# Each row needs two additional update-guard values at the end of that row's batch args.
replace(
    "internal/database/database.go",
    "\t\tfor _, loc := range batch {\n\t\t\tplaceholders = append(placeholders, \"('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?, CASE WHEN ? > 0 THEN ? ELSE unixepoch() END)\")\n\t\t\targs = append(args, loc.Hash, loc.Path, loc.Size, loc.ModTime, loc.Extension, loc.AddedAt, loc.AddedAt)\n\t\t}\n\t\tquery :=",
    "\t\tfor _, loc := range batch {\n\t\t\tplaceholders = append(placeholders, \"('file_' || lower(hex(randomblob(16))), ?, ?, ?, ?, ?, CASE WHEN ? > 0 THEN ? ELSE unixepoch() END)\")\n\t\t\targs = append(args, loc.Hash, loc.Path, loc.Size, loc.ModTime, loc.Extension, loc.AddedAt, loc.AddedAt)\n\t\t}\n\t\tquery :=",
)
# SQLite ON CONFLICT update placeholders cannot be per-row with one trailing guard; use excluded added_at unconditionally.
replace(
    "internal/database/database.go",
    "\t\t\t\textension=excluded.extension,\n\t\t\t\tadded_at=CASE WHEN ? > 0 THEN ? ELSE locations.added_at END`",
    "\t\t\t\textension=excluded.extension,\n\t\t\t\tadded_at=excluded.added_at`",
)
replace(
    "internal/database/database.go",
    "\tconst columns = 9 // content_hash, path, size_bytes, mod_time, extension, plus explicit added_at insert/update guards",
    "\tconst columns = 7 // content_hash, path, size_bytes, mod_time, extension, added_at guard/value",
)

# Target config and normalization.
replace(
    "internal/serve/config.go",
    'type UploadTarget struct {\n\tID   string `yaml:"id"`\n\tName string `yaml:"name"`\n\tPath string `yaml:"path"`\n}',
    'type UploadTarget struct {\n\tID              string `yaml:"id"`\n\tName            string `yaml:"name"`\n\tPath            string `yaml:"path"`\n\tAddedAtStrategy string `yaml:"added_at_strategy"`\n}',
)
replace(
    "internal/serve/config.go",
    "\t\tpath := strings.TrimSpace(target.Path)\n\t\tcfg.Uploads.Targets[i].ID = id\n\t\tcfg.Uploads.Targets[i].Name = name\n\t\tcfg.Uploads.Targets[i].Path = path",
    "\t\tpath := strings.TrimSpace(target.Path)\n\t\tstrategy := normalizeAddedAtStrategy(target.AddedAtStrategy)\n\t\tcfg.Uploads.Targets[i].ID = id\n\t\tcfg.Uploads.Targets[i].Name = name\n\t\tcfg.Uploads.Targets[i].Path = path\n\t\tcfg.Uploads.Targets[i].AddedAtStrategy = strategy",
)
replace(
    "internal/serve/config.go",
    "\t\tif name == \"\" {\n\t\t\terrs = append(errs, fmt.Errorf(\"uploads target %q name is required\", id))\n\t\t}\n\t\tif path == \"\" {",
    "\t\tif name == \"\" {\n\t\t\terrs = append(errs, fmt.Errorf(\"uploads target %q name is required\", id))\n\t\t}\n\t\tif !validAddedAtStrategy(strategy) {\n\t\t\terrs = append(errs, fmt.Errorf(\"uploads target %q added_at_strategy must be one of: queue, reverse_queue, modtime\", id))\n\t\t}\n\t\tif path == \"\" {",
)
insert = '''\nfunc normalizeAddedAtStrategy(value string) string {\n\tvalue = strings.ToLower(strings.TrimSpace(value))\n\tif value == "" {\n\t\treturn "queue"\n\t}\n\treturn value\n}\n\nfunc validAddedAtStrategy(value string) bool {\n\tswitch normalizeAddedAtStrategy(value) {\n\tcase "queue", "reverse_queue", "modtime":\n\t\treturn true\n\tdefault:\n\t\treturn false\n\t}\n}\n'''
replace("internal/serve/config.go", "\nfunc hasUploadTarget(targets []UploadTarget) bool {", insert + "\nfunc hasUploadTarget(targets []UploadTarget) bool {")

# Upload transport and importer state.
replace(
    "internal/serve/uploads.go",
    "\tSourceModTime time.Time\n}",
    "\tSourceModTime time.Time\n\tAddedAt       time.Time\n}",
)
replace(
    "internal/serve/uploads.go",
    'type UploadTargetDTO struct {\n\tID   string `json:"id"`\n\tName string `json:"name"`\n}',
    'type UploadTargetDTO struct {\n\tID              string `json:"id"`\n\tName            string `json:"name"`\n\tAddedAtStrategy string `json:"added_at_strategy"`\n}',
)
replace(
    "internal/serve/uploads.go",
    "items = append(items, UploadTargetDTO{ID: target.ID, Name: target.Name})",
    "items = append(items, UploadTargetDTO{ID: target.ID, Name: target.Name, AddedAtStrategy: normalizeAddedAtStrategy(target.AddedAtStrategy)})",
)
replace(
    "internal/serve/uploads.go",
    "\tsourceModTime   time.Time\n}",
    "\tsourceModTime   time.Time\n\taddedAt         time.Time\n}",
)
replace(
    "internal/serve/uploads.go",
    "SourceModTime: file.sourceModTime})",
    "SourceModTime: file.sourceModTime, AddedAt: file.addedAt})",
)
replace(
    "internal/serve/uploads.go",
    "\t\timportLocations = append(importLocations, types.LocationInfo{\n\t\t\tPath:      file.Path,\n\t\t\tHash:      info.Hash,\n\t\t\tSize:      info.Size,\n\t\t\tModTime:   info.ModTime,\n\t\t\tExtension: filepath.Ext(file.Path),\n\t\t})",
    "\t\taddedAt := int64(0)\n\t\tif !file.AddedAt.IsZero() {\n\t\t\taddedAt = file.AddedAt.Unix()\n\t\t}\n\t\timportLocations = append(importLocations, types.LocationInfo{\n\t\t\tPath:      file.Path,\n\t\t\tHash:      info.Hash,\n\t\t\tSize:      info.Size,\n\t\t\tModTime:   info.ModTime,\n\t\t\tAddedAt:   addedAt,\n\t\t\tExtension: filepath.Ext(file.Path),\n\t\t})",
)

# Parse queue metadata and per-request strategy; compute AddedAt before async handoff.
replace(
    "internal/serve/upload_stream.go",
    "\tsourceModTime time.Time\n}",
    "\tsourceModTime time.Time\n\tqueueTime     time.Time\n\tqueueOrder    int64\n\taddedAt       time.Time\n}",
)
replace(
    "internal/serve/upload_stream.go",
    "\tvar targetID, conflictRequested string\n\tvar targetSeen, conflictSeen bool\n\ttagValues := make([]string, 0)\n\tsourceModTimeValues := make([]string, 0)",
    "\trequestQueueTime := time.Now().UTC()\n\tvar targetID, conflictRequested, strategyRequested string\n\tvar targetSeen, conflictSeen, strategySeen bool\n\ttagValues := make([]string, 0)\n\tsourceModTimeValues := make([]string, 0)\n\tqueueTimeValues := make([]string, 0)\n\tqueueOrderValues := make([]string, 0)",
)
replace(
    "internal/serve/upload_stream.go",
    '\t\t\tcase "source_modtime_ms":\n\t\t\t\tsourceModTimeValues = append(sourceModTimeValues, value)\n\t\t\t}',
    '\t\t\tcase "source_modtime_ms":\n\t\t\t\tsourceModTimeValues = append(sourceModTimeValues, value)\n\t\t\tcase "queue_time_ms":\n\t\t\t\tqueueTimeValues = append(queueTimeValues, value)\n\t\t\tcase "queue_order":\n\t\t\t\tqueueOrderValues = append(queueOrderValues, value)\n\t\t\tcase "added_at_strategy":\n\t\t\t\tif !strategySeen {\n\t\t\t\t\tstrategyRequested, strategySeen = value, true\n\t\t\t\t}\n\t\t\t}',
)
replace(
    "internal/serve/upload_stream.go",
    "\tfor i := range streamed {\n\t\tif i >= len(sourceModTimeValues) {\n\t\t\tbreak\n\t\t}\n\t\tstreamed[i].sourceModTime = parseUploadSourceModTime(sourceModTimeValues[i])\n\t}\n\ttarget, err := s.uploadTarget(targetID)",
    "\tfor i := range streamed {\n\t\tif i < len(sourceModTimeValues) {\n\t\t\tstreamed[i].sourceModTime = parseUploadSourceModTime(sourceModTimeValues[i])\n\t\t}\n\t\tif i < len(queueTimeValues) {\n\t\t\tstreamed[i].queueTime = parseUploadSourceModTime(queueTimeValues[i])\n\t\t}\n\t\tif streamed[i].queueTime.IsZero() {\n\t\t\tstreamed[i].queueTime = requestQueueTime\n\t\t}\n\t\tif i < len(queueOrderValues) {\n\t\t\tstreamed[i].queueOrder = parseUploadQueueOrder(queueOrderValues[i], int64(i))\n\t\t} else {\n\t\t\tstreamed[i].queueOrder = int64(i)\n\t\t}\n\t}\n\ttarget, err := s.uploadTarget(targetID)",
)
replace(
    "internal/serve/upload_stream.go",
    "\tconflictPolicy, err := uploadConflictPolicy(conflictRequested, s.cfg.Uploads.ConflictPolicy)\n\tif err != nil {",
    "\tstrategy, err := uploadAddedAtStrategy(strategyRequested, target.AddedAtStrategy)\n\tif err != nil {\n\t\treturn nil, saved, multipartUploadError{message: err.Error(), err: err}\n\t}\n\tfor i := range streamed {\n\t\tstreamed[i].addedAt = uploadAddedAt(strategy, streamed[i])\n\t}\n\tconflictPolicy, err := uploadConflictPolicy(conflictRequested, s.cfg.Uploads.ConflictPolicy)\n\tif err != nil {",
)
# propagate metadata through finalize outputs (three success paths)
replace(
    "internal/serve/upload_stream.go",
    "status: \"skipped\", sourceModTime: file.sourceModTime}",
    "status: \"skipped\", sourceModTime: file.sourceModTime, addedAt: file.addedAt}",
)
replace(
    "internal/serve/upload_stream.go",
    "replace: true, sourceModTime: file.sourceModTime}",
    "replace: true, sourceModTime: file.sourceModTime, addedAt: file.addedAt}",
)
replace(
    "internal/serve/upload_stream.go",
    "targetID: target.ID, sourceModTime: file.sourceModTime}",
    "targetID: target.ID, sourceModTime: file.sourceModTime, addedAt: file.addedAt}",
)
helpers = '''\nfunc parseUploadQueueOrder(value string, fallback int64) int64 {\n\torder, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)\n\tif err != nil || order < 0 {\n\t\treturn fallback\n\t}\n\treturn order\n}\n\nfunc uploadAddedAtStrategy(requested, fallback string) (string, error) {\n\trequested = normalizeAddedAtStrategy(requested)\n\tif strings.TrimSpace(requested) == "queue" && strings.TrimSpace(fallback) != "" && strings.TrimSpace(requested) == strings.TrimSpace(normalizeAddedAtStrategy("")) {\n\t\t// A blank request normalizes to queue; use the target fallback instead.\n\t\trequested = normalizeAddedAtStrategy(fallback)\n\t}\n\tif !validAddedAtStrategy(requested) {\n\t\treturn "", errors.New("added_at_strategy must be one of: queue, reverse_queue, modtime")\n\t}\n\treturn requested, nil\n}\n\nfunc uploadAddedAt(strategy string, file streamedUpload) time.Time {\n\tqueue := file.queueTime\n\tif queue.IsZero() {\n\t\tqueue = time.Now().UTC()\n\t}\n\t// locations.added_at is second-resolution. Encode queue order as a second offset so\n\t// concurrently submitted files selected together remain deterministic.\n\tqueue = time.Unix(queue.Unix()+file.queueOrder, 0).UTC()\n\tswitch normalizeAddedAtStrategy(strategy) {\n\tcase "reverse_queue":\n\t\treturn time.Unix(file.queueTime.Unix()-file.queueOrder, 0).UTC()\n\tcase "modtime":\n\t\tif !file.sourceModTime.IsZero() {\n\t\t\treturn time.Unix(file.sourceModTime.Unix(), 0).UTC()\n\t\t}\n\t}\n\treturn queue\n}\n'''
replace("internal/serve/upload_stream.go", "\nfunc applyUploadedSourceModTime(path string, sourceModTime time.Time, preserve bool) error {", helpers + "\nfunc applyUploadedSourceModTime(path string, sourceModTime time.Time, preserve bool) error {")

# Config regression tests.
replace(
    "internal/serve/config_test.go",
    'if !cfg.Uploads.PreserveModTime {\n\t\tt.Fatal("uploads.preserve_modtime should default true")\n\t}',
    'if !cfg.Uploads.PreserveModTime {\n\t\tt.Fatal("uploads.preserve_modtime should default true")\n\t}',
)
# Append focused tests to upload stream tests.
p = Path("internal/serve/upload_stream_test.go")
text = p.read_text()
tests = r'''\nfunc TestUploadAddedAtStrategyResolution(t *testing.T) {\n\tbase := time.Unix(1_700_000_000, 0).UTC()\n\tfile := streamedUpload{queueTime: base, queueOrder: 3, sourceModTime: time.Unix(1_600_000_000, 0).UTC()}\n\tif got := uploadAddedAt("queue", file); got.Unix() != base.Unix()+3 {\n\t\tt.Fatalf("queue added_at=%d want %d", got.Unix(), base.Unix()+3)\n\t}\n\tif got := uploadAddedAt("reverse_queue", file); got.Unix() != base.Unix()-3 {\n\t\tt.Fatalf("reverse queue added_at=%d want %d", got.Unix(), base.Unix()-3)\n\t}\n\tif got := uploadAddedAt("modtime", file); got.Unix() != file.sourceModTime.Unix() {\n\t\tt.Fatalf("modtime added_at=%d want %d", got.Unix(), file.sourceModTime.Unix())\n\t}\n\tfile.sourceModTime = time.Time{}\n\tif got := uploadAddedAt("modtime", file); got.Unix() != base.Unix()+3 {\n\t\tt.Fatalf("modtime fallback=%d want queue %d", got.Unix(), base.Unix()+3)\n\t}\n}\n'''.replace("\\t", "\t")
if "func TestUploadAddedAtStrategyResolution" not in text:
    p.write_text(text + tests)

# Canonical config docs.
replace(
    "docs/CONFIG.md",
    "| `path` | yes | Absolute destination path. |",
    "| `path` | yes | Absolute destination path. |\n| `added_at_strategy` | no | Default added-time strategy for this target: `queue` (default), `reverse_queue`, or `modtime`. Upload requests may override it. |",
)
replace(
    "docs/CONFIG.md",
    "      path: /srv/media/inbox",
    "      path: /srv/media/inbox\n      added_at_strategy: queue",
    2,
)

# OpenAPI: target DTO and multipart fields. Use resilient insertions around existing known fields.
p = Path("docs/openapi.yaml")
text = p.read_text()
needle = "                source_modtime_ms:\n                  type: array"
if needle not in text:
    raise SystemExit("openapi source_modtime_ms field not found")
addition = "                queue_time_ms:\n                  type: array\n                  description: Browser-captured queue timestamps in Unix milliseconds, in the same order as `files`.\n                  items:\n                    type: integer\n                    format: int64\n                queue_order:\n                  type: array\n                  description: Deterministic zero-based queue order values, in the same order as `files`.\n                  items:\n                    type: integer\n                    format: int64\n                added_at_strategy:\n                  type: string\n                  enum: [queue, reverse_queue, modtime]\n                  description: Optional per-upload override of the selected target's default added-time strategy.\n"
text = text.replace(needle, addition + needle, 1)
# Add field to UploadTarget schema by locating its name property block.
marker = "        name:\n          type: string\n"
# Restrict to schemas section by replacing last occurrence if necessary.
idx = text.rfind(marker)
if idx < 0:
    raise SystemExit("openapi upload target name schema marker not found")
insert_at = idx + len(marker)
text = text[:insert_at] + "        added_at_strategy:\n          type: string\n          enum: [queue, reverse_queue, modtime]\n" + text[insert_at:]
p.write_text(text)
