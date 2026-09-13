from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing patch target: {label}")
    return text.replace(old, new, 1)


database = Path("internal/database/database.go")
text = database.read_text()
text = replace_once(
    text,
    '''func splitTags(cache string) []string {\n\tif cache == "" {\n\t\treturn nil\n\t}\n\treturn strings.Split(cache, " ")\n}\n''',
    '''func splitTags(cache string) []string {\n\tif cache == "" {\n\t\treturn nil\n\t}\n\treturn strings.Split(cache, " ")\n}\n\n// escapeLikeLiteral escapes SQLite LIKE metacharacters so filesystem paths are\n// matched literally. The trailing wildcard is added separately by the caller.\nfunc escapeLikeLiteral(value string) string {\n\treplacer := strings.NewReplacer("~", "~~", "%", "~%", "_", "~_")\n\treturn replacer.Replace(value)\n}\n''',
    "like escape helper",
)
text = replace_once(
    text,
    '''// GetLocationsForDirs retrieves a map of all known paths to their content hashes for the given directories.\nfunc (s *Store) GetLocationsForDirs(dirs []string) (map[string]types.LocationInfo, error) {\n\tlocations := make(map[string]types.LocationInfo)\n\tfor _, dir := range dirs {\n\t\trows, err := s.Query("SELECT path, content_hash, size_bytes, mod_time, extension, tags_cache FROM locations WHERE path LIKE ?", dir+string(filepath.Separator)+"%")\n\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tdefer rows.Close()\n\n\t\tfor rows.Next() {\n\t\t\tvar path string\n\t\t\tvar info types.LocationInfo\n\t\t\tif err := rows.Scan(&path, &info.Hash, &info.Size, &info.ModTime, &info.Extension, &info.TagsCache); err != nil {\n\t\t\t\treturn nil, err\n\t\t\t}\n\t\t\tlocations[path] = info\n\t\t}\n\t}\n\treturn locations, nil\n}\n''',
    '''// GetLocationsForDirs retrieves a map of all known paths to their content hashes for the given directories.\nfunc (s *Store) GetLocationsForDirs(dirs []string) (map[string]types.LocationInfo, error) {\n\tlocations := make(map[string]types.LocationInfo)\n\tfor _, dir := range dirs {\n\t\tpattern := escapeLikeLiteral(dir+string(filepath.Separator)) + "%"\n\t\terr := func() error {\n\t\t\trows, err := s.Query("SELECT path, content_hash, size_bytes, mod_time, extension, tags_cache FROM locations WHERE path LIKE ? ESCAPE '~'", pattern)\n\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}\n\t\t\tdefer rows.Close()\n\n\t\t\tfor rows.Next() {\n\t\t\t\tvar path string\n\t\t\t\tvar info types.LocationInfo\n\t\t\t\tif err := rows.Scan(&path, &info.Hash, &info.Size, &info.ModTime, &info.Extension, &info.TagsCache); err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n\t\t\t\tlocations[path] = info\n\t\t\t}\n\t\t\treturn rows.Err()\n\t\t}()\n\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t}\n\treturn locations, nil\n}\n''',
    "GetLocationsForDirs",
)
database.write_text(text)

sync_test = Path("gooru/sync_test.go")
text = sync_test.read_text()
if "func TestClient_RelinkTreatsWildcardDirectoriesLiterally" in text:
    raise SystemExit("wildcard regression already present")
text += r'''

func TestClient_RelinkTreatsWildcardDirectoriesLiterally(t *testing.T) {
	tests := []struct {
		name        string
		targetName  string
		siblingName string
	}{
		{name: "underscore", targetName: "scope_a", siblingName: "scopeXa"},
		{name: "percent", targetName: "scope%a", siblingName: "scopeZZa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := setupTestDB(t)
			client, err := gooru.New(dbPath, false)
			require.NoError(t, err)
			defer client.Close()

			parent := t.TempDir()
			targetDir := filepath.Join(parent, tt.targetName)
			siblingDir := filepath.Join(parent, tt.siblingName)
			require.NoError(t, os.Mkdir(targetDir, 0o755))
			require.NoError(t, os.Mkdir(siblingDir, 0o755))

			targetPath := createTestFile(t, targetDir, "target.txt", "target contents")
			siblingPath := createTestFile(t, siblingDir, "sibling.txt", "sibling contents")
			_, err = client.TagFiles([]string{targetPath, siblingPath}, []string{"tag1"}, nil, false)
			require.NoError(t, err)

			needsRelink, err := client.NeedsRelink([]string{targetDir}, false)
			require.NoError(t, err)
			assert.False(t, needsRelink, "LIKE-compatible sibling must stay outside literal relink scope")

			result, err := client.Relink([]string{targetDir})
			require.NoError(t, err)
			assert.Empty(t, result.ProposedMoves)
			assert.Empty(t, result.ProposedAdds)
			assert.Empty(t, result.ProposedDeletes, "out-of-scope sibling must never be proposed for deletion")
		})
	}
}
'''
sync_test.write_text(text)
