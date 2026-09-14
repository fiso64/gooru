#!/usr/bin/env bash
set -euo pipefail

python3 - <<'PY'
from pathlib import Path

def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))

path = "gooru/tagging_known_file_tags.go"
replace_once(path,
'''func appendUniqueKnownFileTags(existing, additions []string) []string {
''',
'''// TagExistingContentByHashTags atomically adds aligned per-content tag sets
// without re-reading or re-hashing filesystem paths that the caller has already
// resolved to tracked content.
func (c *Client) TagExistingContentByHashTags(hashes []string, fileTags [][]string) (types.TagOperationResult, error) {
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
\tif len(tagsByHash) == 0 {
\t\treturn result, nil
\t}
\ttx, err := c.store.Begin()
\tif err != nil {
\t\treturn result, err
\t}
\tdefer tx.Rollback()
\taffectedCount, err := c.associateKnownFileTagsByHash(tx, tagsByHash)
\tif err != nil {
\t\treturn result, err
\t}
\tif err := tx.Commit(); err != nil {
\t\treturn result, err
\t}
\tresult.AffectedCount = int(affectedCount)
\treturn result, nil
}

func appendUniqueKnownFileTags(existing, additions []string) []string {
''')

path = "gooru/tagging_known_file_tags_test.go"
replace_once(path,
'''func TestTagKnownFilesWithBackgroundTasksByFileTagsRejectsMisalignedMetadata(t *testing.T) {
''',
'''func TestTagExistingContentByHashTagsAppliesDistinctTagsInOneBatch(t *testing.T) {
\tclient := newBackgroundEnqueueTestClient(t)
\tfiles := []types.LocationInfo{
\t\t{Path: "/library/existing-one.jpg", Hash: "hash-existing-one", Size: 11, ModTime: 21, Extension: ".jpg"},
\t\t{Path: "/library/existing-two.jpg", Hash: "hash-existing-two", Size: 12, ModTime: 22, Extension: ".jpg"},
\t}
\tif _, err := client.TagKnownFilesWithBackgroundTasksByFileTags(files, [][]string{{"seed"}, {"seed"}}, nil, nil); err != nil {
\t\tt.Fatalf("seed existing content: %v", err)
\t}
\tresult, err := client.TagExistingContentByHashTags(
\t\t[]string{files[0].Hash, files[1].Hash, files[0].Hash},
\t\t[][]string{{"shared", "item:one"}, {"shared", "item:two"}, {"later"}},
\t)
\tif err != nil {
\t\tt.Fatalf("TagExistingContentByHashTags: %v", err)
\t}
\tif result.AffectedCount != 5 {
\t\tt.Fatalf("affected count = %d, want 5", result.AffectedCount)
\t}
\toneTags, err := client.store.GetTagsForContent(files[0].Hash)
\tif err != nil {
\t\tt.Fatalf("GetTagsForContent(one): %v", err)
\t}
\ttwoTags, err := client.store.GetTagsForContent(files[1].Hash)
\tif err != nil {
\t\tt.Fatalf("GetTagsForContent(two): %v", err)
\t}
\tif !sameStringSet(oneTags, []string{"seed", "shared", "item:one", "later"}) {
\t\tt.Fatalf("one tags = %#v", oneTags)
\t}
\tif !sameStringSet(twoTags, []string{"seed", "shared", "item:two"}) {
\t\tt.Fatalf("two tags = %#v", twoTags)
\t}
}

func TestTagKnownFilesWithBackgroundTasksByFileTagsRejectsMisalignedMetadata(t *testing.T) {
''')

path = "internal/serve/uploads.go"
replace_once(path,
'''\topaqueStorageByLogical := make(map[string]protectedUploadMove, len(files))
''',
'''\topaqueStorageByLogical := make(map[string]protectedUploadMove, len(files))
\tduplicateExistingHashes := make([]string, 0, len(files))
\tduplicateExistingTags := make([][]string, 0, len(files))
\tduplicateExistingDiscards := make([]struct {
\t\tfile          StagedUpload
\t\ttrackedAtPath bool
\t}, 0, len(files))
''')
replace_once(path,
'''\t\tdto.Status = "duplicate_existing"
\t\t\tif len(fileTags) > 0 {
\t\t\t\tif _, err := l.mutateTagPaths(TagOperationAdd, []string{existing.Path}, fileTags); err != nil {
\t\t\t\t\treturn UploadImportResponse{}, fmt.Errorf("tag duplicate upload %q: %w", file.Name, err)
\t\t\t\t}
\t\t\t}
\t\t\ttrackedAtPath := status == types.StatusOK && !(l.encryption.Enabled && IsManagedUploadPath(l.managedTargets, file.Path))
\t\t\tdiscardDuplicateUpload(file, trackedAtPath)
\t\t\tresponse.Files = append(response.Files, dto)
''',
'''\t\tdto.Status = "duplicate_existing"
\t\t\tif len(fileTags) > 0 {
\t\t\t\tduplicateExistingHashes = append(duplicateExistingHashes, existing.Hash)
\t\t\t\tduplicateExistingTags = append(duplicateExistingTags, append([]string(nil), fileTags...))
\t\t\t}
\t\t\ttrackedAtPath := status == types.StatusOK && !(l.encryption.Enabled && IsManagedUploadPath(l.managedTargets, file.Path))
\t\t\tduplicateExistingDiscards = append(duplicateExistingDiscards, struct {
\t\t\t\tfile          StagedUpload
\t\t\t\ttrackedAtPath bool
\t\t\t}{file: file, trackedAtPath: trackedAtPath})
\t\t\tresponse.Files = append(response.Files, dto)
''')
replace_once(path,
'''\tif len(importLocations) == 0 {
\t\treturn response, nil
\t}
''',
'''\tif len(duplicateExistingHashes) > 0 {
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
''')
PY

gofmt -w gooru/tagging_known_file_tags.go gooru/tagging_known_file_tags_test.go internal/serve/uploads.go
git diff --check
go test -count=1 ./...

git config user.name "gooru-maintainer-bot"
git config user.email "maintainer@localhost"
git add gooru/tagging_known_file_tags.go gooru/tagging_known_file_tags_test.go internal/serve/uploads.go
git commit -m 'perf: batch duplicate upload tag mutations'
git push origin HEAD:feat/249-upload-item-tags
