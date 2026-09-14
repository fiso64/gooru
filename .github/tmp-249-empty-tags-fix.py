from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if text.count(old) != 1:
        raise SystemExit(f"expected exactly one match in {path}, found {text.count(old)}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "internal/serve/uploads.go",
    '''func cloneUploadTags(tags *[]string) *[]string {
\tif tags == nil {
\t\treturn nil
\t}
\tcopyTags := append([]string(nil), (*tags)...)
\treturn &copyTags
}
''',
    '''func cloneUploadTags(tags *[]string) *[]string {
\tif tags == nil {
\t\treturn nil
\t}
\tcopyTags := make([]string, len(*tags))
\tcopy(copyTags, *tags)
\treturn &copyTags
}
''',
)

replace_once(
    "internal/serve/upload_item_tags_test.go",
    '''func TestBackgroundUploadTaskPreservesExplicitEmptyItemTags(t *testing.T) {
\temptyTags := []string{}
\trequest, err := backgroundUploadTaskRequest("operation-item-tags", []savedUpload{{
\t\tname: "empty.jpg", path: "/tmp/empty.jpg", targetID: "default", tags: &emptyTags,
\t}}, []string{"fallback"})
\tif err != nil {
\t\tt.Fatalf("build background upload task: %v", err)
\t}
\tvar input backgroundUploadTaskInput
\tif err := json.Unmarshal([]byte(request.InputKey), &input); err != nil {
\t\tt.Fatalf("decode background upload task JSON: %v", err)
\t}
\tif len(input.Files) != 1 || input.Files[0].Tags == nil || len(*input.Files[0].Tags) != 0 {
\t\tt.Fatalf("explicit empty item tags did not round-trip: %#v", input.Files)
\t}
}
''',
    '''func TestBackgroundUploadTaskPreservesExplicitEmptyItemTags(t *testing.T) {
\temptyTags := []string{}
\trequest, err := backgroundUploadTaskRequest("operation-item-tags", []savedUpload{{
\t\tname: "empty.jpg", path: "/tmp/empty.jpg", targetID: "default", tags: &emptyTags,
\t}}, []string{"fallback"})
\tif err != nil {
\t\tt.Fatalf("build background upload task: %v", err)
\t}

\tvar raw struct {
\t\tFiles []struct {
\t\t\tTags json.RawMessage `json:"tags"`
\t\t} `json:"files"`
\t}
\tif err := json.Unmarshal([]byte(request.InputKey), &raw); err != nil {
\t\tt.Fatalf("decode raw background upload task JSON: %v", err)
\t}
\tif len(raw.Files) != 1 || string(raw.Files[0].Tags) != "[]" {
\t\tt.Fatalf("explicit empty item tags durable JSON = %q; want []", raw.Files[0].Tags)
\t}

\tvar input backgroundUploadTaskInput
\tif err := json.Unmarshal([]byte(request.InputKey), &input); err != nil {
\t\tt.Fatalf("decode background upload task JSON: %v", err)
\t}
\tif len(input.Files) != 1 || input.Files[0].Tags == nil || len(*input.Files[0].Tags) != 0 {
\t\tt.Fatalf("explicit empty item tags did not round-trip: %#v", input.Files)
\t}
}
''',
)
