from pathlib import Path

upload_panel = r'''<script lang="ts">
  import Icon from './Icon.svelte';
  import { formatBytes, parseTags } from '$lib/utils/format';
  import type { UploadItem } from '$lib/state/uploadItems';

  let {
    uploadFiles,
    uploadItems,
    uploadTags,
    uploadBusy,
    cancelBusy,
    cancelRequested = false,
    uploadStatus,
    activeUploadJobID,
    targets,
    targetID,
    conflictPolicy,
    autoUpload,
    onTargetInput,
    onFiles,
    onTagsInput,
    onConflictInput,
    onAutoUploadInput,
    onSubmit,
    onCancel,
    onClear,
    onRemove
  } = $props<{
    uploadFiles: File[];
    uploadItems: UploadItem[];
    uploadTags: string;
    uploadBusy: boolean;
    cancelBusy: boolean;
    cancelRequested?: boolean;
    uploadStatus: string;
    activeUploadJobID: string;
    targets: Array<{ id: string; name: string }>;
    targetID: string;
    conflictPolicy: string;
    autoUpload: boolean;
    onTargetInput: (value: string) => void;
    onFiles: (files: FileList | File[] | null) => void;
    onTagsInput: (value: string) => void;
    onConflictInput: (value: string) => void;
    onAutoUploadInput: (value: boolean) => void;
    onSubmit: () => void;
    onCancel: (jobID: string) => void;
    onClear: () => void;
    onRemove: (index: number) => void;
  }>();

  let dragActive = $state(false);
  let fileInput: HTMLInputElement | undefined;
  let tagInput: HTMLInputElement | undefined;
  let tagEditorOpen = $state(false);
  let tagDraft = $state('');
  let previewURLs = $state<string[]>([]);

  const stagedItems = $derived(uploadItems.filter((item: UploadItem) => item.status === 'staged'));
  const queueItems = $derived(uploadItems.filter((item: UploadItem) => item.status !== 'staged'));
  const stagedBytes = $derived(uploadFiles.reduce((sum: number, file: File) => sum + file.size, 0));
  const queueBytes = $derived(queueItems.reduce((sum: number, item: UploadItem) => sum + item.size, 0));
  const initialTags = $derived(parseTags(uploadTags));

  $effect(() => {
    const urls = uploadFiles.map((file) =>
      file.type.startsWith('image/') || file.type.startsWith('video/') ? URL.createObjectURL(file) : ''
    );
    previewURLs = urls;
    return () => {
      for (const url of urls) if (url) URL.revokeObjectURL(url);
    };
  });

  function chooseFiles() {
    fileInput?.click();
  }

  function hasFiles(event: DragEvent) {
    return Array.from(event.dataTransfer?.types ?? []).includes('Files');
  }

  function handleDragOver(event: DragEvent) {
    if (!hasFiles(event)) return;
    event.preventDefault();
    dragActive = true;
  }

  function handleDragLeave(event: DragEvent) {
    if (event.currentTarget !== event.target) return;
    dragActive = false;
  }

  function handleDrop(event: DragEvent) {
    if (!hasFiles(event)) return;
    event.preventDefault();
    dragActive = false;
    onFiles(event.dataTransfer?.files ?? null);
  }

  function picked(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    onFiles(input.files);
    input.value = '';
  }

  function openTagEditor() {
    tagEditorOpen = true;
    queueMicrotask(() => tagInput?.focus());
  }

  function closeTagEditor() {
    tagDraft = '';
    tagEditorOpen = false;
  }

  function commitTagDraft() {
    const additions = parseTags(tagDraft);
    if (additions.length) {
      onTagsInput(Array.from(new Set([...initialTags, ...additions])).join(' '));
    }
    closeTagEditor();
  }

  function removeInitialTag(tag: string) {
    onTagsInput(initialTags.filter((candidate) => candidate !== tag).join(' '));
  }

  function itemIcon(item: UploadItem) {
    if (item.type?.startsWith('video/')) return 'video';
    if (item.type?.startsWith('audio/')) return 'audio';
    if (item.type === 'image/gif') return 'gif';
    if (item.name.match(/\.(zip|tar|gz)$/i)) return 'folder';
    return 'photo';
  }

  function statusClass(status: string) {
    if (status === 'imported' || status === 'uploaded') return 'ok';
    if (status === 'error') return 'err';
    return '';
  }

  function statusLabel(status: string) {
    return status.replace(/_/g, ' ');
  }

  function stagedPreview(index: number) {
    return previewURLs[index] ?? '';
  }

  function queuePreview(index: number) {
    return previewURLs[index] ?? '';
  }
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Upload</div>
      <h1>Import media into your library</h1>
      <p>Files are content-hashed on receipt. Duplicates are detected automatically. Initial tags can be applied here.</p>
    </div>

    <form class="upload-stack" onsubmit={(event) => { event.preventDefault(); onSubmit(); }}>
      <section class="g-card upload-config-card">
        <label class="field-row">
          <span>Target</span>
          <select class="g-input" value={targetID} onchange={(event) => onTargetInput(event.currentTarget.value)}>
            {#each targets as target}
              <option value={target.id}>{target.name}</option>
            {:else}
              <option value="">Default target</option>
            {/each}
          </select>
        </label>

        <div class="field-row">
          <span>Initial tags</span>
          <div class="field-control upload-tags-control">
            {#each initialTags as tag}
              {@const separator = tag.indexOf(':')}
              <span class="g-tag">
                {#if separator > 0}
                  <span class="g-tag-ns">{tag.slice(0, separator)}:</span><span>{tag.slice(separator + 1)}</span>
                {:else}
                  <span>{tag}</span>
                {/if}
                <button class="g-tag-x" type="button" aria-label={`Remove ${tag}`} onclick={() => removeInitialTag(tag)}>
                  <Icon name="close" size={11} />
                </button>
              </span>
            {/each}

            {#if tagEditorOpen}
              <input
                bind:this={tagInput}
                class="g-input upload-tag-entry"
                aria-label="Initial tag"
                placeholder="subject:portrait"
                value={tagDraft}
                oninput={(event) => (tagDraft = event.currentTarget.value)}
                onkeydown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault();
                    commitTagDraft();
                  } else if (event.key === 'Escape') {
                    event.preventDefault();
                    closeTagEditor();
                  }
                }}
              />
            {:else}
              <button class="g-btn g-btn-ghost g-btn-sm" type="button" onclick={openTagEditor}><Icon name="plus" size={12} /> Add tag</button>
            {/if}
          </div>
        </div>

        <div class="field-row">
          <span>On conflict</span>
          <div class="field-control">
            <div class="seg" aria-label="Upload conflict policy">
              {#each [{ value: 'skip', label: 'Skip' }, { value: 'rename', label: 'Rename' }, { value: 'replace', label: 'Replace' }] as option}
                <button type="button" class={conflictPolicy === option.value ? 'is-active' : ''} onclick={() => onConflictInput(option.value)}>{option.label}</button>
              {/each}
            </div>
          </div>
        </div>

        <div class="field-row">
          <span>On drop</span>
          <div class="field-control">
            <div class="seg" aria-label="Upload drop behavior">
              <button type="button" class={!autoUpload ? 'is-active' : ''} onclick={() => onAutoUploadInput(false)}>Stage first</button>
              <button type="button" class={autoUpload ? 'is-active' : ''} onclick={() => onAutoUploadInput(true)}>Auto-upload</button>
            </div>
          </div>
        </div>
      </section>

      <section
        class={`upload-zone ${dragActive ? 'is-drag' : ''}`}
        role="button"
        tabindex="0"
        onkeydown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); chooseFiles(); } }}
        onclick={chooseFiles}
        ondragover={handleDragOver}
        ondragleave={handleDragLeave}
        ondrop={handleDrop}
      >
        <input bind:this={fileInput} type="file" multiple hidden onchange={picked} />
        <div class="icon-wrap"><Icon name="upload" size={26} /></div>
        <h3>Drop files here</h3>
        <p>or click to browse</p>
        <div class="upload-zone-actions">
          <button class="g-btn g-btn-primary" type="button" onclick={(event) => { event.stopPropagation(); chooseFiles(); }}><Icon name="folder" size={14} /> Choose files…</button>
          <button class="g-btn" type="button" disabled title="Paste URL import coming soon" onclick={(event) => event.stopPropagation()}><Icon name="external" size={14} /> Paste URL</button>
        </div>
      </section>

      {#if stagedItems.length > 0}
        <section>
          <div class="upload-list-head">
            <div class="g-eyebrow">Staged · {stagedItems.length} {stagedItems.length === 1 ? 'file' : 'files'} · {formatBytes(stagedBytes)}</div>
            <div class="upload-list-actions">
              <button class="g-btn g-btn-sm" type="button" onclick={onClear}><Icon name="close" size={12} /> Clear staged</button>
              <button class="g-btn g-btn-primary g-btn-sm" type="submit" disabled={uploadBusy || Boolean(activeUploadJobID)}>
                <Icon name="upload" size={12} /> Upload {stagedItems.length} {stagedItems.length === 1 ? 'file' : 'files'}
              </button>
            </div>
          </div>
          <div class="g-card upload-list-card">
            <div class="upload-list">
              {#each stagedItems as item, index}
                {@const preview = stagedPreview(index)}
                <div class="upload-row upload-row-staged">
                  <div class="thumb-tile">
                    {#if preview && item.type?.startsWith('video/')}
                      <!-- svelte-ignore a11y_media_has_caption -->
                      <video src={preview} muted playsinline preload="metadata"></video>
                    {:else if preview && item.type?.startsWith('image/')}
                      <img src={preview} alt="" />
                    {:else}
                      <Icon name={itemIcon(item)} size={18} />
                    {/if}
                  </div>
                  <div><div class="name">{item.name}</div></div>
                  <div class="size">{formatBytes(item.size)}</div>
                  <div class="progress is-staged" aria-hidden="true"></div>
                  <div class="status">
                    <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" title="Remove from staging" aria-label={`Remove ${item.name} from staging`} onclick={() => onRemove(index)}>
                      <Icon name="close" size={11} />
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          </div>
        </section>
      {/if}

      {#if queueItems.length > 0}
        <section class="upload-queue-section">
          <div class:has-active-job={Boolean(activeUploadJobID)} class="upload-list-head upload-queue-head">
            <div class="g-eyebrow">Queue · {queueItems.length} {queueItems.length === 1 ? 'file' : 'files'} · {formatBytes(queueBytes)}</div>
            <div class="upload-list-actions upload-queue-actions">
              <button class="g-btn g-btn-sm" type="button" disabled title="Pause uploads coming soon"><Icon name="pause" size={12} /> Pause all</button>
              <button class="g-btn g-btn-sm" type="button" disabled={Boolean(activeUploadJobID)} onclick={onClear}><Icon name="close" size={12} /> Clear done</button>
            </div>
            {#if activeUploadJobID}
              <button
                class="g-btn g-btn-sm upload-cancel-action"
                type="button"
                disabled={cancelBusy || cancelRequested}
                onclick={() => onCancel(activeUploadJobID)}
              >
                {cancelBusy ? 'Canceling' : cancelRequested ? 'Canceled' : 'Cancel'}
              </button>
            {/if}
          </div>
          <div class="g-card upload-list-card">
            <div class="upload-list">
              {#each queueItems as item, index}
                {@const preview = queuePreview(index)}
                <div class="upload-row">
                  <div class="thumb-tile">
                    {#if preview && item.type?.startsWith('video/')}
                      <!-- svelte-ignore a11y_media_has_caption -->
                      <video src={preview} muted playsinline preload="metadata"></video>
                    {:else if preview && item.type?.startsWith('image/')}
                      <img src={preview} alt="" />
                    {:else}
                      <Icon name={itemIcon(item)} size={18} />
                    {/if}
                  </div>
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
          </div>
          {#if uploadStatus && !activeUploadJobID && !queueItems.length}<p class="status-note">{uploadStatus}</p>{/if}
        </section>
      {/if}
    </form>
  </div>
</main>
'''

Path('frontend/src/lib/components/UploadPanel.svelte').write_text(upload_panel)

css = Path('frontend/src/lib/styles/components.css')
text = css.read_text()
text = text.replace(
    ".upload-stack {\n  display: flex;\n  flex-direction: column;\n  gap: 16px;\n}",
    ".upload-stack {\n  display: flex;\n  flex-direction: column;\n  gap: 28px;\n}",
    1,
)
old_controls = """.upload-tags-control,
.inline-control,
.account-control,
.upload-zone-actions,
.upload-list-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.upload-tags-control {
  flex-wrap: wrap;
}

.upload-tags-control .g-input {
  flex: 1 1 260px;
}
"""
new_controls = """.inline-control,
.account-control,
.upload-zone-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.upload-tags-control,
.upload-list-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.upload-tags-control {
  flex-wrap: wrap;
}

.upload-tag-entry {
  flex: 1 1 220px;
  min-width: 180px;
  max-width: 320px;
}

.upload-tags-control .g-tag-x {
  display: inline-flex;
  margin-left: 2px;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
"""
if old_controls not in text:
    raise SystemExit('upload control CSS block not found')
text = text.replace(old_controls, new_controls, 1)
needle = """.upload-list-head,
.list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}
"""
addition = needle + """
.upload-queue-head {
  position: relative;
}

.upload-queue-actions,
.upload-cancel-action {
  transition: opacity 0.12s;
}

.upload-cancel-action {
  position: absolute;
  top: 50%;
  right: 0;
  transform: translateY(-50%);
  opacity: 0;
  pointer-events: none;
}

.upload-queue-head.has-active-job:hover .upload-queue-actions {
  opacity: 0;
  pointer-events: none;
}

.upload-queue-head.has-active-job:hover .upload-cancel-action,
.upload-cancel-action:focus-visible {
  opacity: 1;
  pointer-events: auto;
}
"""
if needle not in text:
    raise SystemExit('upload list head CSS block not found')
text = text.replace(needle, addition, 1)
css.write_text(text)

# The old status block becomes unused once cancellation is integrated contextually
# into the exact queue header. Remove it only if no other source references it.
job_status = Path('frontend/src/lib/components/JobStatus.svelte')
if job_status.exists():
    uses = []
    for candidate in Path('frontend/src').rglob('*'):
        if not candidate.is_file() or candidate == job_status or candidate.suffix not in {'.svelte', '.ts'}:
            continue
        if 'JobStatus' in candidate.read_text():
            uses.append(candidate)
    if not uses:
        job_status.unlink()

spec = Path('frontend/tests/shell.spec.ts')
text = spec.read_text()
old_fill = "  await page.getByPlaceholder('collection:inbox @review').fill('incoming');\n"
new_fill = """  await page.getByRole('button', { name: 'Add tag' }).click();
  await page.getByLabel('Initial tag').fill('incoming');
  await page.getByLabel('Initial tag').press('Enter');
"""
if old_fill not in text:
    raise SystemExit('existing upload tag fill assertion not found')
text = text.replace(old_fill, new_fill, 1)
old_cancel = """  await expect(page.getByText('Importing', { exact: true })).toBeVisible();
  const deleteResponse = page.waitForResponse((response) =>
    response.url().endsWith('/api/v1/jobs/job-one') && response.request().method() === 'DELETE'
  );
  await page.getByRole('button', { name: 'Cancel' }).click();
"""
new_cancel = """  await expect(page.getByText('Importing', { exact: true })).toBeVisible();
  await expect(page.getByText(/Queue · 1 file/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Pause all' })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Clear done' })).toBeDisabled();
  const deleteResponse = page.waitForResponse((response) =>
    response.url().endsWith('/api/v1/jobs/job-one') && response.request().method() === 'DELETE'
  );
  await page.locator('.upload-queue-head').hover();
  await page.getByRole('button', { name: 'Cancel' }).click();
"""
if old_cancel not in text:
    raise SystemExit('existing upload cancel assertion not found')
text = text.replace(old_cancel, new_cancel, 1)

if "test('matches exact Upload staging surface and releases local previews'" not in text:
    text += r'''

test('matches exact Upload staging surface and releases local previews', async ({ page }) => {
  await page.addInitScript(() => {
    const original = URL.revokeObjectURL.bind(URL);
    const state = window as typeof window & { __gooruRevoked?: string[] };
    state.__gooruRevoked = [];
    URL.revokeObjectURL = (url: string) => {
      state.__gooruRevoked?.push(url);
      original(url);
    };
  });
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  await page.goto('/');
  await signIn(page);
  await page.locator('.sidebar').getByRole('button', { name: 'Upload' }).click();
  await expect(page.getByRole('heading', { name: 'Import media into your library' })).toBeVisible();

  const stackStyle = await page.locator('.upload-stack').evaluate((node) => ({ gap: getComputedStyle(node).gap }));
  expect(stackStyle.gap).toBe('28px');
  const configStyle = await page.locator('.upload-config-card').evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, gap: style.gap };
  });
  expect(configStyle).toEqual({ padding: '18px', gap: '16px' });
  const zoneStyle = await page.locator('.upload-zone').evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, radius: style.borderRadius };
  });
  expect(zoneStyle.padding).toBe('48px 24px');
  await expect(page.getByText(/max 5 GB/)).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Paste URL' })).toBeDisabled();

  await expect(page.getByLabel('Initial tag')).toHaveCount(0);
  await page.getByRole('button', { name: 'Add tag' }).click();
  const tagInput = page.getByLabel('Initial tag');
  await expect(tagInput).toBeFocused();
  await tagInput.fill('collection:may-2026 @review');
  await tagInput.press('Enter');
  await expect(page.locator('.upload-tags-control .g-tag')).toHaveCount(2);
  await expect(page.locator('.upload-tags-control')).toContainText('collection:may-2026');
  await expect(page.locator('.upload-tags-control')).toContainText('@review');
  await page.getByRole('button', { name: 'Remove collection:may-2026' }).click();
  await expect(page.locator('.upload-tags-control .g-tag')).toHaveCount(1);

  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zs1sAAAAASUVORK5CYII=', 'base64');
  await page.locator('input[type="file"]').setInputFiles({ name: 'stage.png', mimeType: 'image/png', buffer: png });
  const stagedRow = page.locator('.upload-row-staged').filter({ hasText: 'stage.png' });
  await expect(stagedRow).toBeVisible();
  const preview = stagedRow.locator('.thumb-tile img');
  await expect(preview).toHaveAttribute('src', /^blob:/);
  const previewURL = await preview.getAttribute('src');
  expect(previewURL).toBeTruthy();

  await stagedRow.getByRole('button', { name: 'Remove stage.png from staging' }).click();
  await expect(stagedRow).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => (window as typeof window & { __gooruRevoked?: string[] }).__gooruRevoked ?? [])).toContain(previewURL!);
});
'''

spec.write_text(text)

docs = Path('docs/issue-21-concept-discrepancies.md')
text = docs.read_text()
entry = '44. Upload rendered staging/queue surface: B for the concept card/drop-zone/list geometry, real initial-tag chips with an interaction-only editor, bounded local image/video previews while files are staged, and contextual cancellation; A for unsupported Paste URL and pause-all controls; C for prototype-only paths, mock seed tags, and the unguaranteed 5 GB claim. Local object URLs are revoked whenever the staged file set changes.'
if entry not in text:
    docs.write_text(text.rstrip() + '\n' + entry + '\n')
