from pathlib import Path


def rep(path, old, new):
    p = Path(path)
    text = p.read_text()
    if new in text:
        return
    if old not in text:
        raise SystemExit(f"missing anchor {path}: {old[:100]!r}")
    p.write_text(text.replace(old, new, 1))

rep('internal/serve/config.go', '''\t\tpath := strings.TrimSpace(target.Path)\n\t\tcfg.Uploads.Targets[i].ID = id\n\t\tcfg.Uploads.Targets[i].Name = name\n\t\tcfg.Uploads.Targets[i].Path = path''', '''\t\tpath := strings.TrimSpace(target.Path)\n\t\taddedAtStrategy := strings.TrimSpace(target.AddedAtStrategy)\n\t\tif addedAtStrategy == "" {\n\t\t\taddedAtStrategy = "queue"\n\t\t}\n\t\tcfg.Uploads.Targets[i].ID = id\n\t\tcfg.Uploads.Targets[i].Name = name\n\t\tcfg.Uploads.Targets[i].Path = path\n\t\tcfg.Uploads.Targets[i].AddedAtStrategy = addedAtStrategy''')
rep('internal/serve/config.go', '''\t\tif path == "" {\n\t\t\terrs = append(errs, fmt.Errorf("uploads target %q path is required", id))\n\t\t} else if !filepath.IsAbs(path) {''', '''\t\tswitch addedAtStrategy {\n\t\tcase "queue", "reverse_queue", "modtime":\n\t\tdefault:\n\t\t\terrs = append(errs, fmt.Errorf("uploads target %q added_at_strategy must be one of: queue, reverse_queue, modtime", id))\n\t\t}\n\t\tif path == "" {\n\t\t\terrs = append(errs, fmt.Errorf("uploads target %q path is required", id))\n\t\t} else if !filepath.IsAbs(path) {''')

rep('internal/serve/config_test.go', '''func TestLoadConfigDefaultsAreValid(t *testing.T) {''', '''func TestConfigUploadTargetAddedAtStrategyDefaultsAndValidates(t *testing.T) {\n\tcfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))\n\tcfg.Uploads.Enabled = true\n\tcfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: t.TempDir()}}\n\tif err := cfg.Validate(); err != nil {\n\t\tt.Fatalf("Validate default target strategy: %v", err)\n\t}\n\tif got := cfg.Uploads.Targets[0].AddedAtStrategy; got != "queue" {\n\t\tt.Fatalf("default added_at_strategy=%q want queue", got)\n\t}\n\tcfg.Uploads.Targets[0].AddedAtStrategy = "random"\n\tif err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "added_at_strategy") {\n\t\tt.Fatalf("expected added_at_strategy validation error, got %v", err)\n\t}\n}\n\nfunc TestLoadConfigDefaultsAreValid(t *testing.T) {''')

rep('docs/openapi.yaml', '''            required: [id, name]\n            properties:\n              id:\n                type: string\n              name:\n                type: string''', '''            required: [id, name, added_at_strategy]\n            properties:\n              id:\n                type: string\n              name:\n                type: string\n              added_at_strategy:\n                type: string\n                enum: [queue, reverse_queue, modtime]\n                description: Default added-at strategy configured for this upload target.''')

p = Path('internal/database/database_test.go')
text = p.read_text()
if '"gooru.local/types"' not in text:
    text = text.replace('"testing"\n)', '"testing"\n\n\t"gooru.local/types"\n)')
if 'TestBatchUpsertLocationsPersistsExplicitAddedAtWithoutRewritingExistingValue' not in text:
    text += r'''

func TestBatchUpsertLocationsPersistsExplicitAddedAtWithoutRewritingExistingValue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := CreateEmptyDB(dbPath); err != nil { t.Fatalf("CreateEmptyDB: %v", err) }
	store, err := NewStore(dbPath, false)
	if err != nil { t.Fatalf("NewStore: %v", err) }
	defer store.Close()
	if err := RunMigrations(store.DB); err != nil { t.Fatalf("RunMigrations: %v", err) }
	if err := store.BatchInsertContents(store.DB, []string{"hash-one", "hash-two"}); err != nil { t.Fatalf("BatchInsertContents: %v", err) }
	path := "/library/ordered.jpg"
	if err := store.BatchUpsertLocations(store.DB, map[string]types.LocationInfo{path: {Path: path, Hash: "hash-one", Size: 1, ModTime: 10, AddedAt: 1234, Extension: ".jpg"}}); err != nil { t.Fatalf("first upsert: %v", err) }
	file, err := store.GetFileInfoByPath(path)
	if err != nil { t.Fatal(err) }
	if file.AddedAt != 1234 { t.Fatalf("added_at=%d want 1234", file.AddedAt) }
	if err := store.BatchUpsertLocations(store.DB, map[string]types.LocationInfo{path: {Path: path, Hash: "hash-two", Size: 2, ModTime: 20, AddedAt: 9999, Extension: ".jpg"}}); err != nil { t.Fatalf("second upsert: %v", err) }
	file, err = store.GetFileInfoByPath(path)
	if err != nil { t.Fatal(err) }
	if file.AddedAt != 1234 { t.Fatalf("existing added_at changed to %d, want stable 1234", file.AddedAt) }
}
'''
p.write_text(text)

p = Path('internal/serve/upload_stream.go')
text = p.read_text().replace('\t\tstreamed[i].sourceModTime = firstNonZeroTime(streamed[i].sourceModTime, time.Time{})\n', '')
text = text.replace('''func firstNonZeroTime(value time.Time, fallback time.Time) time.Time {\n\tif value.IsZero() {\n\t\treturn fallback\n\t}\n\treturn value\n}\n\n''', '')
p.write_text(text)
