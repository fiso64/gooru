from pathlib import Path

p = Path('frontend/tests/viewer-metadata.spec.ts')
text = p.read_text()
text = text.replace(
"  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));",
"  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));\n  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));"
)
text = text.replace(
"  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"32\" height=\"32\" />' }));",
"  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"32\" height=\"32\" />' }));\n  await page.route('**/api/v1/files/*/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"64\" height=\"64\" />' }));"
)
text = text.replace("  await card.dblclick();\n  await expect(page.locator('.preview-dialog')).toBeVisible();", "  await card.getByRole('button', { name: `Preview ${name}` }).click();\n  await expect(page.getByRole('dialog')).toBeVisible();")
p.write_text(text)
