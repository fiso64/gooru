from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if new in text:
        return
    if old not in text:
        raise SystemExit(f"missing patch anchor in {path}")
    p.write_text(text.replace(old, new, 1))

replace(
    "internal/serve/upload_stream.go",
    '''\t\t\t\tif err := os.Chtimes(finalized.path, sourceModTime, sourceModTime); err != nil {\n\t\t\t\t\treturn nil, saved, uploadFileError{name: finalized.name, err: errors.New("failed to preserve uploaded file modification time")}\n\t\t\t\t}''',
    '''\t\t\t\tif err := os.Chtimes(finalized.path, sourceModTime, sourceModTime); err != nil {\n\t\t\t\t\t_ = os.Remove(finalized.path)\n\t\t\t\t\treturn nil, saved, uploadFileError{name: finalized.name, err: errors.New("failed to preserve uploaded file modification time")}\n\t\t\t\t}''',
)

replace(
    "frontend/src/lib/api/client.test.ts",
    '''    const file = new File(['hello'], 'hello.txt', { type: 'text/plain' });\n\n    const result = await client.uploadFiles([file], ['reviewed'], true, '', 'rename', progress);''',
    '''    const sourceModTime = 1_700_000_000_123;\n    const file = new File(['hello'], 'hello.txt', { type: 'text/plain', lastModified: sourceModTime });\n\n    const result = await client.uploadFiles([file], ['reviewed'], true, '', 'rename', progress);''',
)
replace(
    "frontend/src/lib/api/client.test.ts",
    '''    expect(((xhr.body as FormData).get('files') as File).name).toBe('hello.txt');\n    expect(progress.mock.calls.map(([value]) => value)).toEqual([20, 70, 100, 100]);''',
    '''    expect(((xhr.body as FormData).get('files') as File).name).toBe('hello.txt');\n    expect((xhr.body as FormData).get('source_mod_time_ms')).toBe(String(sourceModTime));\n    expect(progress.mock.calls.map(([value]) => value)).toEqual([20, 70, 100, 100]);''',
)
