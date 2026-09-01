from pathlib import Path

path = Path('frontend/src/lib/components/UploadPanel.svelte')
text = path.read_text()
text = text.replace(
    '<section class="upload-queue-section">',
    '<section class="upload-queue-section" aria-label={uploadStatus || \'Upload queue\'}>',
    1,
)
text = text.replace(
    '          {#if uploadStatus && !activeUploadJobID && !queueItems.length}<p class="status-note">{uploadStatus}</p>{/if}\n',
    '',
    1,
)
path.write_text(text)
