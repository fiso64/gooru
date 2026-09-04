from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        if new in text:
            return
        raise SystemExit(f"missing anchor in {path}: {old[:120]!r}")
    p.write_text(text.replace(old, new, 1))


def append_once(path, marker, content):
    p = Path(path)
    text = p.read_text()
    if marker not in text:
        p.write_text(text + content)

# The primary patch updates the first shared Scan call. Update every remaining
# saved-search Scan so SELECT and destination arity stay aligned.
p = Path('internal/database/database.go')
text = p.read_text()
old_scan = 'rows.Scan(&item.ID, &item.UserID, &item.Name, &item.Query, &item.Sort, &item.Order, &item.CreatedAt, &item.UpdatedAt)'
new_scan = 'rows.Scan(&item.ID, &item.UserID, &item.Name, &item.Query, &item.Sort, &item.Order, &item.Position, &item.CreatedAt, &item.UpdatedAt)'
text = text.replace(old_scan, new_scan)
p.write_text(text)

# Keep direct/internal inserts safe even if they omit the new ordinal.
p = Path('internal/database/migrations/009_saved_search_position.up.sql')
text = p.read_text()
trigger = '''\nCREATE TRIGGER fill_saved_search_position_after_insert\nAFTER INSERT ON saved_searches\nWHEN NEW.position IS NULL\nBEGIN\n    UPDATE saved_searches\n    SET position = (\n        SELECT COALESCE(MAX(position), -1) + 1\n        FROM saved_searches\n        WHERE user_id = NEW.user_id AND id <> NEW.id\n    )\n    WHERE id = NEW.id;\nEND;\n'''
if 'fill_saved_search_position_after_insert' not in text:
    text += trigger
p.write_text(text)
p = Path('internal/database/migrations/009_saved_search_position.down.sql')
text = p.read_text()
if 'fill_saved_search_position_after_insert' not in text:
    text = 'DROP TRIGGER IF EXISTS fill_saved_search_position_after_insert;\n' + text
p.write_text(text)

# Core username convenience keeps CLI and API callers on the same persisted order.
p = Path('gooru/saved_search.go')
text = p.read_text()
anchor = 'func (c *Client) SavedSearchForUsername(username, reference string) (types.SavedSearch, error) {'
method = '''func (c *Client) ReorderSavedSearchesForUsername(username string, references []string) error {\n\tuserID, err := c.SavedSearchUserID(username)\n\tif err != nil {\n\t\treturn err\n\t}\n\titems, err := c.ListSavedSearches(userID)\n\tif err != nil {\n\t\treturn err\n\t}\n\tids := make([]string, 0, len(references))\n\tfor _, reference := range references {\n\t\treference = strings.TrimSpace(reference)\n\t\tfound := ""\n\t\tfor _, item := range items {\n\t\t\tif item.ID == reference || strings.EqualFold(item.Name, reference) {\n\t\t\t\tfound = item.ID\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n\t\tif found == "" {\n\t\t\treturn fmt.Errorf("%w: %q", ErrSavedSearchNotFound, reference)\n\t\t}\n\t\tids = append(ids, found)\n\t}\n\treturn c.ReorderSavedSearches(userID, ids)\n}\n\n'''
if 'ReorderSavedSearchesForUsername' not in text:
    if anchor not in text:
        raise SystemExit('saved search core anchor missing')
    text = text.replace(anchor, method + anchor, 1)
p.write_text(text)

# Fix the core regression to use the existing update API.
p = Path('gooru/saved_search_test.go')
text = p.read_text()
old = 'if _, err := client.UpdateSavedSearchForUsername("alice", first.ID, "First renamed", "kind:image", "name", "asc"); err != nil {'
new = 'if _, err := client.UpdateSavedSearch(types.SavedSearch{ID: first.ID, UserID: "usr_test", Name: "First renamed", Query: "kind:image", Sort: "name", Order: "asc"}); err != nil {'
text = text.replace(old, new)
p.write_text(text)

# CLI exposes ordering rather than leaving the core capability Web-only.
p = Path('cmd/gooru/cmd/saved_search.go')
text = p.read_text()
if 'ReorderSavedSearchesForUsername' not in text:
    text = text.replace('''\tSearchSavedSearchForUsername(username, reference string, verbose bool) ([]types.FileInfo, error)\n}''', '''\tSearchSavedSearchForUsername(username, reference string, verbose bool) ([]types.FileInfo, error)\n\tReorderSavedSearchesForUsername(username string, references []string) error\n}''', 1)
    text = text.replace('''\tsearchCmd := &cobra.Command{''', '''\treorderCmd := &cobra.Command{\n\t\tUse:   "reorder <name-or-id>...",\n\t\tShort: "Persists a user's complete saved-search order.",\n\t\tArgs:  cobra.MinimumNArgs(1),\n\t\tRunE: func(cmd *cobra.Command, args []string) error {\n\t\t\tif err := client().ReorderSavedSearchesForUsername(username, args); err != nil {\n\t\t\t\treturn fmt.Errorf("reorder saved searches: %w", err)\n\t\t\t}\n\t\t\treturn nil\n\t\t},\n\t}\n\n\tsearchCmd := &cobra.Command{''', 1)
    text = text.replace('cmd.AddCommand(createCmd, listCmd, searchCmd)', 'cmd.AddCommand(createCmd, listCmd, reorderCmd, searchCmd)', 1)
p.write_text(text)

# Replace the first-pass OpenAPI operation with a complete authenticated/CSRF contract.
p = Path('docs/openapi.yaml')
text = p.read_text()
start = text.find('  /saved-searches/reorder:\n')
end = text.find('  /saved-searches/{id}:\n', start)
if start >= 0 and end > start:
    block = '''  /saved-searches/reorder:\n    put:\n      summary: Persist the complete saved-search order.\n      security:\n        - sessionAuth: []\n      parameters:\n        - $ref: "#/components/parameters/CSRF"\n      requestBody:\n        required: true\n        content:\n          application/json:\n            schema:\n              type: object\n              required: [ids]\n              properties:\n                ids:\n                  type: array\n                  items:\n                    type: string\n      responses:\n        "200":\n          description: Saved-search order persisted.\n          content:\n            application/json:\n              schema:\n                type: object\n                required: [ok]\n                properties:\n                  ok:\n                    type: boolean\n        "400":\n          $ref: "#/components/responses/BadRequest"\n        "401":\n          $ref: "#/components/responses/Unauthorized"\n        "403":\n          $ref: "#/components/responses/Forbidden"\n\n'''
    text = text[:start] + block + text[end:]
text = text.replace('required: [id, name, query, sort, order, created_at, updated_at]', 'required: [id, name, query, sort, order, position, created_at, updated_at]', 1)
p.write_text(text)

# Browser/E2E regression: exercise real drag/drop, assert exact API permutation,
# then reload to prove the server-provided order remains durable.
append_once('frontend/tests/shell.spec.ts', "test('persists saved-search drag order across reload'", r'''

test('persists saved-search drag order across reload', async ({ page }) => {
  await mockAuth(page);
  let saved = [
    { id: 's1', name: 'First', query: 'one', sort: 'name', order: 'asc', position: 0 },
    { id: 's2', name: 'Second', query: 'two', sort: 'name', order: 'asc', position: 1 },
    { id: 's3', name: 'Third', query: 'three', sort: 'name', order: 'asc', position: 2 }
  ];
  const reorders: string[][] = [];
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches/reorder', async (route) => {
    const body = route.request().postDataJSON() as { ids: string[] };
    reorders.push(body.ids);
    saved = body.ids.map((id, position) => ({ ...saved.find((item) => item.id === id)!, position }));
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: saved }) });
  });
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));

  await page.goto('/');
  await signIn(page);
  const rows = page.locator('.sidebar-saved-row');
  await expect(rows.locator('.truncate')).toHaveText(['First', 'Second', 'Third']);
  await page.getByLabel('Drag First').dragTo(rows.nth(2));
  await expect.poll(() => reorders).toEqual([['s2', 's3', 's1']]);
  await expect(rows.locator('.truncate')).toHaveText(['Second', 'Third', 'First']);

  await page.reload();
  await expect(page.locator('.sidebar-saved-row .truncate')).toHaveText(['Second', 'Third', 'First']);
});
''')
