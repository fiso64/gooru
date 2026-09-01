from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    file = Path(path)
    text = file.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"expected exactly one match in {path}: {old!r}; found {count}")
    file.write_text(text.replace(old, new, 1))


def replace_in_section(text: str, start: str, end: str, old: str, new: str) -> str:
    start_index = text.find(start)
    if start_index < 0:
        raise SystemExit(f"missing section start: {start!r}")
    end_index = text.find(end, start_index + len(start))
    if end_index < 0:
        raise SystemExit(f"missing section end: {end!r}")
    section = text[start_index:end_index]
    count = section.count(old)
    if count != 1:
        raise SystemExit(f"expected one match in section {start!r}: {old!r}; found {count}")
    return text[:start_index] + section.replace(old, new, 1) + text[end_index:]


openapi_path = Path('docs/openapi.yaml')
text = openapi_path.read_text()

# Cookie-authenticated tag mutations are adminProtected and therefore require CSRF.
for start, end in [
    ('  /files/tags:\n    post:', '    put:'),
    ('  /files/tags:\n    post:', '    delete:'),
]:
    pass

# POST /files/tags
text = replace_in_section(
    text,
    '  /files/tags:\n    post:',
    '    put:',
    '      parameters:\n        - $ref: "#/components/parameters/PreferAsync"\n',
    '      parameters:\n        - $ref: "#/components/parameters/PreferAsync"\n        - $ref: "#/components/parameters/CSRF"\n',
)
text = replace_in_section(
    text,
    '  /files/tags:\n    post:',
    '    put:',
    '        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "404":\n',
    '        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "403":\n          $ref: "#/components/responses/Forbidden"\n        "404":\n',
)
# PUT /files/tags
files_tags_index = text.find('  /files/tags:')
put_index = text.find('    put:', files_tags_index)
delete_index = text.find('    delete:', put_index)
put_section = text[put_index:delete_index]
for old, new in [
    ('      parameters:\n        - $ref: "#/components/parameters/PreferAsync"\n',
     '      parameters:\n        - $ref: "#/components/parameters/PreferAsync"\n        - $ref: "#/components/parameters/CSRF"\n'),
    ('        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "404":\n',
     '        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "403":\n          $ref: "#/components/responses/Forbidden"\n        "404":\n'),
]:
    if put_section.count(old) != 1:
        raise SystemExit(f"unexpected PUT /files/tags shape for {old!r}")
    put_section = put_section.replace(old, new, 1)
text = text[:put_index] + put_section + text[delete_index:]
# DELETE /files/tags
delete_index = text.find('    delete:', text.find('  /files/tags:'))
upload_targets_index = text.find('  /upload-targets:', delete_index)
delete_section = text[delete_index:upload_targets_index]
for old, new in [
    ('      parameters:\n        - $ref: "#/components/parameters/PreferAsync"\n',
     '      parameters:\n        - $ref: "#/components/parameters/PreferAsync"\n        - $ref: "#/components/parameters/CSRF"\n'),
    ('        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "404":\n',
     '        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "403":\n          $ref: "#/components/responses/Forbidden"\n        "404":\n'),
]:
    if delete_section.count(old) != 1:
        raise SystemExit(f"unexpected DELETE /files/tags shape for {old!r}")
    delete_section = delete_section.replace(old, new, 1)
text = text[:delete_index] + delete_section + text[upload_targets_index:]

# Uploads are adminProtected mutations too.
text = replace_in_section(
    text,
    '  /uploads:\n    post:',
    '  /tags:',
    '      parameters:\n        - $ref: "#/components/parameters/PreferAsync"\n',
    '      parameters:\n        - $ref: "#/components/parameters/PreferAsync"\n        - $ref: "#/components/parameters/CSRF"\n',
)

# Job list/detail reads are admin-only; cancellation also requires CSRF.
text = replace_in_section(
    text,
    '  /jobs:\n    get:',
    '    delete:',
    '        "401":\n          $ref: "#/components/responses/Unauthorized"\n',
    '        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "403":\n          $ref: "#/components/responses/Forbidden"\n',
)
text = replace_in_section(
    text,
    '  /jobs/{id}:\n    get:',
    '    delete:',
    '        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "404":\n',
    '        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "403":\n          $ref: "#/components/responses/Forbidden"\n        "404":\n',
)
text = replace_in_section(
    text,
    '    delete:\n      summary: Cancel an asynchronous job.',
    'components:',
    '          schema:\n            type: string\n      responses:\n',
    '          schema:\n            type: string\n        - $ref: "#/components/parameters/CSRF"\n      responses:\n',
)
text = replace_in_section(
    text,
    '    delete:\n      summary: Cancel an asynchronous job.',
    'components:',
    '        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "404":\n',
    '        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "403":\n          $ref: "#/components/responses/Forbidden"\n        "404":\n',
)
text = text.replace(
    '    Forbidden:\n      description: Request is not allowed by server configuration.\n',
    '    Forbidden:\n      description: Request is forbidden, including failed CSRF validation or insufficient privileges.\n',
    1,
)
openapi_path.write_text(text)

client_path = Path('frontend/src/lib/api/client.ts')
client = client_path.read_text()
replacements = [
    ("this.client.POST('/files/tags', { headers: this.csrfHeaders('POST'), body: requestBody })",
     "this.client.POST('/files/tags', { params: { header: this.csrfHeaderParam('POST') }, body: requestBody })"),
    ("this.client.PUT('/files/tags', { headers: this.csrfHeaders('PUT'), body: requestBody })",
     "this.client.PUT('/files/tags', { params: { header: this.csrfHeaderParam('PUT') }, body: requestBody })"),
    ("this.client.DELETE('/files/tags', { headers: this.csrfHeaders('DELETE'), body: requestBody })",
     "this.client.DELETE('/files/tags', { params: { header: this.csrfHeaderParam('DELETE') }, body: requestBody })"),
    ("headers: { ...this.csrfHeaders('POST'), ...(preferAsync ? { Prefer: 'respond-async' } : {}) },",
     "params: { header: { ...this.csrfHeaderParam('POST'), ...(preferAsync ? { Prefer: 'respond-async' as const } : {}) } },"),
    ("this.client.DELETE('/jobs/{id}', { headers: this.csrfHeaders('DELETE'), params: { path: { id } } })",
     "this.client.DELETE('/jobs/{id}', { params: { header: this.csrfHeaderParam('DELETE'), path: { id } } })"),
    ("\n  private csrfHeaders(method: string): Record<string, string> {\n    if (!this.csrfToken || !isMutatingMethod(method)) return {};\n    return { 'X-Gooru-CSRF': this.csrfToken };\n  }\n", ""),
]
for old, new in replacements:
    count = client.count(old)
    if count != 1:
        raise SystemExit(f"expected exactly one client match: {old!r}; found {count}")
    client = client.replace(old, new, 1)
client_path.write_text(client)
