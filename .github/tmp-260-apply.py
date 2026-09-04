from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected exactly one match, found {count}")
    p.write_text(text.replace(old, new, 1))


Path('internal/database/migrations/007_media_page_count.up.sql').write_text(
    'ALTER TABLE media_metadata ADD COLUMN page_count INTEGER;\n'
)
Path('internal/database/migrations/007_media_page_count.down.sql').write_text(
    'ALTER TABLE media_metadata DROP COLUMN page_count;\n'
)

replace_once('types/types.go',
    '\tFrameCount      *int\n}',
    '\tFrameCount      *int\n\tPageCount       *int\n}')

replace_once('internal/database/database.go',
    '\t\t\tvideo_width, video_height, duration_seconds, frame_count, updated_at\n\t\t) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)',
    '\t\t\tvideo_width, video_height, duration_seconds, frame_count, page_count, updated_at\n\t\t) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)')
replace_once('internal/database/database.go',
    '\t\t\tframe_count=excluded.frame_count,\n\t\t\tupdated_at=CURRENT_TIMESTAMP\n\t`, meta.LocationID, meta.MediaKind, meta.MimeType, meta.ImageWidth, meta.ImageHeight, meta.VideoWidth, meta.VideoHeight, meta.DurationSeconds, meta.FrameCount)',
    '\t\t\tframe_count=excluded.frame_count,\n\t\t\tpage_count=excluded.page_count,\n\t\t\tupdated_at=CURRENT_TIMESTAMP\n\t`, meta.LocationID, meta.MediaKind, meta.MimeType, meta.ImageWidth, meta.ImageHeight, meta.VideoWidth, meta.VideoHeight, meta.DurationSeconds, meta.FrameCount, meta.PageCount)')
replace_once('internal/database/database.go',
    'SELECT media_kind, mime_type, image_width, image_height, video_width, video_height, duration_seconds, frame_count\n\t\tFROM media_metadata WHERE location_id = ?\n\t`, locationID).Scan(&meta.MediaKind, &meta.MimeType, &meta.ImageWidth, &meta.ImageHeight, &meta.VideoWidth, &meta.VideoHeight, &meta.DurationSeconds, &meta.FrameCount)',
    'SELECT media_kind, mime_type, image_width, image_height, video_width, video_height, duration_seconds, frame_count, page_count\n\t\tFROM media_metadata WHERE location_id = ?\n\t`, locationID).Scan(&meta.MediaKind, &meta.MimeType, &meta.ImageWidth, &meta.ImageHeight, &meta.VideoWidth, &meta.VideoHeight, &meta.DurationSeconds, &meta.FrameCount, &meta.PageCount)')
replace_once('internal/database/database.go',
    '\t\tmm.video_width, mm.video_height, mm.duration_seconds, mm.frame_count`',
    '\t\tmm.video_width, mm.video_height, mm.duration_seconds, mm.frame_count, mm.page_count`')
replace_once('internal/database/database.go',
    '\t\tvar imageWidth, imageHeight, videoWidth, videoHeight, frameCount sql.NullInt64\n\t\tvar duration sql.NullFloat64\n\t\tif err := rows.Scan(&file.ID, &file.PublicID, &file.Path, &file.Hash, &file.Size, &file.ModTime, &tagsCache, &mediaKind, &mimeType, &imageWidth, &imageHeight, &videoWidth, &videoHeight, &duration, &frameCount); err != nil {',
    '\t\tvar imageWidth, imageHeight, videoWidth, videoHeight, frameCount, pageCount sql.NullInt64\n\t\tvar duration sql.NullFloat64\n\t\tif err := rows.Scan(&file.ID, &file.PublicID, &file.Path, &file.Hash, &file.Size, &file.ModTime, &tagsCache, &mediaKind, &mimeType, &imageWidth, &imageHeight, &videoWidth, &videoHeight, &duration, &frameCount, &pageCount); err != nil {')
replace_once('internal/database/database.go',
    '\t\t\t\tFrameCount:      nullIntPtr(frameCount),\n\t\t\t}',
    '\t\t\t\tFrameCount:      nullIntPtr(frameCount),\n\t\t\t\tPageCount:       nullIntPtr(pageCount),\n\t\t\t}')

replace_once('internal/database/migration_test.go',
    'if version != 6 || dirty {\n\t\tt.Fatalf("schema_migrations = (%d, %t), want (6, false)", version, dirty)',
    'if version != 7 || dirty {\n\t\tt.Fatalf("schema_migrations = (%d, %t), want (7, false)", version, dirty)')

replace_once('internal/serve/metadata.go',
    '\t"os"\n\t"os/exec"',
    '\t"os"\n\t"os/exec"\n\t"path/filepath"')
replace_once('internal/serve/metadata.go',
    '\tFrameCount    *int     `json:"frame_count,omitempty"`\n}',
    '\tFrameCount    *int     `json:"frame_count,omitempty"`\n\tPageCount     *int     `json:"page_count,omitempty"`\n}')
replace_once('internal/serve/metadata.go',
    '\tif err := ctx.Err(); err != nil {\n\t\treturn MediaMetadata{}, err\n\t}\n\tswitch mediaKind {',
    '\tif err := ctx.Err(); err != nil {\n\t\treturn MediaMetadata{}, err\n\t}\n\tif strings.EqualFold(filepath.Ext(file.Path), ".cbz") {\n\t\tarchive, err := openComicArchive(file.Path)\n\t\tif err != nil {\n\t\t\treturn MediaMetadata{}, err\n\t\t}\n\t\tdefer archive.Close()\n\t\tcount := len(archive.pages)\n\t\treturn MediaMetadata{PageCount: &count}, nil\n\t}\n\tswitch mediaKind {')

p = Path('internal/serve/metadata.go')
text = p.read_text()
marker = 'func (p BasicMediaMetadataProvider) MetadataFromSource(ctx context.Context, file types.FileInfo, source io.ReaderAt, size int64, mediaType string, mediaKind string) (MediaMetadata, error) {'
start = text.index(marker)
tail = text[start:]
old = '\tif source == nil || size < 0 {\n\t\treturn MediaMetadata{}, nil\n\t}\n\treader := io.NewSectionReader(source, 0, size)'
if tail.count(old) != 1:
    raise SystemExit('metadata source guard anchor mismatch')
new = '\tif source == nil || size < 0 {\n\t\treturn MediaMetadata{}, nil\n\t}\n\tif strings.EqualFold(filepath.Ext(file.Path), ".cbz") {\n\t\tarchive, err := openComicArchiveReader(file.Path, source, size, nil)\n\t\tif err != nil {\n\t\t\treturn MediaMetadata{}, err\n\t\t}\n\t\tdefer archive.Close()\n\t\tcount := len(archive.pages)\n\t\treturn MediaMetadata{PageCount: &count}, nil\n\t}\n\treader := io.NewSectionReader(source, 0, size)'
p.write_text(text[:start] + tail.replace(old, new, 1))

replace_once('internal/serve/browse.go',
'''func (l *GooruLibrary) FileMetadata(ctx context.Context, locationID int64) (MediaMetadata, error) {
\tif err := ctx.Err(); err != nil {
\t\treturn MediaMetadata{}, err
\t}
\tmeta, err := l.client.GetMediaMetadata(locationID)
\tif errors.Is(err, sql.ErrNoRows) {
\t\treturn MediaMetadata{}, nil
\t}
\tif err != nil {
\t\treturn MediaMetadata{}, err
\t}
\treturn mediaMetadataDTO(meta), nil
}''',
'''func (l *GooruLibrary) FileMetadata(ctx context.Context, locationID int64) (MediaMetadata, error) {
\tif err := ctx.Err(); err != nil {
\t\treturn MediaMetadata{}, err
\t}
\tmeta, err := l.client.GetMediaMetadata(locationID)
\thaveStored := err == nil
\tif err != nil && !errors.Is(err, sql.ErrNoRows) {
\t\treturn MediaMetadata{}, err
\t}
\tif haveStored && meta.PageCount != nil {
\t\treturn mediaMetadataDTO(meta), nil
\t}

\tfile, fileErr := l.client.GetFileInfoByLocationID(locationID)
\tif fileErr != nil || !strings.EqualFold(filepath.Ext(file.Path), ".cbz") {
\t\tif haveStored {
\t\t\treturn mediaMetadataDTO(meta), nil
\t\t}
\t\treturn MediaMetadata{}, fileErr
\t}
\tmediaType := mediaTypeForPath(file.Path)
\tmediaKind := mediaKindForType(mediaType)
\tprovider := l.metadata
\tif provider == nil {
\t\tprovider = BasicMediaMetadataProvider{}
\t}
\tderived, deriveErr := l.importedMediaMetadata(ctx, provider, file, file.Path, mediaType, mediaKind)
\tif deriveErr != nil || derived.PageCount == nil {
\t\tif haveStored {
\t\t\treturn mediaMetadataDTO(meta), nil
\t\t}
\t\treturn MediaMetadata{}, nil
\t}
\tif !haveStored {
\t\tmeta = types.MediaMetadata{LocationID: locationID, MediaKind: mediaKind, MimeType: mediaType}
\t} else {
\t\tif meta.MediaKind == "" {
\t\t\tmeta.MediaKind = mediaKind
\t\t}
\t\tif meta.MimeType == "" {
\t\t\tmeta.MimeType = mediaType
\t\t}
\t}
\tmeta.PageCount = derived.PageCount
\tif err := l.client.UpsertMediaMetadata(meta); err != nil {
\t\treturn MediaMetadata{}, err
\t}
\treturn mediaMetadataDTO(meta), nil
}''')
replace_once('internal/serve/browse.go',
    '\t\tFrameCount:    meta.FrameCount,\n\t}',
    '\t\tFrameCount:    meta.FrameCount,\n\t\tPageCount:     meta.PageCount,\n\t}')
replace_once('internal/serve/browse.go',
    '\t}\n\treturn dto\n}\n\nfunc (s *Server) publicFileID',
    '\t}\n\tif strings.EqualFold(filepath.Ext(file.Path), ".cbz") && dto.Metadata.PageCount == nil {\n\t\tif search, ok := s.library.(SearchLibrary); ok {\n\t\t\tif metadata, err := search.FileMetadata(ctx, file.ID); err == nil {\n\t\t\t\tdto.Metadata = metadata\n\t\t\t}\n\t\t}\n\t}\n\treturn dto\n}\n\nfunc (s *Server) publicFileID')

replace_once('internal/serve/uploads.go',
    'metadata.VideoDuration == nil && metadata.FrameCount == nil {',
    'metadata.VideoDuration == nil && metadata.FrameCount == nil && metadata.PageCount == nil {')
replace_once('internal/serve/uploads.go',
    '\t\t\tFrameCount:      metadata.FrameCount,\n\t\t})',
    '\t\t\tFrameCount:      metadata.FrameCount,\n\t\t\tPageCount:       metadata.PageCount,\n\t\t})')

replace_once('docs/openapi.yaml',
    '        frame_count:\n          type: integer\n    MediaURLs:',
    '        frame_count:\n          type: integer\n        page_count:\n          type: integer\n          minimum: 1\n          description: Number of displayable pages for paged media such as CBZ comics.\n    MediaURLs:')

replace_once('frontend/src/lib/components/PreviewDialog.svelte',
    '<dt>Size</dt><dd>{mediaDimensions(file) || file.media_type} · {formatBytes(file.size)}</dd>',
    '<dt>Size</dt><dd>{file.metadata.page_count ? `${file.metadata.page_count} ${file.metadata.page_count === 1 ? \'page\' : \'pages\'} · ` : mediaDimensions(file) ? `${mediaDimensions(file)} · ` : \'\'}{formatBytes(file.size)}</dd>')
replace_once('frontend/src/lib/components/PreviewDialog.svelte',
    '\n      {#if comicManifest}<dt>Pages</dt><dd>{comicManifest.pages.length}</dd>{/if}',
    '')

replace_once('internal/serve/upload_cbz_e2e_test.go',
    '\t"context"\n\t"image/color"',
    '\t"context"\n\t"encoding/json"\n\t"image/color"')
replace_once('internal/serve/upload_cbz_e2e_test.go',
'''\tif file.PublicID == "" {
\t\tt.Fatal("uploaded comic has no public id")
\t}

\tmanifest := httptest.NewRecorder()''',
'''\tif file.PublicID == "" {
\t\tt.Fatal("uploaded comic has no public id")
\t}
\tif file.Metadata == nil || file.Metadata.PageCount == nil || *file.Metadata.PageCount != 1 {
\t\tt.Fatalf("uploaded comic page count = %+v, want 1", file.Metadata)
\t}
\tfileResponse := httptest.NewRecorder()
\tserver.Handler().ServeHTTP(fileResponse, httptest.NewRequest(http.MethodGet, "/api/v1/files/"+file.PublicID, nil))
\tif fileResponse.Code != http.StatusOK {
\t\tt.Fatalf("file metadata status = %d: %s", fileResponse.Code, fileResponse.Body.String())
\t}
\tvar fileDTO FileDTO
\tif err := json.Unmarshal(fileResponse.Body.Bytes(), &fileDTO); err != nil {
\t\tt.Fatalf("decode file metadata: %v", err)
\t}
\tif fileDTO.Metadata.PageCount == nil || *fileDTO.Metadata.PageCount != 1 {
\t\tt.Fatalf("file API page count = %+v, want 1", fileDTO.Metadata.PageCount)
\t}

\tmanifest := httptest.NewRecorder()''')

Path('frontend/tests/viewer-metadata.spec.ts').write_text('''import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: string, name: string, mediaType: string, size: number, metadata: Record<string, number> = {}) {
  return {
    id, content_id: `hash-${id}`, name, safe_display_path: `uploads/${name}`, size,
    modified_time: '2026-09-02T15:08:00Z', media_type: mediaType, media_kind: 'other', metadata, tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download`
    }, can_delete: false
  };
}

async function mockApp(page: Page) {
  const files = [
    fileItem('text', 'CONFIG.md', 'text/markdown; charset=utf-8', 7680),
    fileItem('comic', 'book.cbz', 'application/vnd.comicbook+zip', 2048, { page_count: 12 })
  ];
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401, contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  for (const path of ['jobs', 'saved-searches']) {
    await page.route(`**/api/v1/${path}`, async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  }
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], meta_tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files, total_count: 2, library_count: 2, facets: { kind: [] } }) }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' }));
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function openViewerFor(page: Page, name: string) {
  const card = page.locator('.thumb').filter({ has: page.getByAltText(name) });
  await card.dblclick();
  await expect(page.locator('.preview-dialog')).toBeVisible();
}

test('viewer size omits duplicate MIME for generic files and uses persisted CBZ page count', async ({ page }) => {
  await mockApp(page);
  await openViewerFor(page, 'CONFIG.md');
  const genericMeta = page.locator('.lightbox-meta');
  await expect(genericMeta.locator('dd').nth(1)).toHaveText('7.5 KB');
  await expect(genericMeta).toContainText('text/markdown; charset=utf-8');
  await page.keyboard.press('Escape');

  await openViewerFor(page, 'book.cbz');
  const comicMeta = page.locator('.lightbox-meta');
  await expect(comicMeta.locator('dd').nth(1)).toHaveText('12 pages · 2.0 KB');
  await expect(comicMeta.getByText('Pages', { exact: true })).toHaveCount(0);
});
''')
