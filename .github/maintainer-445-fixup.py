from pathlib import Path

p = Path('.github/maintainer-445-apply.py')
s = p.read_text()
s = s.replace("replace('internal/serve/config_upload_policy_test.go', 'default upload conflict policy = %q, want skip', 'default upload conflict policy = %q, want rename')", "replace('internal/serve/config_upload_policy_test.go', 'expected default uploads.conflict_policy=skip, got %q', 'expected default uploads.conflict_policy=rename, got %q')")
s = s.replace("replace('internal/serve/config_upload_policy_test.go', 'empty upload conflict policy = %q, want skip', 'empty upload conflict policy = %q, want rename')", "replace('internal/serve/config_upload_policy_test.go', 'expected omitted conflict policy to resolve to skip, got %q', 'expected omitted conflict policy to resolve to rename, got %q')")
s = s.replace("replace('internal/serve/uploads.go', 'response, err := importer.ImportUploadedFiles(ctx, stagedUploads(saved), tags)', 'response, err := importer.ImportUploadedFiles(ctx, stagedUploads(saved, conflictPolicy), tags)')\n", "")
s = s.replace("replace('internal/serve/uploads.go', 'func stagedUploads(files []savedUpload) []StagedUpload {', 'func stagedUploads(files []savedUpload, conflictPolicy string) []StagedUpload {')\n", "")
s = s.replace("replace('internal/serve/uploads.go', 'SourceModTime: file.sourceModTime, AddedAt: file.addedAt})', 'SourceModTime: file.sourceModTime, AddedAt: file.addedAt, ConflictPolicy: conflictPolicy})')", "replace('internal/serve/uploads.go', 'SourceModTime: file.sourceModTime, AddedAt: file.addedAt})', 'SourceModTime: file.sourceModTime, AddedAt: file.addedAt, ConflictPolicy: file.conflictPolicy})')")
s += "\nreplace('internal/serve/uploads.go', '\\tsourceModTime   time.Time\\n\\taddedAt         time.Time\\n}', '\\tsourceModTime   time.Time\\n\\taddedAt         time.Time\\n\\tconflictPolicy  string\\n}')\n"
s += "replace('internal/serve/upload_stream.go', '\\t\\t\\tfinalized.addedAt = file.addedAt\\n', '\\t\\t\\tfinalized.addedAt = file.addedAt\\n\\t\\t\\tfinalized.conflictPolicy = conflictPolicy\\n')\n"
s += "replace('internal/serve/config_test.go', 'cfg.Uploads.ConflictPolicy != \\\"skip\\\"', 'cfg.Uploads.ConflictPolicy != \\\"rename\\\"')\n"
p.write_text(s)
