from pathlib import Path

stage_path = Path('frontend/src/lib/components/ViewerStage.svelte')
stage = stage_path.read_text()
old_effect = '''  $effect(() => {\n    renderedFile.id;\n    renderedImageSource;\n    intrinsicWidth = 0;\n    intrinsicHeight = 0;\n    videoPaused = true;\n    videoTime = 0;\n    videoLength = 0;\n  });\n'''
new_effect = '''  $effect(() => {\n    const nextFile = renderedFile;\n    renderedImageSource;\n    const rendersImage = nextFile.media_kind !== 'video' && nextFile.media_kind !== 'audio' && !nextFile.media_type.startsWith('audio/');\n    // The next image is preloaded and decoded before this source swap. Preserve the\n    // current geometry until its own load event supplies new intrinsic dimensions,\n    // otherwise the rendered image collapses to 0x0 for a frame during navigation.\n    if (!rendersImage) {\n      intrinsicWidth = 0;\n      intrinsicHeight = 0;\n    }\n    videoPaused = true;\n    videoTime = 0;\n    videoLength = 0;\n  });\n'''
if stage.count(old_effect) != 1:
    raise SystemExit('ViewerStage reset effect did not match exactly once')
stage_path.write_text(stage.replace(old_effect, new_effect, 1))

test_path = Path('frontend/tests/viewer-animation.spec.ts')
tests = test_path.read_text()
anchor = '''test('rotation keeps the requested direction when crossing the 0/360 boundary', async ({ page }) => {\n'''
new_test = '''test('navigation never collapses the displayed image while swapping decoded sources', async ({ page }) => {\n  await mockApp(page);\n  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();\n\n  const media = page.locator('.viewer-visual-media');\n  await expect(media).toBeVisible();\n  await expect.poll(() => media.evaluate((node) => node.getBoundingClientRect().width)).toBeGreaterThan(0);\n  await media.evaluate((node) => {\n    const state = window as typeof window & { __viewerZeroSizeObserved?: boolean; __viewerSizeObserver?: MutationObserver };\n    state.__viewerZeroSizeObserved = false;\n    const inspect = () => {\n      const rect = node.getBoundingClientRect();\n      if (rect.width === 0 || rect.height === 0) state.__viewerZeroSizeObserved = true;\n    };\n    const observer = new MutationObserver(inspect);\n    observer.observe(node, { attributes: true, attributeFilter: ['style', 'src'] });\n    state.__viewerSizeObserver = observer;\n  });\n\n  await page.getByLabel('Next file').click();\n  await expect(page.getByRole('dialog', { name: 'tall.jpg' })).toBeVisible();\n  await expect.poll(() => media.getAttribute('src')).toContain('/tall/preview');\n  await expect.poll(() => media.evaluate((node) => node.getBoundingClientRect().height)).toBeGreaterThan(0);\n  expect(await page.evaluate(() => (window as typeof window & { __viewerZeroSizeObserved?: boolean }).__viewerZeroSizeObserved)).toBe(false);\n  await page.evaluate(() => {\n    const state = window as typeof window & { __viewerSizeObserver?: MutationObserver };\n    state.__viewerSizeObserver?.disconnect();\n  });\n});\n\n'''
if tests.count(anchor) != 1:
    raise SystemExit('viewer animation test anchor did not match exactly once')
test_path.write_text(tests.replace(anchor, new_test + anchor, 1))
