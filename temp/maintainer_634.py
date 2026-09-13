from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing patch target: {label}")
    return text.replace(old, new, 1)


scanner = Path("internal/scanning/scanner.go")
text = scanner.read_text()
text = replace_once(text, 'import (\n\t"io/fs"', 'import (\n\t"fmt"\n\t"io/fs"', "scanner import")
text = replace_once(
    text,
    'func DirsConcurrently(dirs []string, sizeToHashes map[int64][]string, hasher *hashing.Hasher) (map[string]types.LocationInfo, int) {\n\tjobs := make(chan job)\n\tresults := make(chan result)',
    'func DirsConcurrently(dirs []string, sizeToHashes map[int64][]string, hasher *hashing.Hasher) (map[string]types.LocationInfo, int, error) {\n\tjobs := make(chan job)\n\tresults := make(chan result)\n\twalkErrs := make(chan error, len(dirs))',
    "scanner signature",
)
text = replace_once(
    text,
    '''\t\tgo func(d string) {\n\t\t\tdefer walkWg.Done()\n\t\t\t_ = filepath.WalkDir(d, func(path string, de fs.DirEntry, err error) error {\n\t\t\t\tif err != nil {\n\t\t\t\t\treturn nil // Skip files we can't access.\n\t\t\t\t}\n\t\t\t\tif !de.IsDir() {\n\t\t\t\t\tjobs <- job{path: path}\n\t\t\t\t}\n\t\t\t\treturn nil\n\t\t\t})\n\t\t}(dir)''',
    '''\t\tgo func(d string) {\n\t\t\tdefer walkWg.Done()\n\t\t\tif err := filepath.WalkDir(d, func(path string, de fs.DirEntry, err error) error {\n\t\t\t\tif err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n\t\t\t\tif !de.IsDir() {\n\t\t\t\t\tjobs <- job{path: path}\n\t\t\t\t}\n\t\t\t\treturn nil\n\t\t\t}); err != nil {\n\t\t\t\twalkErrs <- fmt.Errorf("walk %q: %w", d, err)\n\t\t\t}\n\t\t}(dir)''',
    "scanner walk",
)
text = replace_once(
    text,
    '''\tgo func() {\n\t\twalkWg.Wait()\n\t\tclose(jobs)\n\t}()''',
    '''\tgo func() {\n\t\twalkWg.Wait()\n\t\tclose(jobs)\n\t\tclose(walkErrs)\n\t}()''',
    "scanner walk close",
)
text = replace_once(
    text,
    '''\tfoundFiles := make(map[string]types.LocationInfo)\n\tfilesScanned := 0\n\tfor res := range results {\n\t\tfilesScanned++\n\t\tif res.err == nil && !res.skipped {\n\t\t\tfoundFiles[res.path] = res.info\n\t\t}\n\t}\n\n\treturn foundFiles, filesScanned''',
    '''\tfoundFiles := make(map[string]types.LocationInfo)\n\tfilesScanned := 0\n\tvar firstErr error\n\tfor res := range results {\n\t\tfilesScanned++\n\t\tif res.err != nil {\n\t\t\tif firstErr == nil {\n\t\t\t\tfirstErr = fmt.Errorf("inspect %q: %w", res.path, res.err)\n\t\t\t}\n\t\t\tcontinue\n\t\t}\n\t\tif !res.skipped {\n\t\t\tfoundFiles[res.path] = res.info\n\t\t}\n\t}\n\tfor err := range walkErrs {\n\t\tif firstErr == nil {\n\t\t\tfirstErr = err\n\t\t}\n\t}\n\tif firstErr != nil {\n\t\treturn nil, filesScanned, firstErr\n\t}\n\n\treturn foundFiles, filesScanned, nil''',
    "scanner result collection",
)
scanner.write_text(text)

sync = Path("gooru/sync.go")
text = sync.read_text()
text = replace_once(text, '''\t\t\tif err != nil {\n\t\t\t\treturn nil // Skip unreadable files/dirs\n\t\t\t}''', '''\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}''', "precheck walk error")
text = replace_once(text, '''\t\t\t\tinfo, err := c.hasher.FileMetadata(path)\n\t\t\t\tif err != nil {\n\t\t\t\t\treturn nil // Skip files we can't inspect through the configured source policy\n\t\t\t\t}''', '''\t\t\t\tinfo, err := c.hasher.FileMetadata(path)\n\t\t\t\tif err != nil {\n\t\t\t\t\treturn fmt.Errorf("inspect %q: %w", path, err)\n\t\t\t\t}''', "precheck metadata error")
text = replace_once(text, '''\t\tinfo, err := c.hasher.FileMetadata(source.StoragePath)\n\t\tif err != nil {\n\t\t\tcontinue\n\t\t}''', '''\t\tinfo, err := c.hasher.FileMetadata(source.StoragePath)\n\t\tif err != nil {\n\t\t\treturn false, fmt.Errorf("inspect managed source %q: %w", source.StoragePath, err)\n\t\t}''', "managed source metadata error")
text = replace_once(text, '''\t\t\tcurrentHash, err := c.hasher.HashFile(sourcePath)\n\t\t\tif err != nil {\n\t\t\t\treturn true, nil // Can't hash the file, treat as changed.\n\t\t\t}''', '''\t\t\tcurrentHash, err := c.hasher.HashFile(sourcePath)\n\t\t\tif err != nil {\n\t\t\t\treturn false, fmt.Errorf("hash %q during relink pre-check: %w", sourcePath, err)\n\t\t\t}''', "precheck hash error")
text = replace_once(text, '''\tfsLocations, filesScanned := scanning.DirsConcurrently(absDirs, sizeToHashes, c.hasher)\n\n\tmanagedAliases, err := c.store.BatchGetManagedLocationSourcesByPhysicalPaths(locationPaths(fsLocations))''', '''\tfsLocations, filesScanned, err := scanning.DirsConcurrently(absDirs, sizeToHashes, c.hasher)\n\tif err != nil {\n\t\treturn result, fmt.Errorf("scan relink directories: %w", err)\n\t}\n\n\tmanagedAliases, err := c.store.BatchGetManagedLocationSourcesByPhysicalPaths(locationPaths(fsLocations))''', "relink scanner error")
sync.write_text(text)

test = Path("gooru/sync_test.go")
text = test.read_text()
if "func TestClient_RelinkFailsClosedOnScanErrors" in text:
    raise SystemExit("regression test already present")
text += r'''

func TestClient_RelinkFailsClosedOnScanErrors(t *testing.T) {
	t.Run("missing scan root", func(t *testing.T) {
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		defer client.Close()

		root := t.TempDir()
		path := createTestFile(t, root, "tracked.txt", "tracked contents")
		_, err = client.TagFiles([]string{path}, []string{"tag1"}, nil, false)
		require.NoError(t, err)
		require.NoError(t, os.RemoveAll(root))

		result, err := client.Relink([]string{root})
		require.Error(t, err)
		assert.Empty(t, result.ProposedMoves)
		assert.Empty(t, result.ProposedAdds)
		assert.Empty(t, result.ProposedDeletes)
	})

	t.Run("protected source inspection failure", func(t *testing.T) {
		dbPath := setupTestDB(t)
		root := t.TempDir()
		path := createTestFile(t, root, "tracked.bin", "plaintext tracked contents")

		plainClient, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		_, err = plainClient.TagFiles([]string{path}, []string{"tag1"}, nil, false)
		require.NoError(t, err)
		require.NoError(t, plainClient.Close())

		client, err := gooru.NewWithOptions(dbPath, false, gooru.OpenOptions{
			Content: gooru.ContentSourceOptions{
				EncryptionKey:  bytes.Repeat([]byte{0x6b}, 32),
				ProtectedRoots: []string{root},
			},
		})
		require.NoError(t, err)
		defer client.Close()

		needsRelink, err := client.NeedsRelink([]string{root}, false)
		require.Error(t, err)
		assert.False(t, needsRelink)

		result, err := client.Relink([]string{root})
		require.Error(t, err)
		assert.Empty(t, result.ProposedMoves)
		assert.Empty(t, result.ProposedAdds)
		assert.Empty(t, result.ProposedDeletes)
	})
}
'''
test.write_text(text)
