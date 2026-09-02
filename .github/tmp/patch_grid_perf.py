from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    target = Path(path)
    text = target.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    target.write_text(text.replace(old, new, 1))


replace_once(
    "frontend/src/lib/state/ui.ts",
    """const gridPadding = 16 * 2;\nconst gridGap = 5;\nconst minCardWidth = 180;\n""",
    """const gridPadding = 16 * 2;\nconst gridGap = 5;\nconst minCardWidth = 180;\nconst overscanRows = 4;\n""",
)

replace_once(
    "frontend/src/lib/state/ui.ts",
    """export function virtualGrid(\n""",
    """export function virtualGridStartRow(scrollY: number, gridTop: number, rowHeight: number) {\n  if (!Number.isFinite(rowHeight) || rowHeight <= 0) return 0;\n  const viewportStart = Math.max(0, scrollY - gridTop);\n  return Math.max(0, Math.floor(viewportStart / rowHeight) - overscanRows);\n}\n\nexport function virtualGrid(\n""",
)

replace_once(
    "frontend/src/lib/state/ui.ts",
    """  const overscanRows = 4;\n  const totalRows = Math.ceil(Math.max(totalItems, retainedStartIndex + files.length) / columns);\n  const viewportStart = Math.max(0, scrollY - gridTop);\n  const startRow = Math.max(0, Math.floor(viewportStart / rowHeight) - overscanRows);\n""",
    """  const totalRows = Math.ceil(Math.max(totalItems, retainedStartIndex + files.length) / columns);\n  const startRow = virtualGridStartRow(scrollY, gridTop, rowHeight);\n""",
)

replace_once(
    "frontend/src/lib/components/MediaGrid.svelte",
    """  import { virtualGrid } from '$lib/state/ui';\n""",
    """  import { virtualGrid, virtualGridStartRow } from '$lib/state/ui';\n""",
)

replace_once(
    "frontend/src/lib/components/MediaGrid.svelte",
    """  function handleScroll() {\n    paneScrollY = mainHost?.scrollTop ?? 0;\n  }\n""",
    """  function handleScroll() {\n    const nextScrollY = mainHost?.scrollTop ?? 0;\n    // The rendered file slice changes only when the overscanned virtual window\n    // crosses a row boundary. Avoid invalidating the Svelte tree for every\n    // intermediate scroll event inside the same window.\n    if (virtualGridStartRow(nextScrollY, gridTop, virtual.rowHeight) === virtualGridStartRow(paneScrollY, gridTop, virtual.rowHeight)) return;\n    paneScrollY = nextScrollY;\n  }\n""",
)

Path("frontend/src/lib/state/ui.test.ts").write_text("""import { describe, expect, it } from 'vitest';\nimport { gridRowHeight, virtualGrid, virtualGridStartRow } from './ui';\n\ndescribe('virtual grid scrolling', () => {\n  it('keeps the reactive window stable across scroll events within overscan rows', () => {\n    const rowHeight = gridRowHeight(960);\n    expect(virtualGridStartRow(0, 120, rowHeight)).toBe(0);\n    expect(virtualGridStartRow(120 + rowHeight * 4.9, 120, rowHeight)).toBe(0);\n    expect(virtualGridStartRow(120 + rowHeight * 5.1, 120, rowHeight)).toBe(1);\n  });\n\n  it('changes the rendered slice only when the virtual start row changes', () => {\n    const files = Array.from({ length: 240 }, (_, index) => ({ id: `file-${index}` })) as never[];\n    const first = virtualGrid(files, 960, 800, 700, 100, 10_000, 0);\n    const sameWindow = virtualGrid(files, 960, 800, 760, 100, 10_000, 0);\n    const nextWindow = virtualGrid(files, 960, 800, 900, 100, 10_000, 0);\n    expect(sameWindow.offsetTop).toBe(first.offsetTop);\n    expect(sameWindow.files.map((file: { id: string }) => file.id)).toEqual(first.files.map((file: { id: string }) => file.id));\n    expect(nextWindow.offsetTop).toBeGreaterThanOrEqual(first.offsetTop);\n  });\n});\n""")

Path("frontend/tests/grid-scroll-performance.spec.ts").write_text("""import { expect, test, type Page } from '@playwright/test';\n\nconst session = {\n  user: { id: 'usr_test', username: 'mac', role: 'admin' },\n  capabilities: { upload: true, tag: true, delete: true, admin: true },\n  csrf_token: 'csrf-one'\n};\n\nfunction fileItem(index: number) {\n  const id = `file-${index}`;\n  return {\n    id, content_id: `hash-${id}`, name: `perf-${index}.jpg`, safe_display_path: `library/${id}.jpg`,\n    size: 2048, modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo',\n    metadata: { image_width: 800, image_height: 600 }, tags: [],\n    media_urls: {\n      thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`,\n      content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download`\n    }\n  };\n}\n\nasync function mockApp(page: Page) {\n  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));\n  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));\n  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));\n  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));\n  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));\n  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));\n  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));\n  const files = Array.from({ length: 600 }, (_, index) => fileItem(index));\n  await page.route('**/api/v1/files?**', async (route) => route.fulfill({\n    contentType: 'application/json',\n    body: JSON.stringify({ files, total_count: 10_000, library_count: 10_000, facets: { kind: [{ value: 'photo', count: 10_000 }] } })\n  }));\n  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({\n    contentType: 'image/svg+xml',\n    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>'\n  }));\n}\n\ntest('large library keeps a bounded DOM while sustained scrolling advances the virtual window', async ({ page }) => {\n  await mockApp(page);\n  await page.goto('/');\n  await expect(page.getByText('10,000 files')).toBeVisible();\n  const cards = page.locator('.thumb');\n  await expect.poll(() => cards.count()).toBeLessThan(100);\n\n  const samples = await page.locator('.main').evaluate(async (node) => {\n    const grid = node.querySelector<HTMLElement>('[data-testid="virtual-media-grid"]')!;\n    const values: string[] = [];\n    for (let step = 1; step <= 30; step += 1) {\n      node.scrollTop = step * 250;\n      node.dispatchEvent(new Event('scroll'));\n      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));\n      values.push(grid.style.transform);\n    }\n    return values;\n  });\n  expect(new Set(samples).size).toBeGreaterThan(10);\n  expect(await cards.count()).toBeLessThan(100);\n});\n""")
