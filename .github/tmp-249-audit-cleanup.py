from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    '''export interface UploadSubmissionSegment {\n  start: number;\n  end: number;\n  tags: string[];\n}\n\nexport function uploadSubmissionSegments(\n  itemIndices: number[],\n  items: UploadItem[],\n  fallbackTags: string[],\n  maxFiles: number\n): UploadSubmissionSegment[] {\n  if (!itemIndices.length) return [];\n  const limit = Math.max(1, Math.trunc(maxFiles));\n  const firstTags = [...(items[itemIndices[0]]?.tags ?? fallbackTags)];\n  const commonTags = [...firstTags];\n\n  for (let index = 1; index < itemIndices.length && commonTags.length; index += 1) {\n    const tags = new Set(items[itemIndices[index]]?.tags ?? fallbackTags);\n    let writeIndex = 0;\n    for (const tag of commonTags) {\n      if (tags.has(tag)) commonTags[writeIndex++] = tag;\n    }\n    commonTags.length = writeIndex;\n  }\n\n  const segments: UploadSubmissionSegment[] = [];\n  for (let start = 0; start < itemIndices.length; start += limit) {\n    segments.push({ start, end: Math.min(itemIndices.length, start + limit), tags: [...commonTags] });\n  }\n  return segments;\n}\n''',
    '''export interface UploadSubmissionSegment {\n  start: number;\n  end: number;\n}\n\nexport function uploadSubmissionSegments(itemCount: number, maxFiles: number): UploadSubmissionSegment[] {\n  const count = Math.max(0, Math.trunc(itemCount));\n  if (!count) return [];\n  const limit = Math.max(1, Math.trunc(maxFiles));\n  const segments: UploadSubmissionSegment[] = [];\n  for (let start = 0; start < count; start += limit) {\n    segments.push({ start, end: Math.min(count, start + limit) });\n  }\n  return segments;\n}\n''',
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    '''    const segments = uploadSubmissionSegments(batchItemIndices, items, parsedTags, multipartUploadChunkSize());\n''',
    '''    const segments = uploadSubmissionSegments(batchItemIndices.length, multipartUploadChunkSize());\n''',
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    '''          tags: segment.tags,\n''',
    '''          tags: parsedTags,\n''',
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.chunking.test.ts",
    '''        tags: ['common'],\n''',
    '''        tags: ['project:inbox', 'common'],\n''',
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.chunking.test.ts",
    '''    expect(workflow.activeJobIDs).toEqual(['job-logical']);\n    expect(workflow.items.map((item) => item.status)).toEqual(Array.from({ length: 2001 }, () => 'queued'));\n  });\n\n  it('keeps differing item tags in one request with aligned native per-file tags', async () => {\n''',
    '''    expect(workflow.activeJobIDs).toEqual(['job-logical']);\n    expect(workflow.items.map((item) => item.status)).toEqual(Array.from({ length: 2001 }, () => 'queued'));\n  });\n\n  it('keeps per-file tags aligned across a multipart segmentation boundary', async () => {\n    const workflow = createUploadWorkflow();\n    const files = Array.from({ length: maxFilesPerMultipartUpload + 1 }, (_, index) => uploadFile(index));\n    workflow.tags = 'fallback';\n    workflow.select(files);\n    workflow.setItemTags(maxFilesPerMultipartUpload - 1, ['edge:left']);\n    workflow.setItemTags(maxFilesPerMultipartUpload, ['edge:right']);\n    const calls: Array<{ names: string[]; itemTags: string[][] }> = [];\n\n    await workflow.submit(async (variables) => {\n      calls.push({\n        names: variables.files.map((file) => file.name),\n        itemTags: variables.itemTags?.map((itemTags) => [...itemTags]) ?? []\n      });\n      return pendingJob('job-boundary-tags', 2);\n    });\n\n    expect(calls.map((call) => call.names.length)).toEqual([maxFilesPerMultipartUpload, 1]);\n    expect(calls[0]?.names[maxFilesPerMultipartUpload - 1]).toBe(`file-${maxFilesPerMultipartUpload - 1}.jpg`);\n    expect(calls[0]?.itemTags[maxFilesPerMultipartUpload - 1]).toEqual(['edge:left']);\n    expect(calls[1]?.names[0]).toBe(`file-${maxFilesPerMultipartUpload}.jpg`);\n    expect(calls[1]?.itemTags[0]).toEqual(['edge:right']);\n  });\n\n  it('keeps differing item tags in one request with aligned native per-file tags', async () => {\n''',
)

path = Path("gooru/tagging_known_file_tags.go")
text = path.read_text()
start = text.index("// TagKnownFilesWithBackgroundTasksByFileTags atomically registers known files")
end = text.index("// TagKnownFilesWithBackgroundTasksByHashTags registers the supplied known files")
text = text[:start] + text[end:]
stale_start = text.index("// TagExistingContentByHashTags atomically adds aligned per-content tag sets")
append_start = text.index("func appendUniqueKnownFileTags", stale_start)
append_end = text.index("func (c *Client) associateKnownFileTagsByHash", append_start)
text = text[:stale_start] + text[append_end:]
path.write_text(text)

path = Path("gooru/tagging_known_file_tags_test.go")
text = path.read_text()
first = text.index("func TestTagKnownFilesWithBackgroundTasksByFileTagsAppliesAlignedTagsAtomically")
third = text.index("func TestTagKnownFilesByHashTagsRollsBackExistingAndNewContentTogether")
text = text[:first] + text[third:]
text = text.replace(
    '''\tif _, err := client.TagKnownFilesWithBackgroundTasksByFileTags([]types.LocationInfo{existing}, [][]string{{"seed"}}, nil, nil); err != nil {\n\t\tt.Fatalf("seed existing content: %v", err)\n\t}\n''',
    '''\tif _, err := client.TagKnownFilesWithBackgroundTasksByHashTags([]types.LocationInfo{existing}, map[string][]string{existing.Hash: {"seed"}}, nil, nil); err != nil {\n\t\tt.Fatalf("seed existing content: %v", err)\n\t}\n''',
    1,
)
path.write_text(text)

css = Path("frontend/src/app.css")
css.write_text(css.read_text().rstrip("\n") + "\n")
