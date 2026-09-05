from pathlib import Path

panel = Path('frontend/src/lib/components/UploadPanel.svelte')
text = panel.read_text()

if "import UploadMediaPreview from './UploadMediaPreview.svelte';" in text:
    raise SystemExit(0)

text = text.replace("  import { onDestroy } from 'svelte';\n", "")
text = text.replace(
    "  import UploadTargetPicker from './UploadTargetPicker.svelte';\n",
    "  import UploadTargetPicker from './UploadTargetPicker.svelte';\n  import UploadMediaPreview from './UploadMediaPreview.svelte';\n",
)

start = text.index("  let tagDraft = $state('');")
end = text.index("\n  function chooseFiles()", start)
replacement = '''  let tagDraft = $state('');
  let stagedPage = $state(0);
  let queuePage = $state(0);

  type IndexedUploadRow = { item: UploadItem; index: number };
  const uploadListPageSize = 100;
  const indexedItems = $derived(uploadItems.map((item: UploadItem, index: number) => ({ item, index })));
  const stagedRows = $derived(indexedItems.filter((row: IndexedUploadRow) => row.item.status === 'staged'));
  const queueRows = $derived(indexedItems.filter((row: IndexedUploadRow) => row.item.status !== 'staged'));
  const stagedBytes = $derived(uploadFiles.reduce((sum: number, file: File) => sum + file.size, 0));
  const queueBytes = $derived(queueRows.reduce((sum: number, row: IndexedUploadRow) => sum + row.item.size, 0));
  const initialTags = $derived(parseTags(uploadTags));
  const selectedTargetID = $derived(effectiveUploadTargetID(targetID, targets));
  const stagedPageCount = $derived(Math.max(1, Math.ceil(stagedRows.length / uploadListPageSize)));
  const queuePageCount = $derived(Math.max(1, Math.ceil(queueRows.length / uploadListPageSize)));
  const stagedVisibleRows = $derived.by(() => pageRows(stagedRows, stagedPage));
  const queueVisibleRows = $derived.by(() => pageRows(queueRows, queuePage));

  function pageRows(rows: IndexedUploadRow[], page: number) {
    const start = Math.max(0, page) * uploadListPageSize;
    return rows.slice(start, start + uploadListPageSize);
  }

  $effect(() => {
    if (stagedPage >= stagedPageCount) stagedPage = stagedPageCount - 1;
    if (queuePage >= queuePageCount) queuePage = queuePageCount - 1;
  });
'''
text = text[:start] + replacement + text[end:]
text = text.replace("  function stagedPreview(index: number) {\n    return previewURLs[index] ?? '';\n  }\n\n  function queuePreview(index: number) {\n    return previewURLs[index] ?? '';\n  }\n", "")
text = text.replace('stagedItems.length', 'stagedRows.length').replace('queueItems.length', 'queueRows.length')

staged_start = text.index('          <div class="g-card upload-list-card">', text.index('{#if stagedRows.length > 0}'))
staged_end = text.index('          </div>\n        </section>', staged_start) + len('          </div>')
staged_markup = '''          <div class="g-card upload-list-card">
            <div class="upload-list" data-testid="staged-upload-list">
              {#each stagedVisibleRows as row (row.index)}
                {@const item = row.item}
                <div class="upload-row upload-row-staged">
                  <UploadMediaPreview file={uploadFiles[row.index]} {item} />
                  <div><div class="name">{item.name}</div></div>
                  <div class="size">{formatBytes(item.size)}</div>
                  <div class="progress is-staged" aria-hidden="true"></div>
                  <div class="status">
                    <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" title="Remove from staging" aria-label={`Remove ${item.name} from staging`} onclick={() => onRemove(row.index)}>
                      <Icon name="close" size={11} />
                    </button>
                  </div>
                </div>
              {/each}
            </div>
            {#if stagedPageCount > 1}
              <div class="upload-list-pager" aria-label="Staged upload pages">
                <button class="g-btn g-btn-sm" type="button" disabled={stagedPage === 0} onclick={() => stagedPage -= 1}>Previous</button>
                <span>Page {stagedPage + 1} of {stagedPageCount} · showing {stagedVisibleRows.length} at a time</span>
                <button class="g-btn g-btn-sm" type="button" disabled={stagedPage + 1 >= stagedPageCount} onclick={() => stagedPage += 1}>Next</button>
              </div>
            {/if}
          </div>'''
text = text[:staged_start] + staged_markup + text[staged_end:]

queue_start = text.index('          <div class="g-card upload-list-card">', text.index('{#if queueRows.length > 0}'))
queue_end = text.index('          </div>\n        </section>', queue_start) + len('          </div>')
queue_markup = '''          <div class="g-card upload-list-card">
            <div class="upload-list" data-testid="upload-queue-list">
              {#each queueVisibleRows as row (row.index)}
                {@const item = row.item}
                <div class="upload-row">
                  <UploadMediaPreview file={uploadFiles[row.index]} {item} />
                  <div>
                    <div class="name">{item.name}</div>
                    {#if item.error}<div class="upload-error">{item.error}</div>{/if}
                  </div>
                  <div class="size">{formatBytes(item.size)}</div>
                  <div class="progress" aria-label={`${statusLabel(item.status)} ${item.progress}%`}>
                    <div style={`width: ${item.progress}%`}></div>
                  </div>
                  <div class={`status ${statusClass(item.status)}`}>{statusLabel(item.status)}</div>
                </div>
              {/each}
            </div>
            {#if queuePageCount > 1}
              <div class="upload-list-pager" aria-label="Upload queue pages">
                <button class="g-btn g-btn-sm" type="button" disabled={queuePage === 0} onclick={() => queuePage -= 1}>Previous</button>
                <span>Page {queuePage + 1} of {queuePageCount} · showing {queueVisibleRows.length} at a time</span>
                <button class="g-btn g-btn-sm" type="button" disabled={queuePage + 1 >= queuePageCount} onclick={() => queuePage += 1}>Next</button>
              </div>
            {/if}
          </div>'''
text = text[:queue_start] + queue_markup + text[queue_end:]

text += '''\n\n<style>\n  .upload-list-pager {\n    display: flex;\n    align-items: center;\n    justify-content: center;\n    gap: 12px;\n    padding: 10px 12px;\n    border-top: 1px solid var(--border);\n    color: var(--text-3);\n    font-family: var(--font-mono);\n    font-size: 11px;\n  }\n</style>\n'''
panel.write_text(text)

Path('frontend/src/lib/components/UploadMediaPreview.svelte').write_text('''<script lang="ts">\n  import { onMount } from 'svelte';\n  import Icon from './Icon.svelte';\n  import type { UploadItem } from '$lib/state/uploadItems';\n\n  let { file, item } = $props<{ file?: File; item: UploadItem }>();\n  let host = $state<HTMLDivElement | undefined>();\n  let visible = $state(false);\n  let previewURL = $state('');\n\n  onMount(() => {\n    const node = host;\n    if (!node || typeof IntersectionObserver === 'undefined') {\n      visible = true;\n      return;\n    }\n    const observer = new IntersectionObserver((entries) => {\n      visible = entries.some((entry) => entry.isIntersecting);\n    }, { rootMargin: '160px 0px' });\n    observer.observe(node);\n    return () => observer.disconnect();\n  });\n\n  $effect(() => {\n    const source = file;\n    if (!visible || !source || (!source.type.startsWith('image/') && !source.type.startsWith('video/'))) {\n      previewURL = '';\n      return;\n    }\n    const url = URL.createObjectURL(source);\n    previewURL = url;\n    return () => {\n      URL.revokeObjectURL(url);\n      if (previewURL === url) previewURL = '';\n    };\n  });\n\n  function itemIcon() {\n    if (item.type?.startsWith('video/')) return 'video';\n    if (item.type?.startsWith('audio/')) return 'audio';\n    if (item.type === 'image/gif') return 'gif';\n    if (item.name.match(/\\.(zip|tar|gz)$/i)) return 'folder';\n    return 'photo';\n  }\n</script>\n\n<div bind:this={host} class="thumb-tile" data-upload-preview>\n  {#if previewURL && item.type?.startsWith('video/')}\n    <!-- svelte-ignore a11y_media_has_caption -->\n    <video src={previewURL} muted playsinline preload="metadata"></video>\n  {:else if previewURL && item.type?.startsWith('image/')}\n    <img src={previewURL} alt="" loading="lazy" decoding="async" />\n  {:else}\n    <Icon name={itemIcon()} size={18} />\n  {/if}\n</div>\n''')

Path('frontend/tests/upload-large-batch.spec.ts').write_text('''import { expect, test, type Page } from '@playwright/test';\n\nconst session = {\n  user: { id: 'usr_test', username: 'mac', role: 'admin' },\n  capabilities: { upload: true, tag: true, delete: true, admin: true },\n  csrf_token: 'csrf-one'\n};\n\nasync function openUpload(page: Page) {\n  let loggedIn = false;\n  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: loggedIn ? 200 : 401, contentType: 'application/json', body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } }) }));\n  await page.route('**/api/v1/auth/login', async (route) => { loggedIn = true; await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }); });\n  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));\n  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));\n  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));\n  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));\n  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) }));\n  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));\n  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));\n  await page.goto('/');\n  await page.getByLabel('Username').fill('mac');\n  await page.getByLabel('Password').fill('correct horse');\n  await page.getByRole('button', { name: 'Sign in' }).click();\n  await page.getByRole('button', { name: 'Upload' }).click();\n}\n\ntest('dropping ten thousand images keeps rendered rows and blob previews bounded', async ({ page }) => {\n  await page.addInitScript(() => {\n    const originalCreate = URL.createObjectURL.bind(URL);\n    const originalRevoke = URL.revokeObjectURL.bind(URL);\n    const stats = { active: 0, peak: 0, created: 0 };\n    Object.defineProperty(window, '__uploadBlobStats', { value: stats, configurable: false });\n    URL.createObjectURL = (object: Blob | MediaSource) => { stats.active += 1; stats.created += 1; stats.peak = Math.max(stats.peak, stats.active); return originalCreate(object); };\n    URL.revokeObjectURL = (url: string) => { stats.active = Math.max(0, stats.active - 1); originalRevoke(url); };\n  });\n  await openUpload(page);\n\n  await page.getByRole('group', { name: 'File upload drop zone' }).evaluate((zone) => {\n    const transfer = new DataTransfer();\n    for (let index = 0; index < 10_000; index += 1) {\n      transfer.items.add(new File([new Uint8Array([index % 251])], `geom_${String(index + 1).padStart(5, '0')}.jpg`, { type: 'image/jpeg' }));\n    }\n    zone.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer }));\n  });\n\n  await expect(page.getByText(/Staged · 10000 files/)).toBeVisible({ timeout: 15_000 });\n  const list = page.getByTestId('staged-upload-list');\n  await expect(list.locator('.upload-row')).toHaveCount(100);\n  await expect(page.getByLabel('Staged upload pages')).toContainText('Page 1 of 100');\n  await expect(list.getByText('geom_00001.jpg')).toBeVisible();\n  await expect(list.getByText('geom_00101.jpg')).toHaveCount(0);\n\n  const stats = await page.evaluate(() => (window as unknown as { __uploadBlobStats: { active: number; peak: number; created: number } }).__uploadBlobStats);\n  expect(stats.peak).toBeLessThan(30);\n  expect(stats.created).toBeLessThan(50);\n\n  await page.getByLabel('Staged upload pages').getByRole('button', { name: 'Next' }).click();\n  await expect(list.getByText('geom_00101.jpg')).toBeVisible();\n  await expect(list.locator('.upload-row')).toHaveCount(100);\n});\n''')
