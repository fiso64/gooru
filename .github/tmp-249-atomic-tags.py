from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))


p = Path("gooru/tagging_known_file_tags.go")
text = p.read_text()
start = text.index("func (c *Client) TagKnownFilesWithBackgroundTasksByFileTags(")
end = text.index("// TagExistingContentByHashTags")
new_block = '''func (c *Client) TagKnownFilesWithBackgroundTasksByFileTags(files []types.LocationInfo, fileTags [][]string, tasks []BackgroundTaskRequest, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
\treturn c.TagKnownFilesWithBackgroundTasksByFileTagsAndOperationState(files, fileTags, tasks, nil, progressCb)
}

// TagKnownFilesWithBackgroundTasksByFileTagsAndOperationState extends the
// per-file known-file path with producer-owned operation/task state persisted in
// the same transaction as content, tags, and child tasks.
func (c *Client) TagKnownFilesWithBackgroundTasksByFileTagsAndOperationState(files []types.LocationInfo, fileTags [][]string, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
\tresult := types.TagOperationResult{}
\tif len(fileTags) != len(files) {
\t\treturn result, fmt.Errorf("per-file tag count %d does not match file count %d", len(fileTags), len(files))
\t}
\ttagsByHash := make(map[string][]string, len(files))
\tfor index, file := range files {
\t\tif err := query.ValidateTags(fileTags[index]); err != nil {
\t\t\treturn result, fmt.Errorf("validate tags for file %d: %w", index, err)
\t\t}
\t\tif file.Path == "" || file.Hash == "" {
\t\t\tcontinue
\t\t}
\t\ttagsByHash[file.Hash] = appendUniqueKnownFileTags(tagsByHash[file.Hash], fileTags[index])
\t}
\treturn c.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState(files, tagsByHash, tasks, stateBuilder, progressCb)
}

// TagKnownFilesWithBackgroundTasksByHashTags registers the supplied known files
// and applies tag sets by content hash in one transaction. The tag map may also
// contain already-tracked hashes, which lets callers update duplicate content
// atomically with newly registered files.
func (c *Client) TagKnownFilesWithBackgroundTasksByHashTags(files []types.LocationInfo, tagsByHash map[string][]string, tasks []BackgroundTaskRequest, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
\treturn c.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState(files, tagsByHash, tasks, nil, progressCb)
}

// TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState also persists
// producer recovery state in that same transaction.
func (c *Client) TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState(files []types.LocationInfo, tagsByHash map[string][]string, tasks []BackgroundTaskRequest, stateBuilder BackgroundOperationTransactionStateBuilder, progressCb func(filePath string, err error)) (types.TagOperationResult, error) {
\tresult := types.TagOperationResult{}
\tfor hash, tags := range tagsByHash {
\t\tif hash == "" {
\t\t\treturn result, fmt.Errorf("content hash is empty")
\t\t}
\t\tif err := query.ValidateTags(tags); err != nil {
\t\t\treturn result, fmt.Errorf("validate tags for content %q: %w", hash, err)
\t\t}
\t}

\thashes := make([]string, 0, len(files))
\tlocations := make(map[string]types.LocationInfo, len(files))
\tvalidPaths := make([]string, 0, len(files))
\tfor _, file := range files {
\t\tif file.Path == "" || file.Hash == "" {
\t\t\tif progressCb != nil {
\t\t\t\tprogressCb(file.Path, fmt.Errorf("file path and hash are required"))
\t\t\t}
\t\t\tcontinue
\t\t}
\t\thashes = append(hashes, file.Hash)
\t\tlocations[file.Path] = file
\t\tvalidPaths = append(validPaths, file.Path)
\t}
\tif len(hashes) == 0 && len(tagsByHash) == 0 {
\t\treturn result, nil
\t}

\ttx, err := c.store.Begin()
\tif err != nil {
\t\treturn result, err
\t}
\tdefer tx.Rollback()

\tif len(hashes) > 0 {
\t\tif err := c.store.BatchInsertContents(tx, hashes); err != nil {
\t\t\treturn result, fmt.Errorf("failed to batch insert contents: %w", err)
\t\t}
\t\tif err := c.store.BatchUpsertLocations(tx, locations); err != nil {
\t\t\treturn result, fmt.Errorf("failed to batch upsert locations: %w", err)
\t\t}
\t}
\taffectedCount, err := c.associateKnownFileTagsByHash(tx, tagsByHash)
\tif err != nil {
\t\treturn result, err
\t}
\tif err := c.persistTaggingFollowUpInTx(tx, tasks, stateBuilder, affectedCount); err != nil {
\t\treturn result, err
\t}
\tif err := tx.Commit(); err != nil {
\t\treturn result, err
\t}
\tresult.AffectedCount = int(affectedCount)
\tif progressCb != nil {
\t\tfor _, path := range validPaths {
\t\t\tprogressCb(path, nil)
\t\t}
\t}
\treturn result, nil
}

'''
p.write_text(text[:start] + new_block + text[end:])

p = Path("gooru/tagging_known_file_tags.go")
text = p.read_text()
old_start = text.index("func (c *Client) TagExistingContentByHashTags(")
old_end = text.index("\nfunc appendUniqueKnownFileTags", old_start)
old_block = text[old_start:old_end]
new_existing = '''func (c *Client) TagExistingContentByHashTags(hashes []string, fileTags [][]string) (types.TagOperationResult, error) {
\tresult := types.TagOperationResult{}
\tif len(fileTags) != len(hashes) {
\t\treturn result, fmt.Errorf("per-content tag count %d does not match hash count %d", len(fileTags), len(hashes))
\t}
\ttagsByHash := make(map[string][]string, len(hashes))
\tfor index, hash := range hashes {
\t\tif hash == "" {
\t\t\treturn result, fmt.Errorf("content hash %d is empty", index)
\t\t}
\t\tif err := query.ValidateTags(fileTags[index]); err != nil {
\t\t\treturn result, fmt.Errorf("validate tags for content %d: %w", index, err)
\t\t}
\t\ttagsByHash[hash] = appendUniqueKnownFileTags(tagsByHash[hash], fileTags[index])
\t}
\treturn c.TagKnownFilesWithBackgroundTasksByHashTags(nil, tagsByHash, nil, nil)
}
'''
p.write_text(text[:old_start] + new_existing + text[old_end:])

p = Path("internal/serve/uploads.go")
text = p.read_text()
text = text.replace("\timportLocationTags := make([][]string, 0, len(files))\n", "", 1)
text = text.replace("\tduplicateExistingHashes := make([]string, 0, len(files))\n\tduplicateExistingTags := make([][]string, 0, len(files))\n", "", 1)
text = text.replace('''\t\t\tif len(fileTags) > 0 {
\t\t\t\tduplicateExistingHashes = append(duplicateExistingHashes, existing.Hash)
\t\t\t\tduplicateExistingTags = append(duplicateExistingTags, append([]string(nil), fileTags...))
\t\t\t}
''', "", 1)
text = text.replace("\t\timportLocationTags = append(importLocationTags, append([]string(nil), fileTags...))\n", "", 1)
text = text.replace('''\tif len(duplicateExistingHashes) > 0 {
\t\tif _, err := l.client.TagExistingContentByHashTags(duplicateExistingHashes, duplicateExistingTags); err != nil {
\t\t\treturn UploadImportResponse{}, fmt.Errorf("tag duplicate uploads: %w", err)
\t\t}
\t}
\tfor _, duplicate := range duplicateExistingDiscards {
\t\tdiscardDuplicateUpload(duplicate.file, duplicate.trackedAtPath)
\t}
\tif len(importLocations) == 0 {
\t\treturn response, nil
\t}
''', "", 1)
text = text.replace(
    "\t\tresult, err = l.client.TagKnownFilesWithBackgroundTasksByFileTags(importLocations, importLocationTags, backgroundTasks, progress)\n",
    "\t\tresult, err = l.client.TagKnownFilesWithBackgroundTasksByHashTags(importLocations, tagsByHash, backgroundTasks, progress)\n",
    1,
)
text = text.replace(
    "\t\tresult, err = l.client.TagKnownFilesWithBackgroundTasksByFileTagsAndOperationState(importLocations, importLocationTags, backgroundTasks, func(affectedCount int) (core.BackgroundOperationTransactionState, error) {\n",
    "\t\tresult, err = l.client.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState(importLocations, tagsByHash, backgroundTasks, func(affectedCount int) (core.BackgroundOperationTransactionState, error) {\n",
    1,
)
marker = '''\tif err != nil {
\t\tvar restoreErr error
'''
insert = '''\tif err != nil {
\t\tvar restoreErr error
'''
if text.count(marker) != 1:
    raise SystemExit(f"internal/serve/uploads.go: err marker count {text.count(marker)}")
# Add discard only after the combined transaction succeeds, immediately after the error block.
error_end = '''\t\treturn UploadImportResponse{}, err
\t}
\tfor path, message := range failures {
'''
replacement = '''\t\treturn UploadImportResponse{}, err
\t}
\tfor _, duplicate := range duplicateExistingDiscards {
\t\tdiscardDuplicateUpload(duplicate.file, duplicate.trackedAtPath)
\t}
\tfor path, message := range failures {
'''
if text.count(error_end) != 1:
    raise SystemExit(f"internal/serve/uploads.go: error-end marker count {text.count(error_end)}")
text = text.replace(error_end, replacement, 1)
p.write_text(text)

p = Path("gooru/tagging_known_file_tags_test.go")
text = p.read_text()
text = text.replace('import (\n\t"testing"\n', 'import (\n\t"errors"\n\t"testing"\n', 1)
test_marker = "\nfunc sameStringSet(got, want []string) bool {"
new_test = '''
func TestTagKnownFilesByHashTagsRollsBackExistingAndNewContentTogether(t *testing.T) {
\tclient := newBackgroundEnqueueTestClient(t)
\texisting := types.LocationInfo{Path: "/library/existing.jpg", Hash: "hash-atomic-existing", Size: 11, ModTime: 21, Extension: ".jpg"}
\tif _, err := client.TagKnownFilesWithBackgroundTasksByFileTags([]types.LocationInfo{existing}, [][]string{{"seed"}}, nil, nil); err != nil {
\t\tt.Fatalf("seed existing content: %v", err)
\t}
\tfresh := types.LocationInfo{Path: "/library/fresh.jpg", Hash: "hash-atomic-fresh", Size: 12, ModTime: 22, Extension: ".jpg"}
\t_, err := client.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState(
\t\t[]types.LocationInfo{fresh},
\t\tmap[string][]string{existing.Hash: {"duplicate:tag"}, fresh.Hash: {"fresh:tag"}},
\t\tnil,
\t\tfunc(int) (BackgroundOperationTransactionState, error) {
\t\t\treturn BackgroundOperationTransactionState{}, errors.New("stop before commit")
\t\t},
\t\tnil,
\t)
\tif err == nil {
\t\tt.Fatal("expected state builder failure")
\t}
\ttags, err := client.store.GetTagsForContent(existing.Hash)
\tif err != nil {
\t\tt.Fatalf("GetTagsForContent(existing): %v", err)
\t}
\tif !sameStringSet(tags, []string{"seed"}) {
\t\tt.Fatalf("existing tags committed despite rollback: %#v", tags)
\t}
\texists, err := client.ContentExists(fresh.Hash)
\tif err != nil {
\t\tt.Fatalf("ContentExists(fresh): %v", err)
\t}
\tif exists {
\t\tt.Fatal("fresh content committed despite rollback")
\t}
}
'''
if text.count(test_marker) != 1:
    raise SystemExit("gooru/tagging_known_file_tags_test.go: sameStringSet marker missing")
text = text.replace(test_marker, new_test + test_marker, 1)
p.write_text(text)

p = Path("internal/serve/upload_identity_test.go")
text = p.read_text()
needle = '''\tif len(response.Files) != 3 {
\t\tt.Fatalf("upload response files = %d, want 3: %+v", len(response.Files), response.Files)
\t}
'''
addition = needle + '''\tif response.AffectedCount != 2 {
\t\tt.Fatalf("upload affected_count = %d, want 2 combined duplicate/new tag associations", response.AffectedCount)
\t}
'''
if text.count(needle) != 1:
    raise SystemExit("internal/serve/upload_identity_test.go: response length marker missing")
text = text.replace(needle, addition, 1)
p.write_text(text)
