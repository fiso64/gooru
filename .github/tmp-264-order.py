from pathlib import Path


def rep(path, old, new):
    p=Path(path); t=p.read_text()
    if new in t: return
    if old not in t: raise SystemExit(f'missing anchor {path}: {old[:100]!r}')
    p.write_text(t.replace(old,new,1))

# Schema: nullable during ALTER then fully backfilled to the legacy server ordering.
Path('internal/database/migrations/009_saved_search_position.up.sql').write_text('''ALTER TABLE saved_searches ADD COLUMN position INTEGER;\n\nWITH ranked AS (\n    SELECT id, ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY updated_at DESC, name COLLATE NOCASE ASC, id ASC) - 1 AS new_position\n    FROM saved_searches\n)\nUPDATE saved_searches\nSET position = (SELECT new_position FROM ranked WHERE ranked.id = saved_searches.id);\n\nCREATE UNIQUE INDEX idx_saved_searches_user_position ON saved_searches(user_id, position);\n''')
Path('internal/database/migrations/009_saved_search_position.down.sql').write_text('''DROP INDEX IF EXISTS idx_saved_searches_user_position;\nALTER TABLE saved_searches DROP COLUMN position;\n''')

rep('types/types.go', '''\tOrder     string\n\tCreatedAt int64''', '''\tOrder     string\n\tPosition  int\n\tCreatedAt int64''')

rep('internal/database/database.go', '''SELECT id, user_id, name, query, sort, "order", strftime('%s', created_at), strftime('%s', updated_at) FROM saved_searches WHERE user_id = ? ORDER BY updated_at DESC, name ASC''', '''SELECT id, user_id, name, query, sort, "order", position, strftime('%s', created_at), strftime('%s', updated_at) FROM saved_searches WHERE user_id = ? ORDER BY position ASC, created_at ASC, id ASC''')
rep('internal/database/database.go', '''SELECT id, user_id, name, query, sort, "order", strftime('%s', created_at), strftime('%s', updated_at) FROM saved_searches WHERE user_id = ? AND id = ?''', '''SELECT id, user_id, name, query, sort, "order", position, strftime('%s', created_at), strftime('%s', updated_at) FROM saved_searches WHERE user_id = ? AND id = ?''')
rep('internal/database/database.go', '''\t_, err := s.Exec(`\n\t\tINSERT INTO saved_searches (id, user_id, name, query, sort, "order")\n\t\tVALUES (?, ?, ?, ?, ?, ?)\n\t`, item.ID, item.UserID, item.Name, item.Query, item.Sort, item.Order)''', '''\t_, err := s.Exec(`\n\t\tINSERT INTO saved_searches (id, user_id, name, query, sort, "order", position)\n\t\tVALUES (?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM saved_searches WHERE user_id = ?))\n\t`, item.ID, item.UserID, item.Name, item.Query, item.Sort, item.Order, item.UserID)''')
rep('internal/database/database.go', '''\t\tif err := rows.Scan(&item.ID, &item.UserID, &item.Name, &item.Query, &item.Sort, &item.Order, &item.CreatedAt, &item.UpdatedAt); err != nil {''', '''\t\tif err := rows.Scan(&item.ID, &item.UserID, &item.Name, &item.Query, &item.Sort, &item.Order, &item.Position, &item.CreatedAt, &item.UpdatedAt); err != nil {''')
# Atomic full-order replacement. The unique index requires a two-phase negative staging to avoid transient collisions.
insert = r'''
func (s *Store) ReorderSavedSearches(userID string, ids []string) error {
	tx, err := s.Begin()
	if err != nil { return err }
	defer tx.Rollback()

	rows, err := tx.Query("SELECT id FROM saved_searches WHERE user_id = ? ORDER BY position ASC, created_at ASC, id ASC", userID)
	if err != nil { return err }
	var existing []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil { rows.Close(); return err }
		existing = append(existing, id)
	}
	if err := rows.Err(); err != nil { rows.Close(); return err }
	rows.Close()
	if len(existing) != len(ids) { return fmt.Errorf("saved search reorder must contain every saved search exactly once") }
	seen := make(map[string]struct{}, len(ids))
	valid := make(map[string]struct{}, len(existing))
	for _, id := range existing { valid[id] = struct{}{} }
	for _, id := range ids {
		if _, ok := valid[id]; !ok { return fmt.Errorf("saved search reorder contains unknown id") }
		if _, duplicate := seen[id]; duplicate { return fmt.Errorf("saved search reorder contains duplicate id") }
		seen[id] = struct{}{}
	}
	if _, err := tx.Exec("UPDATE saved_searches SET position = -position - 1 WHERE user_id = ?", userID); err != nil { return err }
	for position, id := range ids {
		if _, err := tx.Exec("UPDATE saved_searches SET position = ? WHERE user_id = ? AND id = ?", position, userID, id); err != nil { return err }
	}
	return tx.Commit()
}

'''
p=Path('internal/database/database.go'); t=p.read_text(); anchor='func (s *Store) DeleteSavedSearch(userID string, id string) (bool, error) {'
if 'func (s *Store) ReorderSavedSearches' not in t:
    t=t.replace(anchor, insert+anchor,1); p.write_text(t)

# Core client wrapper.
p=Path('gooru/saved_search.go'); t=p.read_text(); anchor='func (c *Client) SavedSearchForUsername'
if 'func (c *Client) ReorderSavedSearches' not in t:
    t=t.replace(anchor, '''func (c *Client) ReorderSavedSearches(userID string, ids []string) error {\n\treturn c.store.ReorderSavedSearches(userID, ids)\n}\n\n'''+anchor,1); p.write_text(t)

# Serve library/handler.
rep('internal/serve/saved_search_library.go', '''func (l *GooruLibrary) DeleteSavedSearch(ctx context.Context, userID string, id string) (bool, error) {''', '''func (l *GooruLibrary) ReorderSavedSearches(ctx context.Context, userID string, ids []string) error {\n\tif err := ctx.Err(); err != nil { return err }\n\treturn l.client.ReorderSavedSearches(userID, ids)\n}\n\nfunc (l *GooruLibrary) DeleteSavedSearch(ctx context.Context, userID string, id string) (bool, error) {''')
rep('internal/serve/search_saved.go', '''\tDeleteSavedSearch(ctx context.Context, userID string, id string) (bool, error)\n}''', '''\tDeleteSavedSearch(ctx context.Context, userID string, id string) (bool, error)\n\tReorderSavedSearches(ctx context.Context, userID string, ids []string) error\n}''')
rep('internal/serve/search_saved.go', '''\tUpdatedAt time.Time `json:"updated_at"`''', '''\tPosition  int       `json:"position"`\n\tUpdatedAt time.Time `json:"updated_at"`''')
rep('internal/serve/search_saved.go', '''\t\tOrder:     item.Order,\n\t\tCreatedAt:''', '''\t\tOrder:     item.Order,\n\t\tPosition:  item.Position,\n\t\tCreatedAt:''')
# Dedicated endpoint handler.
p=Path('internal/serve/search_saved.go'); t=p.read_text(); anchor='func (s *Server) handleSavedSearch(w http.ResponseWriter, r *http.Request) {'
handler=r'''type savedSearchReorderRequest struct {
	IDs []string `json:"ids"`
}

func (s *Server) handleSavedSearchReorder(w http.ResponseWriter, r *http.Request) {
	library, ok := s.library.(SavedSearchLibrary)
	if !ok { writeError(w, http.StatusServiceUnavailable, "service_unavailable", "saved search service is not configured", nil); return }
	auth, ok := currentAuth(r.Context())
	if !ok { writeError(w, http.StatusUnauthorized, "unauthorized", "login required", nil); return }
	var req savedSearchReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request body", nil); return }
	if req.IDs == nil { writeError(w, http.StatusBadRequest, "invalid_request", "ids is required", nil); return }
	if err := library.ReorderSavedSearches(r.Context(), auth.User.ID, req.IDs); err != nil { writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil); return }
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

'''
if 'handleSavedSearchReorder' not in t:
    t=t.replace(anchor,handler+anchor,1); p.write_text(t)
rep('internal/serve/server.go', '''\tmux.Handle("/api/v1/saved-searches/", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleSavedSearch))))''', '''\tmux.Handle("/api/v1/saved-searches/reorder", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, methodHandler(http.MethodPut, s.handleSavedSearchReorder))))\n\tmux.Handle("/api/v1/saved-searches/", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleSavedSearch))))''')

# OpenAPI endpoint + SavedSearch position. Anchor around saved-searches/{id}.
p=Path('docs/openapi.yaml'); t=p.read_text()
if '/saved-searches/reorder:' not in t:
    anchor='  /saved-searches/{id}:\n'
    block='''  /saved-searches/reorder:\n    put:\n      summary: Persist the complete saved-search order\n      requestBody:\n        required: true\n        content:\n          application/json:\n            schema:\n              type: object\n              required: [ids]\n              properties:\n                ids:\n                  type: array\n                  items: { type: string }\n      responses:\n        '200':\n          description: Order persisted\n          content:\n            application/json:\n              schema:\n                type: object\n                properties:\n                  ok: { type: boolean }\n        '400': { $ref: '#/components/responses/ErrorResponse' }\n        '401': { $ref: '#/components/responses/ErrorResponse' }\n        '403': { $ref: '#/components/responses/ErrorResponse' }\n\n'''
    if anchor not in t: raise SystemExit('openapi saved id anchor missing')
    t=t.replace(anchor,block+anchor,1)
# add position after order only in SavedSearch schema if identifiable globally by created_at sequence
needle='''          order:\n            type: string\n          created_at:'''
if needle in t:
    t=t.replace(needle,'''          order:\n            type: string\n          position:\n            type: integer\n            minimum: 0\n          created_at:''',1)
p.write_text(t)

# Frontend client + query mutation.
rep('frontend/src/lib/api/client.ts', '''  async deleteSavedSearch(id: string): Promise<void> {''', '''  async reorderSavedSearches(ids: string[]): Promise<void> {\n    await this.unwrap(this.client.PUT('/saved-searches/reorder', { params: { header: this.csrfHeaderParam('PUT') }, body: { ids } }));\n  }\n\n  async deleteSavedSearch(id: string): Promise<void> {''')
rep('frontend/src/lib/queries/library.ts', '''export function createSavedSearchDeleteMutation''', '''export function createSavedSearchReorderMutation(getCSRFToken: () => string, queryClient: QueryClient) {\n  return createMutation<void, Error, string[]>(() => ({\n    mutationFn: (ids) => new ApiClient(getCSRFToken()).reorderSavedSearches(ids),\n    onSuccess: () => queryClient.invalidateQueries({ queryKey: libraryKeys.savedSearchesRoot })\n  }));\n}\n\nexport function createSavedSearchDeleteMutation''')
# Authenticated app imports and mutation / callback / prop.
rep('frontend/src/lib/components/AuthenticatedApp.svelte', '''    createSavedSearchesQuery,\n    createSavedSearchUpdateMutation,''', '''    createSavedSearchesQuery,\n    createSavedSearchReorderMutation,\n    createSavedSearchUpdateMutation,''')
rep('frontend/src/lib/components/AuthenticatedApp.svelte', '''  const updateSavedSearchMutation = createSavedSearchUpdateMutation''', '''  const reorderSavedSearchMutation = createSavedSearchReorderMutation(() => $authState.csrfToken, queryClient);\n  const updateSavedSearchMutation = createSavedSearchUpdateMutation''')
# Add handler before delete function.
rep('frontend/src/lib/components/AuthenticatedApp.svelte', '''  function deleteSavedSearch(id: string, name: string) {''', '''  async function reorderSavedSearches(ids: string[]) {\n    await reorderSavedSearchMutation.mutateAsync(ids);\n  }\n\n  function deleteSavedSearch(id: string, name: string) {''')
# AppShell prop invocation anchor.
rep('frontend/src/lib/components/AuthenticatedApp.svelte', '''    onDeleteSavedSearch={deleteSavedSearch}\n''', '''    onDeleteSavedSearch={deleteSavedSearch}\n    onReorderSavedSearches={reorderSavedSearches}\n''')

# AppShell native drag-drop with a dedicated handle and full-list reorder callback.
rep('frontend/src/lib/components/AppShell.svelte', '''    onDeleteSavedSearch,\n    onSearchDraft,''', '''    onDeleteSavedSearch,\n    onReorderSavedSearches,\n    onSearchDraft,''')
rep('frontend/src/lib/components/AppShell.svelte', '''    onDeleteSavedSearch: (id: string, name: string) => void;\n    onSearchDraft:''', '''    onDeleteSavedSearch: (id: string, name: string) => void;\n    onReorderSavedSearches: (ids: string[]) => Promise<void>;\n    onSearchDraft:''')
rep('frontend/src/lib/components/AppShell.svelte', '''  let shortcutsReturnRoute = $state('library');''', '''  let shortcutsReturnRoute = $state('library');\n  let draggedSavedSearchID = $state('');\n  let savedSearchReorderBusy = $state(false);''')
# functions before openLibrary.
rep('frontend/src/lib/components/AppShell.svelte', '''  function openLibrary() {''', '''  function startSavedSearchDrag(event: DragEvent, id: string) {\n    if (savedSearchReorderBusy) { event.preventDefault(); return; }\n    draggedSavedSearchID = id;\n    event.dataTransfer?.setData('text/plain', id);\n    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';\n  }\n\n  async function dropSavedSearch(event: DragEvent, targetID: string) {\n    event.preventDefault();\n    const sourceID = draggedSavedSearchID || event.dataTransfer?.getData('text/plain') || '';\n    draggedSavedSearchID = '';\n    if (!sourceID || sourceID === targetID || savedSearchReorderBusy) return;\n    const ids = savedSearches.map((item) => item.id);\n    const from = ids.indexOf(sourceID);\n    const to = ids.indexOf(targetID);\n    if (from < 0 || to < 0) return;\n    const [moved] = ids.splice(from, 1);\n    ids.splice(to, 0, moved);\n    savedSearchReorderBusy = true;\n    try { await onReorderSavedSearches(ids); } finally { savedSearchReorderBusy = false; }\n  }\n\n  function openLibrary() {''')
rep('frontend/src/lib/components/AppShell.svelte', '''        <div class="sidebar-saved-row">\n          <button class="sidebar-item"''', '''        <div class="sidebar-saved-row" ondragover={(event) => { if (draggedSavedSearchID) event.preventDefault(); }} ondrop={(event) => void dropSavedSearch(event, saved.id)}>\n          <button class="sidebar-mini saved-search-drag" type="button" title={`Drag ${saved.name}`} aria-label={`Drag ${saved.name}`} draggable={!savedSearchReorderBusy} ondragstart={(event) => startSavedSearchDrag(event, saved.id)} ondragend={() => (draggedSavedSearchID = '')}>\n            <span aria-hidden="true">⋮⋮</span>\n          </button>\n          <button class="sidebar-item"''')
# style for cursor
rep('frontend/src/lib/components/AppShell.svelte', '''  .common-tags-toggle {''', '''  .saved-search-drag {\n    cursor: grab;\n    flex: 0 0 auto;\n  }\n\n  .saved-search-drag:active { cursor: grabbing; }\n\n  .common-tags-toggle {''')
