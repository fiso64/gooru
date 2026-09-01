from pathlib import Path

path = Path('frontend/tests/shell.spec.ts')
text = path.read_text()
old = "  expect(inputStyle).toEqual({ padding: '9px 12px', radius: '6px', fontSize: '14px' });\n"
new = """  expect(inputStyle).toEqual({ padding: '9px 12px', radius: '4px', fontSize: '14px' });
  await input.evaluate((node) => node.blur());
  await expect.poll(() => input.evaluate((node) => getComputedStyle(node).borderRadius)).toBe('6px');
"""
if old not in text:
    raise SystemExit('focused input expectation not found')
path.write_text(text.replace(old, new, 1))
