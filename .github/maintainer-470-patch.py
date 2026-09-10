from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected exactly one occurrence, found {count}: {old!r}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "internal/serve/browse.go",
    '\tMediaKind       string        `json:"media_kind"`\n\tMetadata        MediaMetadata `json:"metadata,omitempty"`',
    '\tMediaKind       string        `json:"media_kind"`\n\tViewerSupport   string        `json:"viewer_support"`\n\tMetadata        MediaMetadata `json:"metadata,omitempty"`',
)
replace_once(
    "internal/serve/browse.go",
    '\tif strings.EqualFold(filepath.Ext(file.Path), ".cbz") && (dto.Metadata.PageCount == nil || dto.Metadata.ImageWidth == nil || dto.Metadata.ImageHeight == nil) {\n\t\tif search, ok := s.library.(SearchLibrary); ok {\n\t\t\tif metadata, err := search.FileMetadata(ctx, file.ID); err == nil {\n\t\t\t\tdto.Metadata = metadata\n\t\t\t}\n\t\t}\n\t}\n\treturn dto\n}',
    '\tif strings.EqualFold(filepath.Ext(file.Path), ".cbz") && (dto.Metadata.PageCount == nil || dto.Metadata.ImageWidth == nil || dto.Metadata.ImageHeight == nil) {\n\t\tif search, ok := s.library.(SearchLibrary); ok {\n\t\t\tif metadata, err := search.FileMetadata(ctx, file.ID); err == nil {\n\t\t\t\tdto.Metadata = metadata\n\t\t\t}\n\t\t}\n\t}\n\tdto.ViewerSupport = viewerSupportForMediaKind(dto.MediaKind)\n\treturn dto\n}',
)
replace_once(
    "internal/serve/browse.go",
    '\tdefault:\n\t\treturn "other"\n\t}\n}\n\nfunc nonNilStrings',
    '\tdefault:\n\t\treturn "other"\n\t}\n}\n\nfunc viewerSupportForMediaKind(mediaKind string) string {\n\tswitch strings.ToLower(strings.TrimSpace(mediaKind)) {\n\tcase "photo", "gif", "video", "audio", "comic":\n\t\treturn "supported"\n\tdefault:\n\t\treturn "unsupported_media_type"\n\t}\n}\n\nfunc nonNilStrings',
)
replace_once(
    "internal/serve/browse_test.go",
    'func TestBrowseRoutesRequireSession(t *testing.T) {',
    '''func TestFileDTOReportsViewerSupportFromBackendMediaKind(t *testing.T) {
\tserver := &Server{cfg: DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))}
\timage := server.fileDTO(context.Background(), types.FileInfo{Path: filepath.Join(t.TempDir(), "image.jpg")}, false)
\tif image.MediaKind != "photo" || image.ViewerSupport != "supported" {
\t\tt.Fatalf("expected supported photo viewer state, got kind=%q support=%q", image.MediaKind, image.ViewerSupport)
\t}
\ttext := server.fileDTO(context.Background(), types.FileInfo{Path: filepath.Join(t.TempDir(), "notes.txt")}, false)
\tif text.MediaKind != "other" || text.ViewerSupport != "unsupported_media_type" {
\t\tt.Fatalf("expected unsupported text viewer state, got kind=%q support=%q", text.MediaKind, text.ViewerSupport)
\t}
}

func TestBrowseRoutesRequireSession(t *testing.T) {''',
)
replace_once(
    "docs/openapi.yaml",
    '        - media_kind\n        - metadata',
    '        - media_kind\n        - viewer_support\n        - metadata',
)
replace_once(
    "docs/openapi.yaml",
    '        media_kind:\n          type: string\n          enum: [photo, video, gif, audio, other]\n        metadata:',
    '        media_kind:\n          type: string\n          enum: [photo, video, gif, audio, comic, other]\n        viewer_support:\n          type: string\n          enum: [supported, unsupported_media_type]\n          description: Backend-owned indication of whether the built-in viewer supports this media type.\n        metadata:',
)

p = Path("frontend/src/lib/components/ViewerStage.svelte")
text = p.read_text()
old = "  function clearWaitingTimer() {"
new = '''  function viewerSupportsFile(target: FileItem) {
    return target.viewer_support === 'supported';
  }

  function unsupportedViewerMessage(target: FileItem) {
    return `No viewer is available for this file type (${target.media_type}).`;
  }

  function clearWaitingTimer() {'''
if text.count(old) != 1:
    raise SystemExit("ViewerStage: helper insertion mismatch")
text = text.replace(old, new, 1)
old = "    mediaError = '';\n\n    if (!displayedFile) {\n      displayedFile = targetFile;\n      displayedImageSource = targetImageSource;\n      armWaitingTimer(generation);\n      return () => { if (generation === transitionGeneration) clearWaitingTimer(); };\n    }"
new = "    mediaError = viewerSupportsFile(targetFile) ? '' : unsupportedViewerMessage(targetFile);\n\n    if (!displayedFile) {\n      displayedFile = targetFile;\n      displayedImageSource = targetImageSource;\n      if (!viewerSupportsFile(targetFile)) {\n        freezeGeneration += 1;\n        freezeVisible = false;\n        intrinsicWidth = 0;\n        intrinsicHeight = 0;\n        return;\n      }\n      armWaitingTimer(generation);\n      return () => { if (generation === transitionGeneration) clearWaitingTimer(); };\n    }"
if text.count(old) != 1:
    raise SystemExit("ViewerStage: initial transition mismatch")
text = text.replace(old, new, 1)
replace_a = "    const rendersImage = targetFile.media_kind !== 'video' && targetFile.media_kind !== 'audio' && !targetFile.media_type.startsWith('audio/');"
replace_b = "    const rendersImage = viewerSupportsFile(targetFile) && targetFile.media_kind !== 'video' && targetFile.media_kind !== 'audio' && !targetFile.media_type.startsWith('audio/');"
if text.count(replace_a) != 1:
    raise SystemExit("ViewerStage: target image predicate mismatch")
text = text.replace(replace_a, replace_b, 1)
old = "    displayedFile = targetFile;\n    displayedImageSource = targetImageSource;\n    armWaitingTimer(generation);"
new = "    displayedFile = targetFile;\n    displayedImageSource = targetImageSource;\n    if (!viewerSupportsFile(targetFile)) {\n      clearWaitingTimer();\n      waitingForTarget = false;\n      freezeGeneration += 1;\n      freezeVisible = false;\n      intrinsicWidth = 0;\n      intrinsicHeight = 0;\n      return;\n    }\n    armWaitingTimer(generation);"
if text.count(old) != 1:
    raise SystemExit("ViewerStage: target install mismatch")
text = text.replace(old, new, 1)
replace_a = "    const rendersImage = nextFile.media_kind !== 'video' && nextFile.media_kind !== 'audio' && !nextFile.media_type.startsWith('audio/');"
replace_b = "    const rendersImage = viewerSupportsFile(nextFile) && nextFile.media_kind !== 'video' && nextFile.media_kind !== 'audio' && !nextFile.media_type.startsWith('audio/');"
if text.count(replace_a) != 1:
    raise SystemExit("ViewerStage: rendered image predicate mismatch")
text = text.replace(replace_a, replace_b, 1)
old = "      {#if renderedFile.media_kind === 'video'}\n        <!-- svelte-ignore a11y_media_has_caption -->"
new = "      {#if renderedFile.viewer_support !== 'unsupported_media_type'}\n      {#if renderedFile.media_kind === 'video'}\n        <!-- svelte-ignore a11y_media_has_caption -->"
if text.count(old) != 1:
    raise SystemExit("ViewerStage: media block opening mismatch")
text = text.replace(old, new, 1)
old = "        />\n      {/if}\n    </div>"
new = "        />\n      {/if}\n      {/if}\n    </div>"
if text.count(old) != 1:
    raise SystemExit("ViewerStage: media block ending mismatch")
text = text.replace(old, new, 1)
p.write_text(text)

p = Path("frontend/tests/viewer-image-geometry-hold.spec.ts")
text = p.read_text()
text = text.replace(
    "function fileItem(id: 'portrait' | 'landscape' | 'text') {\n  const portrait = id === 'portrait';\n  const text = id === 'text';",
    "function fileItem(id: 'portrait' | 'landscape' | 'missing' | 'text') {\n  const portrait = id === 'portrait';\n  const text = id === 'text';",
    1,
)
text = text.replace(
    "    media_kind: text ? 'document' : 'photo',",
    "    media_kind: text ? 'other' : 'photo',\n    viewer_support: text ? 'unsupported_media_type' : 'supported',",
    1,
)
text = text.replace(
    "  const files = [fileItem('portrait'), fileItem('landscape'), fileItem('text')];\n  let loggedIn = false;",
    "  const files = [fileItem('portrait'), fileItem('landscape'), fileItem('missing'), fileItem('text')];\n  let loggedIn = false;\n  let unsupportedMediaRequests = 0;",
    1,
)
text = text.replace(
    "    if (route.request().url().includes('/text/')) {\n      await route.fulfill({ status: 404, body: '' });\n      return;\n    }",
    "    if (route.request().url().includes('/text/')) {\n      unsupportedMediaRequests += 1;\n      await route.fulfill({ contentType: 'text/plain', body: 'present text file' });\n      return;\n    }\n    if (route.request().url().includes('/missing/')) {\n      await route.fulfill({ status: 404, body: '' });\n      return;\n    }",
    1,
)
text = text.replace(
    "  await page.route('**/api/v1/files/*/content', async (route) => route.fulfill({ status: 404, body: '' }));",
    "  await page.route('**/api/v1/files/*/content', async (route) => {\n    if (route.request().url().includes('/text/')) {\n      unsupportedMediaRequests += 1;\n      await route.fulfill({ contentType: 'text/plain', body: 'present text file' });\n      return;\n    }\n    await route.fulfill({ status: 404, body: '' });\n  });",
    1,
)
text = text.replace(
    "  return { releaseLandscape };",
    "  return { releaseLandscape, unsupportedMediaRequestCount: () => unsupportedMediaRequests };",
    1,
)
text = text.replace(
    "test('failed non-image preview releases the frozen previous presentation and reports missing media'",
    "test('failed supported image preview releases the frozen previous presentation and reports missing media'",
    1,
)
text = text.replace(
    "  await expect(page.getByRole('dialog', { name: 'text.txt' })).toBeVisible();",
    "  await expect(page.getByRole('dialog', { name: 'missing.jpg' })).toBeVisible();",
    1,
)
text += '''

test('present unsupported file reports viewer capability without attempting media load', async ({ page }) => {
  const { unsupportedMediaRequestCount } = await mockApp(page);
  await page.getByRole('button', { name: 'Preview text.txt' }).click();

  await expect(page.getByRole('dialog', { name: 'text.txt' })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveText('No viewer is available for this file type (text/plain).');
  await expect(page.locator('.viewer-visual-media')).toHaveCount(0);
  expect(unsupportedMediaRequestCount()).toBe(0);
});
'''
p.write_text(text)
