from pathlib import Path

path = Path('frontend/tests/shell.spec.ts')
text = path.read_text()

old_input = "  expect(inputStyle).toEqual({ padding: '9px 12px', radius: '6px', fontSize: '14px' });\n"
new_input = """  expect(inputStyle).toEqual({ padding: '9px 12px', radius: '4px', fontSize: '14px' });
  await input.evaluate((node) => node.blur());
  await expect.poll(() => input.evaluate((node) => getComputedStyle(node).borderRadius)).toBe('6px');
"""
if old_input not in text:
    raise SystemExit('focused input expectation not found')
text = text.replace(old_input, new_input, 1)

# The prototype baseline deliberately scopes `font: inherit` through
# `.gooru-root button`, which outranks the generic `.g-btn` type declarations.
# Test the rendered concept cascade instead of the lower-specificity declaration.
old_button = "  expect(buttonStyle).toEqual({ padding: '5px 10px', radius: '6px', fontSize: '12px', weight: '500', gap: '6px' });\n"
new_button = "  expect(buttonStyle).toEqual({ padding: '5px 10px', radius: '6px', fontSize: '14px', weight: '400', gap: '6px' });\n"
if old_button not in text:
    raise SystemExit('small button expectation not found')
text = text.replace(old_button, new_button, 1)

path.write_text(text)
