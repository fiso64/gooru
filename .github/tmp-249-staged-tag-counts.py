from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))


Path("frontend/src/lib/state/uploadStagedTagCounts.ts").write_text('''export type UploadStagedTagCandidate = { name: string; count: number };\n\nfunction uniqueTagNames(tags: string[]): Map<string, string> {\n  const unique = new Map<string, string>();\n  for (const rawTag of tags) {\n    const name = rawTag.trim();\n    const key = name.toLowerCase();\n    if (name && !unique.has(key)) unique.set(key, name);\n  }\n  return unique;\n}\n\nexport function createUploadStagedTagCounts() {\n  const counts = new Map<string, UploadStagedTagCandidate>();\n\n  function add(tags: string[]) {\n    for (const [key, name] of uniqueTagNames(tags)) {\n      const current = counts.get(key);\n      if (current) current.count += 1;\n      else counts.set(key, { name, count: 1 });\n    }\n  }\n\n  function remove(tags: string[]) {\n    for (const key of uniqueTagNames(tags).keys()) {\n      const current = counts.get(key);\n      if (!current) continue;\n      if (current.count <= 1) counts.delete(key);\n      else current.count -= 1;\n    }\n  }\n\n  function replace(previousTags: string[], nextTags: string[]) {\n    const previous = uniqueTagNames(previousTags);\n    const next = uniqueTagNames(nextTags);\n    for (const [key, name] of previous) {\n      if (!next.has(key)) remove([name]);\n    }\n    for (const [key, name] of next) {\n      if (!previous.has(key)) add([name]);\n    }\n  }\n\n  return {\n    add,\n    remove,\n    replace,\n    clear: () => counts.clear(),\n    candidates: (): UploadStagedTagCandidate[] => Array.from(counts.values(), (candidate) => ({ ...candidate }))\n  };\n}\n''')

Path("frontend/src/lib/state/uploadStagedTagCounts.test.ts").write_text('''import { describe, expect, it } from 'vitest';\nimport { createUploadStagedTagCounts } from './uploadStagedTagCounts';\n\ndescribe('createUploadStagedTagCounts', () => {\n  it('counts each staged item once per case-insensitive tag', () => {\n    const counts = createUploadStagedTagCounts();\n    counts.add(['artist:Alice', 'artist:Alice']);\n    counts.add(['ARTIST:ALICE', 'local:only']);\n\n    expect(counts.candidates()).toEqual([\n      { name: 'artist:Alice', count: 2 },\n      { name: 'local:only', count: 1 }\n    ]);\n  });\n\n  it('updates only changed tag occurrences and removes zero-count candidates', () => {\n    const counts = createUploadStagedTagCounts();\n    counts.add(['shared', 'old']);\n    counts.add(['shared']);\n\n    counts.replace(['shared', 'old'], ['shared', 'new']);\n    expect(counts.candidates()).toEqual([\n      { name: 'shared', count: 2 },\n      { name: 'new', count: 1 }\n    ]);\n\n    counts.remove(['shared', 'new']);\n    expect(counts.candidates()).toEqual([{ name: 'shared', count: 1 }]);\n    counts.clear();\n    expect(counts.candidates()).toEqual([]);\n  });\n});\n''')

path = Path("frontend/src/lib/utils/tagSuggestions.ts")
text = path.read_text()
start = text.index("export function mergeTagCandidateCounts")
end = text.index("\nexport function plainTagSuggestions", start)
replacement = '''export function mergeTagCandidateOccurrenceCounts(candidates: TagCandidate[], stagedCandidates: TagCandidate[]): TagCandidate[] {\n  const merged = candidates.map((candidate) => ({ ...candidate }));\n  const indexByName = new Map<string, number>();\n  for (let index = 0; index < merged.length; index += 1) {\n    const name = candidateTagName(merged[index]!).trim();\n    if (name) indexByName.set(name.toLowerCase(), index);\n  }\n\n  for (const staged of stagedCandidates) {\n    const name = candidateTagName(staged).trim();\n    const count = Math.max(0, Math.trunc(staged.count ?? 0));\n    if (!name || count === 0) continue;\n    const key = name.toLowerCase();\n    const index = indexByName.get(key);\n    if (index == null) {\n      indexByName.set(key, merged.length);\n      merged.push({ name, count });\n      continue;\n    }\n    const candidate = merged[index]!;\n    merged[index] = { ...candidate, count: (candidate.count ?? 0) + count };\n  }\n\n  return merged;\n}\n\nexport function mergeTagCandidateCounts(candidates: TagCandidate[], tagSets: string[][]): TagCandidate[] {\n  const stagedCounts = new Map<string, { name: string; count: number }>();\n  for (const tags of tagSets) {\n    const seen = new Set<string>();\n    for (const rawTag of tags) {\n      const name = rawTag.trim();\n      const key = name.toLowerCase();\n      if (!name || seen.has(key)) continue;\n      seen.add(key);\n      const previous = stagedCounts.get(key);\n      stagedCounts.set(key, { name: previous?.name ?? name, count: (previous?.count ?? 0) + 1 });\n    }\n  }\n  return mergeTagCandidateOccurrenceCounts(candidates, [...stagedCounts.values()]);\n}\n'''
path.write_text(text[:start] + replacement + text[end:])

replace_once(
    "frontend/src/lib/utils/tagSuggestions.test.ts",
    "import { isPlainTag, mergeTagCandidateCounts, plainTagSuggestions, plainTagsFromInput } from './tagSuggestions';",
    "import { isPlainTag, mergeTagCandidateCounts, mergeTagCandidateOccurrenceCounts, plainTagSuggestions, plainTagsFromInput } from './tagSuggestions';",
)
replace_once(
    "frontend/src/lib/utils/tagSuggestions.test.ts",
    "});\n\ndescribe('plainTagSuggestions', () => {",
    """\n\n  it('merges pre-aggregated staged counts without rescanning staged rows', () => {\n    expect(mergeTagCandidateOccurrenceCounts(\n      [{ namespace: 'artist', value: 'alice', count: 8 }],\n      [{ name: 'ARTIST:ALICE', count: 2 }, { name: 'local:only', count: 3 }]\n    )).toEqual([\n      { namespace: 'artist', value: 'alice', count: 10 },\n      { name: 'local:only', count: 3 }\n    ]);\n  });\n});\n\ndescribe('plainTagSuggestions', () => {""",
)

replace_once(
    "frontend/src/lib/components/UploadPanel.svelte",
    "import { mergeTagCandidateCounts, type TagCandidate } from '$lib/utils/tagSuggestions';",
    "import { mergeTagCandidateOccurrenceCounts, type TagCandidate } from '$lib/utils/tagSuggestions';",
)
replace_once(
    "frontend/src/lib/components/UploadPanel.svelte",
    "    tags,\n    onTargetInput,",
    "    tags,\n    stagedTagCandidates,\n    onTargetInput,",
)
replace_once(
    "frontend/src/lib/components/UploadPanel.svelte",
    "    tags: TagCandidate[];\n    onTargetInput:",
    "    tags: TagCandidate[];\n    stagedTagCandidates: TagCandidate[];\n    onTargetInput:",
)
replace_once(
    "frontend/src/lib/components/UploadPanel.svelte",
    "  const completionTags = $derived(mergeTagCandidateCounts(tags, stagedRows.map((row) => row.item.tags ?? [])));",
    "  const completionTags = $derived(mergeTagCandidateOccurrenceCounts(tags, stagedTagCandidates));",
)

replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "        tags={tagsQuery.data?.tags ?? []}\n        onTargetInput={selectUploadTarget}",
    "        tags={tagsQuery.data?.tags ?? []}\n        stagedTagCandidates={upload.stagedTagCandidates}\n        onTargetInput={selectUploadTarget}",
)

replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "import { uploadJobStatusBatchSize } from '$lib/uploadBackpressure';",
    "import { uploadJobStatusBatchSize } from '$lib/uploadBackpressure';\nimport { createUploadStagedTagCounts } from './uploadStagedTagCounts';",
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "  let statusCounts: UploadStatusCounts = {};\n  let nextBatchID = 0;",
    """  let statusCounts: UploadStatusCounts = {};\n  let nextBatchID = 0;\n  const stagedTagCounts = createUploadStagedTagCounts();\n  let stagedTagCandidates = $state(stagedTagCounts.candidates());\n\n  function refreshStagedTagCandidates() {\n    stagedTagCandidates = stagedTagCounts.candidates();\n  }\n\n  function clearStagedTagCandidates() {\n    stagedTagCounts.clear();\n    stagedTagCandidates = [];\n  }""",
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "    nextBatchID = 0;\n  }",
    "    nextBatchID = 0;\n    clearStagedTagCandidates();\n  }",
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "      statusCounts = {};\n      return;",
    "      statusCounts = {};\n      clearStagedTagCandidates();\n      return;",
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "    if (scope === 'staged') {\n      files = [];\n      items = items.filter((item) => item.status !== 'staged');",
    "    if (scope === 'staged') {\n      files = [];\n      items = items.filter((item) => item.status !== 'staged');\n      clearStagedTagCandidates();",
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "  function removeAt(index: number) {\n    const stagedIndices = items.flatMap((item, itemIndex) => item.status === 'staged' ? [itemIndex] : []);",
    """  function removeAt(index: number) {\n    const removedItem = items[index];\n    if (removedItem?.status === 'staged') {\n      stagedTagCounts.remove(removedItem.tags ?? []);\n      refreshStagedTagCandidates();\n    }\n    const stagedIndices = items.flatMap((item, itemIndex) => item.status === 'staged' ? [itemIndex] : []);""",
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "  function setItemTags(index: number, nextTags: string[]) {\n    setUploadItemTagsInPlace(items, index, nextTags);\n  }",
    """  function setItemTags(index: number, nextTags: string[]) {\n    const current = items[index];\n    const previousStagedTags = current?.status === 'staged' ? [...(current.tags ?? [])] : undefined;\n    setUploadItemTagsInPlace(items, index, nextTags);\n    if (previousStagedTags && current) {\n      stagedTagCounts.replace(previousStagedTags, current.tags ?? []);\n      refreshStagedTagCandidates();\n    }\n  }""",
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "    files = [...files, ...additions];\n    items = [...items, ...stagedUploadItems(additions, targetID, queueTimeMs, parseTags(tags))];",
    """    const stagedAdditions = stagedUploadItems(additions, targetID, queueTimeMs, parseTags(tags));\n    files = [...files, ...additions];\n    items = [...items, ...stagedAdditions];\n    for (const item of stagedAdditions) stagedTagCounts.add(item.tags ?? []);\n    refreshStagedTagCandidates();""",
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "    items = nextItems;\n    files = [];",
    "    items = nextItems;\n    files = [];\n    clearStagedTagCandidates();",
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "    get tags() { return tags; },\n    set tags(value: string) { tags = value; },",
    "    get tags() { return tags; },\n    set tags(value: string) { tags = value; },\n    get stagedTagCandidates() { return stagedTagCandidates; },",
)

path = Path("frontend/src/lib/state/uploadWorkflow.svelte.test.ts")
text = path.read_text()
insert = """\n\n  it('maintains staged completion counts incrementally through edits, removal, and admission', async () => {\n    const workflow = createUploadWorkflow();\n    workflow.tags = 'shared';\n    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);\n    expect(workflow.stagedTagCandidates).toEqual([{ name: 'shared', count: 2 }]);\n\n    workflow.setItemTags(0, ['shared', 'local:first']);\n    expect(workflow.stagedTagCandidates).toEqual([\n      { name: 'shared', count: 2 },\n      { name: 'local:first', count: 1 }\n    ]);\n\n    workflow.removeAt(1);\n    expect(workflow.stagedTagCandidates).toEqual([\n      { name: 'shared', count: 1 },\n      { name: 'local:first', count: 1 }\n    ]);\n\n    await workflow.submit(async () => pendingJob('job-staged-counts', 0, 1));\n    expect(workflow.stagedTagCandidates).toEqual([]);\n  });\n"""
marker = "\n  it('submits one durable operation with per-file queue metadata arrays', async () => {"
if text.count(marker) != 1:
    raise SystemExit("uploadWorkflow.svelte.test.ts: marker mismatch")
path.write_text(text.replace(marker, insert + marker, 1))
