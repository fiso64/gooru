from pathlib import Path

viewer = Path('frontend/src/lib/utils/viewer.ts')
text = viewer.read_text()
old = """export function viewerMediaStyle(geometry: ViewerGeometry): string {\n  return [\n    'position:absolute',\n    'left:50%',\n    'top:50%',\n    `width:${geometry.width}px`,\n    `height:${geometry.height}px`,\n    'max-width:none',\n    'max-height:none',\n    `transform:translate(-50%, -50%) rotate(${geometry.rotation}deg)`,\n    'transform-origin:center center'\n  ].join(';');\n}\n"""
new = """export type ViewerMediaTransform = {\n  zoom?: number;\n  panX?: number;\n  panY?: number;\n};\n\nexport function viewerMediaStyle(geometry: ViewerGeometry, transform: ViewerMediaTransform = {}): string {\n  const zoom = Math.max(0, transform.zoom ?? 1);\n  const panX = Number.isFinite(transform.panX) ? (transform.panX ?? 0) : 0;\n  const panY = Number.isFinite(transform.panY) ? (transform.panY ?? 0) : 0;\n  return [\n    'position:absolute',\n    'left:50%',\n    'top:50%',\n    `width:${geometry.width}px`,\n    `height:${geometry.height}px`,\n    'max-width:none',\n    'max-height:none',\n    `transform:translate(calc(-50% + ${panX}px), calc(-50% + ${panY}px)) rotate(${geometry.rotation}deg) scale(${zoom})`,\n    'transform-origin:center center'\n  ].join(';');\n}\n"""
if old in text:
    viewer.write_text(text.replace(old, new))
elif 'export type ViewerMediaTransform' not in text:
    raise SystemExit('viewer.ts patch anchor not found')

stage = Path('frontend/src/lib/components/ViewerStage.svelte')
text = stage.read_text()
old = """  let cursorIdle = $state(false);\n  let cursorIdleTimer: ReturnType<typeof setTimeout> | undefined;\n\n  const renderedFile = $derived(displayedFile ?? file);\n"""
new = """  let cursorIdle = $state(false);\n  let cursorIdleTimer: ReturnType<typeof setTimeout> | undefined;\n  let zoom = $state(1);\n  let panX = $state(0);\n  let panY = $state(0);\n\n  const renderedFile = $derived(displayedFile ?? file);\n"""
if old in text:
    text = text.replace(old, new)

old = """  const geometry = $derived(viewerGeometry({\n    intrinsicWidth,\n    intrinsicHeight,\n    viewportWidth: stageWidth,\n    viewportHeight: stageHeight,\n    rotation,\n    fitMode,\n    inset: isFullscreen ? 0 : 36,\n    maxScale: preserveNativeViewerSize(renderedFile) ? 1 : Number.POSITIVE_INFINITY\n  }));\n  const visualStyle = $derived(viewerMediaStyle(geometry));\n"""
new = """  const geometry = $derived(viewerGeometry({\n    intrinsicWidth,\n    intrinsicHeight,\n    viewportWidth: stageWidth,\n    viewportHeight: stageHeight,\n    rotation,\n    fitMode,\n    inset: isFullscreen ? 0 : 36,\n    maxScale: preserveNativeViewerSize(renderedFile) ? 1 : Number.POSITIVE_INFINITY\n  }));\n  const fitGeometry = $derived(viewerGeometry({\n    intrinsicWidth,\n    intrinsicHeight,\n    viewportWidth: stageWidth,\n    viewportHeight: stageHeight,\n    rotation,\n    fitMode: 'screen',\n    inset: isFullscreen ? 0 : 36,\n    maxScale: preserveNativeViewerSize(renderedFile) ? 1 : Number.POSITIVE_INFINITY\n  }));\n  const minimumZoom = $derived(fitMode === 'screen' ? 1 : Math.min(1, fitGeometry.scale));\n  const visualStyle = $derived(viewerMediaStyle(geometry, { zoom, panX, panY }));\n"""
if old in text:
    text = text.replace(old, new)
elif 'const minimumZoom' not in text:
    raise SystemExit('geometry patch anchor not found')

old = """    videoPaused = true;\n    videoTime = 0;\n    videoLength = 0;\n"""
new = """    zoom = 1;\n    panX = 0;\n    panY = 0;\n    videoPaused = true;\n    videoTime = 0;\n    videoLength = 0;\n"""
if old in text and "    zoom = 1;\n    panX = 0;\n    panY = 0;" not in text:
    text = text.replace(old, new, 1)

anchor = """  async function toggleFullscreen() {\n"""
addition = """  function clampViewerPan(nextX: number, nextY: number, nextZoom: number) {\n    const normalizedRotation = ((rotation % 360) + 360) % 360;\n    const quarterTurn = normalizedRotation === 90 || normalizedRotation === 270;\n    const renderedWidth = (quarterTurn ? geometry.height : geometry.width) * nextZoom;\n    const renderedHeight = (quarterTurn ? geometry.width : geometry.height) * nextZoom;\n    const inset = isFullscreen ? 0 : 36;\n    const viewportWidth = Math.max(0, stageWidth - inset * 2);\n    const viewportHeight = Math.max(0, stageHeight - inset * 2);\n    const limitX = Math.max(0, (renderedWidth - viewportWidth) / 2);\n    const limitY = Math.max(0, (renderedHeight - viewportHeight) / 2);\n    return {\n      x: Math.max(-limitX, Math.min(limitX, nextX)),\n      y: Math.max(-limitY, Math.min(limitY, nextY))\n    };\n  }\n\n  function resetViewerTransform() {\n    zoom = 1;\n    panX = 0;\n    panY = 0;\n  }\n\n  function setFitMode(mode: ViewerFitMode) {\n    fitMode = mode;\n    resetViewerTransform();\n  }\n\n  function handleViewerWheel(event: WheelEvent) {\n    if (renderedFile.media_kind === 'audio' || renderedFile.media_type.startsWith('audio/')) return;\n    const stage = stageElement;\n    if (!stage) return;\n\n    if (event.ctrlKey) {\n      event.preventDefault();\n      const nextZoom = Math.max(minimumZoom, Math.min(32, zoom * Math.exp(-event.deltaY * 0.002)));\n      if (Math.abs(nextZoom - zoom) < 0.0001) return;\n      const rect = stage.getBoundingClientRect();\n      const pointerX = event.clientX - (rect.left + rect.width / 2);\n      const pointerY = event.clientY - (rect.top + rect.height / 2);\n      const ratio = nextZoom / zoom;\n      const anchoredX = pointerX - (pointerX - panX) * ratio;\n      const anchoredY = pointerY - (pointerY - panY) * ratio;\n      const clamped = clampViewerPan(anchoredX, anchoredY, nextZoom);\n      zoom = nextZoom;\n      panX = clamped.x;\n      panY = clamped.y;\n      return;\n    }\n\n    if (zoom <= minimumZoom + 0.0001) return;\n    event.preventDefault();\n    const horizontalDelta = event.altKey ? event.deltaY + event.deltaX : event.deltaX;\n    const verticalDelta = event.altKey ? 0 : event.deltaY;\n    const clamped = clampViewerPan(panX - horizontalDelta, panY - verticalDelta, zoom);\n    panX = clamped.x;\n    panY = clamped.y;\n  }\n\n"""
if addition.strip() not in text:
    if anchor not in text:
        raise SystemExit('wheel patch anchor not found')
    text = text.replace(anchor, addition + anchor, 1)

text = text.replace("      fitMode = 'screen';", "      setFitMode('screen');")
text = text.replace("      fitMode = 'actual';", "      setFitMode('actual');")
text = text.replace("class=\"lightbox-stage viewer-stage\" tabindex=\"-1\" aria-busy={waitingForTarget} onpointermove={handleStagePointerMove}>", "class=\"lightbox-stage viewer-stage\" tabindex=\"-1\" aria-busy={waitingForTarget} onpointermove={handleStagePointerMove} onwheel={handleViewerWheel}>")
text = text.replace("onclick={(event) => { fitMode = 'screen'; restoreStageFocusAfterPointer(event); }}", "onclick={(event) => { setFitMode('screen'); restoreStageFocusAfterPointer(event); }}")
text = text.replace("onclick={(event) => { fitMode = 'actual'; restoreStageFocusAfterPointer(event); }}", "onclick={(event) => { setFitMode('actual'); restoreStageFocusAfterPointer(event); }}")
if 'onwheel={handleViewerWheel}' not in text:
    raise SystemExit('stage wheel handler patch failed')
stage.write_text(text)

test = Path('frontend/tests/viewer-zoom-pan.spec.ts')
if not test.exists():
    test.write_text('''import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function imageItem() {
  return {
    id: 'large-image',
    content_id: 'hash-large-image',
    name: 'large.jpg',
    safe_display_path: 'library/large.jpg',
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'image',
    metadata: { image_width: 2400, image_height: 1600 },
    tags: [],
    media_urls: {
      thumbnail: '/api/v1/files/large-image/thumbnail',
      preview: '/api/v1/files/large-image/preview',
      content: '/api/v1/files/large-image/content',
      download: '/api/v1/files/large-image/download'
    }
  };
}

async function mockViewer(page: Page) {
  let loggedIn = false;
  const files = [imageItem()];
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files, total_count: 1, library_count: 1, facets: { kind: [] } }) }));
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="2400" height="1600"><rect width="2400" height="1600"/></svg>';
  await page.route('**/api/v1/files/large-image/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  await page.route('**/api/v1/files/large-image/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  await page.route('**/api/v1/files/large-image/content', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await page.getByRole('button', { name: 'Preview large.jpg' }).click();
  const image = page.locator('img.viewer-visual-media');
  await expect(image).toBeVisible();
  return image;
}

async function wheelWithModifier(page: Page, key: 'Control' | 'Alt', deltaY: number) {
  await page.keyboard.down(key);
  await page.mouse.wheel(0, deltaY);
  await page.keyboard.up(key);
}

test('ctrl-wheel zooms toward the pointer and wheel/alt-wheel pan while zoomed', async ({ page }) => {
  const image = await mockViewer(page);
  const stage = page.locator('.viewer-stage');
  const stageBox = await stage.boundingBox();
  const before = await image.boundingBox();
  expect(stageBox).not.toBeNull();
  expect(before).not.toBeNull();

  const cursorX = stageBox!.x + stageBox!.width * 0.7;
  const cursorY = stageBox!.y + stageBox!.height * 0.4;
  await page.mouse.move(cursorX, cursorY);
  await wheelWithModifier(page, 'Control', -360);
  await page.waitForTimeout(160);
  const zoomed = await image.boundingBox();
  expect(zoomed).not.toBeNull();
  expect(zoomed!.width).toBeGreaterThan(before!.width * 1.5);
  const beforeU = (cursorX - before!.x) / before!.width;
  const beforeV = (cursorY - before!.y) / before!.height;
  expect(Math.abs((zoomed!.x + beforeU * zoomed!.width) - cursorX)).toBeLessThan(3);
  expect(Math.abs((zoomed!.y + beforeV * zoomed!.height) - cursorY)).toBeLessThan(3);

  await page.mouse.wheel(0, 120);
  await page.waitForTimeout(160);
  const verticallyPanned = await image.boundingBox();
  expect(verticallyPanned!.y).toBeLessThan(zoomed!.y - 20);

  await wheelWithModifier(page, 'Alt', 120);
  await page.waitForTimeout(160);
  const horizontallyPanned = await image.boundingBox();
  expect(horizontallyPanned!.x).toBeLessThan(verticallyPanned!.x - 20);
});

test('zoom minima respect fit and actual-size modes in normal and fullscreen viewer', async ({ page }) => {
  const image = await mockViewer(page);
  const stage = page.locator('.viewer-stage');
  const fit = await image.boundingBox();
  expect(fit).not.toBeNull();

  const box = await stage.boundingBox();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await wheelWithModifier(page, 'Control', 2000);
  await page.waitForTimeout(160);
  const fitMinimum = await image.boundingBox();
  expect(Math.abs(fitMinimum!.width - fit!.width)).toBeLessThan(2);

  await stage.focus();
  await page.keyboard.press('2');
  await page.waitForTimeout(160);
  const actual = await image.boundingBox();
  expect(actual!.width).toBeGreaterThan(fit!.width * 1.5);
  await wheelWithModifier(page, 'Control', 4000);
  await page.waitForTimeout(160);
  const actualMinimum = await image.boundingBox();
  expect(actualMinimum!.width).toBeGreaterThanOrEqual(fit!.width - 2);
  expect(actualMinimum!.width).toBeLessThan(actual!.width);

  await page.keyboard.press('1');
  await page.keyboard.press('f');
  await expect.poll(() => stage.evaluate((node) => document.fullscreenElement === node)).toBe(true);
  const fullscreenFit = await image.boundingBox();
  const fullscreenStage = await stage.boundingBox();
  await page.mouse.move(fullscreenStage!.x + fullscreenStage!.width * 0.65, fullscreenStage!.y + fullscreenStage!.height * 0.45);
  await wheelWithModifier(page, 'Control', -300);
  await page.waitForTimeout(160);
  const fullscreenZoomed = await image.boundingBox();
  expect(fullscreenZoomed!.width).toBeGreaterThan(fullscreenFit!.width * 1.4);
});
''')
