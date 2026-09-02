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
    """const overscanRows = 4;\n""",
    """const overscanRows = 4;\nconst virtualWindowStrideRows = 3;\n""",
)
replace_once(
    "frontend/src/lib/state/ui.ts",
    """  const viewportStart = Math.max(0, scrollY - gridTop);\n  return Math.max(0, Math.floor(viewportStart / rowHeight) - overscanRows);\n""",
    """  const viewportStart = Math.max(0, scrollY - gridTop);\n  const overscannedStart = Math.max(0, Math.floor(viewportStart / rowHeight) - overscanRows);\n  return Math.floor(overscannedStart / virtualWindowStrideRows) * virtualWindowStrideRows;\n""",
)
replace_once(
    "frontend/src/lib/state/ui.ts",
    """  const visibleRows = Math.ceil(viewportHeight / rowHeight) + overscanRows * 2;\n""",
    """  // Keep enough trailing rows for the window to stay mounted while its\n  // chunked start lags the viewport by up to stride - 1 rows.\n  const visibleRows = Math.ceil(viewportHeight / rowHeight) + overscanRows * 2 + virtualWindowStrideRows - 1;\n""",
)
replace_once(
    "frontend/src/lib/state/ui.test.ts",
    """    expect(virtualGridStartRow(120 + rowHeight * 5.1, 120, rowHeight)).toBe(1);\n""",
    """    expect(virtualGridStartRow(120 + rowHeight * 5.1, 120, rowHeight)).toBe(0);\n    expect(virtualGridStartRow(120 + rowHeight * 7.1, 120, rowHeight)).toBe(3);\n""",
)
replace_once(
    "frontend/src/lib/styles/components.css",
    """  background: rgba(0, 0, 0, 0.72);\n  -webkit-backdrop-filter: blur(8px);\n  backdrop-filter: blur(8px);\n""",
    """  background: rgba(0, 0, 0, 0.82);\n""",
)

# Exercise the compositor-sensitive badge path in the existing large-library browser test.
p = Path("frontend/tests/grid-scroll-performance.spec.ts")
text = p.read_text()
text = text.replace("media_type: 'image/jpeg', media_kind: 'photo',", "media_type: index % 2 ? 'video/mp4' : 'image/jpeg', media_kind: index % 2 ? 'video' : 'photo',")
text = text.replace(
    """  const cards = page.locator('.thumb');\n  await expect.poll(() => cards.count()).toBeLessThan(100);\n\n""",
    """  const cards = page.locator('.thumb');\n  await expect.poll(() => cards.count()).toBeLessThan(100);\n  const badge = page.locator('.thumb-badge').first();\n  await expect(badge).toBeVisible();\n  expect(await badge.evaluate((node) => getComputedStyle(node).backdropFilter)).toBe('none');\n\n""",
)
p.write_text(text)
