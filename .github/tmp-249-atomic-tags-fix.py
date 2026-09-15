from pathlib import Path

p = Path("internal/serve/uploads.go")
text = p.read_text()
needle = "\t\tfileTags := tagsByHash[info.Hash]\n"
if text.count(needle) != 1:
    raise SystemExit(f"uploads.go: expected one obsolete fileTags line, found {text.count(needle)}")
p.write_text(text.replace(needle, "", 1))

p = Path("gooru/tagging_known_file_tags.go")
text = p.read_text()
start = text.index("func (c *Client) TagExistingContentByHashTags(")
end = text.index("\nfunc appendUniqueKnownFileTags", start)
p.write_text(text[:start] + text[end:])

p = Path("gooru/tagging_known_file_tags_test.go")
text = p.read_text()
start = text.index("func TestTagExistingContentByHashTagsAppliesDistinctTagsInOneBatch(")
end = text.index("func TestTagKnownFilesWithBackgroundTasksByFileTagsRejectsMisalignedMetadata", start)
p.write_text(text[:start] + text[end:])
