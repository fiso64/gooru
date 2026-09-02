from pathlib import Path


def replace(path: str, old: str, new: str, count: int = 1):
    p = Path(path)
    s = p.read_text()
    if old not in s:
        raise SystemExit(f"missing anchor in {path}:\n{old[:300]}")
    p.write_text(s.replace(old, new, count))

# Backend route dispatch: preserve authenticated GET and add CSRF/admin-protected bulk DELETE.
replace(
    "internal/serve/server.go",
    '\tmux.Handle("/api/v1/files/", s.protected(http.HandlerFunc(s.handleFile)))\n\tmux.Handle("/api/v1/files", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleListFiles)))\n',
    '\tmux.Handle("/api/v1/files/", s.protected(http.HandlerFunc(s.handleFile)))\n\tmux.Handle("/api/v1/files", s.protected(http.HandlerFunc(s.handleFiles)))\n',
)
replace(
    "internal/serve/browse.go",
    'func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {\n',
    '''func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
\tswitch r.Method {
\tcase http.MethodGet:
\t\ts.handleListFiles(w, r)
\tcase http.MethodDelete:
\t\tif !s.requireAdmin(w, r) {
\t\t\treturn
\t\t}
\t\ts.handleRemoveFiles(w, r)
\tdefault:
\t\tw.Header().Set("Allow", "GET, DELETE")
\t\twriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
\t}
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
''',
)

Path("internal/serve/file_removals.go").write_text(r'''package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	core "gooru.local/gooru"
	"gooru.local/internal/query"
	"gooru.local/types"
)

type FileRemovalRequest struct {
	Mode           string   `json:"mode"`
	FileIDs        []string `json:"file_ids,omitempty"`
	Query          string   `json:"query,omitempty"`
	ExcludeFileIDs []string `json:"exclude_file_ids,omitempty"`
}

type FileRemovalSelector struct {
	FileIDs        []string `json:"file_ids,omitempty"`
	Query          string   `json:"query,omitempty"`
	ExcludeFileIDs []string `json:"exclude_file_ids,omitempty"`
}

type FileRemovalResponse struct {
	Mode             string              `json:"mode"`
	Selector         FileRemovalSelector `json:"selector"`
	RemovedLocations int                 `json:"removed_locations"`
}

func (s *Server) handleRemoveFiles(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	if _, ok := s.library.(PublicFileLibrary); !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file mutation service is not configured", nil)
		return
	}
	request, err := decodeFileRemovalRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if err := validateFileRemovalRequest(request); err != nil {
		if errors.Is(err, core.ErrInvalidQuery) {
			writeError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	files, err := s.resolveFileRemovalSelection(r.Context(), request)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve selected files", nil)
		return
	}

	// Physical deletion is intentionally all-or-nothing at the policy boundary:
	// validate every selected path before moving or untracking any one of them.
	if request.Mode == "delete" {
		for _, file := range files {
			if !s.canDeleteFilePath(file.Path) {
				writeError(w, http.StatusConflict, "file_not_managed", "one or more selected files are outside configured upload targets; untrack them instead", nil)
				return
			}
		}
	}

	removed := 0
	for _, file := range files {
		publicID := s.publicFileID(file)
		var changed bool
		if request.Mode == "delete" {
			changed, err = s.deleteManagedFile(r.Context(), publicID)
		} else {
			changed, err = s.deleteFileByPublicID(r.Context(), publicID)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to "+request.Mode+" selected files", nil)
			return
		}
		if changed {
			removed++
		}
	}
	writeJSON(w, http.StatusOK, FileRemovalResponse{
		Mode: request.Mode,
		Selector: FileRemovalSelector{FileIDs: request.FileIDs, Query: request.Query, ExcludeFileIDs: request.ExcludeFileIDs},
		RemovedLocations: removed,
	})
}

func decodeFileRemovalRequest(r *http.Request) (FileRemovalRequest, error) {
	defer r.Body.Close()
	var request FileRemovalRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return request, errors.New("request body is required")
		}
		return request, fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return request, errors.New("request body must contain a single JSON object")
	}
	request.Mode = strings.TrimSpace(request.Mode)
	request.Query = strings.TrimSpace(request.Query)
	request.FileIDs = normalizeStrings(request.FileIDs)
	request.ExcludeFileIDs = normalizeStrings(request.ExcludeFileIDs)
	return request, nil
}

func validateFileRemovalRequest(request FileRemovalRequest) error {
	if request.Mode != "untrack" && request.Mode != "delete" {
		return errors.New("mode must be untrack or delete")
	}
	hasIDs := len(request.FileIDs) > 0
	hasQuery := request.Query != ""
	if hasIDs == hasQuery {
		return errors.New("provide exactly one selector: file_ids or query")
	}
	if len(request.ExcludeFileIDs) > 0 && !hasQuery {
		return errors.New("exclude_file_ids requires a query selector")
	}
	if err := rejectDuplicateFileIDs(request.FileIDs, "file id"); err != nil {
		return err
	}
	if err := rejectDuplicateFileIDs(request.ExcludeFileIDs, "excluded file id"); err != nil {
		return err
	}
	if hasQuery {
		ast, err := query.Parse(request.Query)
		if err != nil {
			return fmt.Errorf("%w: could not parse query: %v", core.ErrInvalidQuery, err)
		}
		if err := query.ValidateAST(ast); err != nil {
			return fmt.Errorf("%w: invalid tag in query: %v", core.ErrInvalidQuery, err)
		}
	}
	return nil
}

func rejectDuplicateFileIDs(ids []string, label string) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate %s %q", label, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (s *Server) resolveFileRemovalSelection(ctx context.Context, request FileRemovalRequest) ([]types.FileInfo, error) {
	if len(request.FileIDs) > 0 {
		files := make([]types.FileInfo, 0, len(request.FileIDs))
		for _, id := range request.FileIDs {
			file, err := s.getFileByPublicID(ctx, id)
			if err != nil {
				return nil, err
			}
			files = append(files, file)
		}
		return files, nil
	}

	excluded := make(map[string]struct{}, len(request.ExcludeFileIDs))
	for _, id := range request.ExcludeFileIDs {
		if _, err := s.getFileByPublicID(ctx, id); err != nil {
			return nil, err
		}
		excluded[id] = struct{}{}
	}
	files, err := s.library.ListFiles(ctx, request.Query)
	if err != nil {
		return nil, err
	}
	selected := make([]types.FileInfo, 0, len(files))
	for _, file := range files {
		if _, skip := excluded[s.publicFileID(file)]; skip {
			continue
		}
		selected = append(selected, file)
	}
	return selected, nil
}
''')

# Backend handler coverage for the scalable query selector and destructive prevalidation.
replace(
    "internal/serve/file_delete_test.go",
    '"context"\n\t"errors"\n',
    '"context"\n\t"encoding/json"\n\t"errors"\n',
)
replace(
    "internal/serve/file_delete_test.go",
    'func TestDeleteModeRemovesManagedUploadFileAndLocation(t *testing.T) {\n',
    r'''func TestBulkUntrackQueryHonorsExclusions(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	page := listTestFiles(t, server, "kind:image", 10)
	if len(page.Files) < 2 {
		t.Fatalf("test fixture needs at least two images, got %d", len(page.Files))
	}
	body := []byte(fmt.Sprintf(`{"mode":"untrack","query":"kind:image","exclude_file_ids":[%q]}`, page.Files[0].ID))
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected bulk untrack 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response FileRemovalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Mode != "untrack" || response.RemovedLocations != len(page.Files)-1 {
		t.Fatalf("unexpected response: %+v", response)
	}

	kept := httptest.NewRecorder()
	server.Handler().ServeHTTP(kept, authedRequest(http.MethodGet, "/api/v1/files/"+page.Files[0].ID))
	if kept.Code != http.StatusOK {
		t.Fatalf("excluded file should remain tracked, got %d: %s", kept.Code, kept.Body.String())
	}
	for _, removed := range page.Files[1:] {
		missing := httptest.NewRecorder()
		server.Handler().ServeHTTP(missing, authedRequest(http.MethodGet, "/api/v1/files/"+removed.ID))
		if missing.Code != http.StatusNotFound {
			t.Fatalf("selected file %s should be untracked, got %d: %s", removed.ID, missing.Code, missing.Body.String())
		}
	}
}

func TestDeleteModeRemovesManagedUploadFileAndLocation(t *testing.T) {
''',
)
# fmt import needed by the new test.
replace(
    "internal/serve/file_delete_test.go",
    '"errors"\n\t"net/http"\n',
    '"errors"\n\t"fmt"\n\t"net/http"\n',
)

# OpenAPI: expose the scalable selection deletion endpoint and generated client types.
replace(
    "docs/openapi.yaml",
    '      responses:\n        "200":\n          description: Page of tracked files.\n',
    '''      responses:
        "200":
          description: Page of tracked files.
''',
)
# Insert DELETE on /files before the /files/{id} path.
replace(
    "docs/openapi.yaml",
    '        "401":\n          $ref: "#/components/responses/Unauthorized"\n  /files/{id}:\n',
    '''        "401":
          $ref: "#/components/responses/Unauthorized"
    delete:
      summary: Untrack or physically delete a selected set of files.
      description: Accepts the same explicit-ID or query-with-exclusions selector used by bulk UI actions. Physical deletion is accepted only when every selected file is inside configured upload targets.
      security:
        - sessionAuth: []
      parameters:
        - $ref: "#/components/parameters/CSRF"
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/FileRemovalRequest"
      responses:
        "200":
          description: Selected file locations were removed.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/FileRemovalResponse"
        "400":
          $ref: "#/components/responses/BadRequest"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"
        "409":
          description: Physical deletion was requested for a selection containing files outside configured upload targets.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ErrorResponse"
  /files/{id}:
''',
)
replace(
    "docs/openapi.yaml",
    '    TagMutationRequest:\n',
    '''    FileRemovalSelector:
      type: object
      properties:
        file_ids:
          type: array
          items:
            type: string
        query:
          type: string
        exclude_file_ids:
          type: array
          items:
            type: string
    FileRemovalRequest:
      allOf:
        - $ref: "#/components/schemas/FileRemovalSelector"
        - type: object
          required: [mode]
          properties:
            mode:
              type: string
              enum: [untrack, delete]
    FileRemovalResponse:
      type: object
      required: [mode, selector, removed_locations]
      properties:
        mode:
          type: string
          enum: [untrack, delete]
        selector:
          $ref: "#/components/schemas/FileRemovalSelector"
        removed_locations:
          type: integer
          minimum: 0
    TagMutationRequest:
''',
)

# Frontend API/types/mutations.
replace(
    "frontend/src/lib/api/types.ts",
    "export type FileListResponse = components['schemas']['FileListResponse'];\n",
    "export type FileListResponse = components['schemas']['FileListResponse'];\nexport type FileRemovalRequest = components['schemas']['FileRemovalRequest'];\nexport type FileRemovalResponse = components['schemas']['FileRemovalResponse'];\n",
)
replace(
    "frontend/src/lib/api/client.ts",
    '  FileItem,\n  FileListResponse,\n',
    '  FileItem,\n  FileListResponse,\n  FileRemovalRequest,\n  FileRemovalResponse,\n',
)
replace(
    "frontend/src/lib/api/client.ts",
    '''  async removeFile(id: string, mode: 'untrack' | 'delete' = 'untrack'): Promise<void> {
    await this.unwrap(
      this.client.DELETE('/files/{id}', {
        params: { header: this.csrfHeaderParam('DELETE'), path: { id } },
        body: { mode }
      })
    );
  }
''',
    '''  async removeFile(id: string, mode: 'untrack' | 'delete' = 'untrack'): Promise<void> {
    await this.unwrap(
      this.client.DELETE('/files/{id}', {
        params: { header: this.csrfHeaderParam('DELETE'), path: { id } },
        body: { mode }
      })
    );
  }

  async removeFiles(body: FileRemovalRequest): Promise<FileRemovalResponse> {
    return this.unwrap<FileRemovalResponse>(
      this.client.DELETE('/files', { params: { header: this.csrfHeaderParam('DELETE') }, body })
    );
  }
''',
)
replace(
    "frontend/src/lib/queries/files.ts",
    "import type { FileListResponse, TagMutationOperation, TagMutationRequest, TagMutationResponse } from '$lib/api/types';\n",
    "import type { FileListResponse, FileRemovalRequest, FileRemovalResponse, TagMutationOperation, TagMutationRequest, TagMutationResponse } from '$lib/api/types';\n",
)
replace(
    "frontend/src/lib/queries/files.ts",
    'export interface FileRemovalVariables {\n',
    '''export function createFilesRemovalMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<FileRemovalResponse, Error, FileRemovalRequest>(() => ({
    mutationFn: (body) => new ApiClient(getCSRFToken()).removeFiles(body),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: fileKeys.all }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.tagsRoot })
      ]);
    }
  }));
}

export interface FileRemovalVariables {
''',
)

# Keyboard action mapping includes Delete / Shift+Delete only when a selection exists.
replace(
    "frontend/src/lib/utils/keyboard.ts",
    "export type LibraryShortcutAction = 'select-all' | 'tag-selected' | 'untag-selected' | null;\n\nexport function libraryShortcutAction(key: string, selectedCount: number): LibraryShortcutAction {\n",
    "export type LibraryShortcutAction = 'select-all' | 'tag-selected' | 'untag-selected' | 'untrack-selected' | 'delete-selected' | null;\n\nexport function libraryShortcutAction(key: string, selectedCount: number, shiftKey = false): LibraryShortcutAction {\n",
)
replace(
    "frontend/src/lib/utils/keyboard.ts",
    "    case 'u': return selectedCount > 0 ? 'untag-selected' : null;\n",
    "    case 'u': return selectedCount > 0 ? 'untag-selected' : null;\n    case 'delete': return selectedCount > 0 ? (shiftKey ? 'delete-selected' : 'untrack-selected') : null;\n",
)
replace(
    "frontend/src/lib/utils/keyboard.test.ts",
    "    expect(libraryShortcutAction('u', 2)).toBe('untag-selected');\n",
    "    expect(libraryShortcutAction('u', 2)).toBe('untag-selected');\n    expect(libraryShortcutAction('Delete', 2)).toBe('untrack-selected');\n    expect(libraryShortcutAction('Delete', 2, true)).toBe('delete-selected');\n",
)
replace(
    "frontend/src/lib/utils/keyboard.test.ts",
    "    expect(libraryShortcutAction('u', 0)).toBeNull();\n",
    "    expect(libraryShortcutAction('u', 0)).toBeNull();\n    expect(libraryShortcutAction('Delete', 0)).toBeNull();\n",
)

# MediaGrid surface actions.
replace(
    "frontend/src/lib/components/MediaGrid.svelte",
    '    onBulkTag,\n    onBulkUntag,\n',
    '    onBulkTag,\n    onBulkUntag,\n    onBulkUntrack,\n    onBulkDelete,\n',
)
replace(
    "frontend/src/lib/components/MediaGrid.svelte",
    '    onBulkTag: () => void;\n    onBulkUntag: () => void;\n',
    '    onBulkTag: () => void;\n    onBulkUntag: () => void;\n    onBulkUntrack: () => void;\n    onBulkDelete: () => void;\n',
)
replace(
    "frontend/src/lib/components/MediaGrid.svelte",
    '''        <button class="g-btn g-btn-sm" type="button" onclick={onBulkUntag}><Icon name="trash" size={13} /> <u>U</u>ntag…</button>
        <button class="g-btn g-btn-sm g-btn-icon" type="button" title="Clear" aria-label="Clear selection" onclick={onClearSelection}><Icon name="close" size={13} /></button>
''',
    '''        <button class="g-btn g-btn-sm" type="button" onclick={onBulkUntag}><Icon name="trash" size={13} /> <u>U</u>ntag…</button>
        <button class="g-btn g-btn-sm" type="button" onclick={onBulkUntrack}>Untrack…</button>
        <button class="g-btn g-btn-sm" type="button" onclick={onBulkDelete}><Icon name="trash" size={13} /> Delete…</button>
        <button class="g-btn g-btn-sm g-btn-icon" type="button" title="Clear" aria-label="Clear selection" onclick={onClearSelection}><Icon name="close" size={13} /></button>
''',
)

# AuthenticatedApp bulk dialogs/mutation/shortcuts. Narrow tag-dialog detection so removal dialogs have no tag input.
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "import { createFilesQuery, createFileRemovalMutation, createTagMutation, pageTokenOffset, type FileSort } from '$lib/queries/files';\n",
    "import { createFilesQuery, createFileRemovalMutation, createFilesRemovalMutation, createTagMutation, pageTokenOffset, type FileSort } from '$lib/queries/files';\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "import { createLibraryWorkflow } from '$lib/state/libraryWorkflow.svelte';\n",
    "import { createLibraryWorkflow } from '$lib/state/libraryWorkflow.svelte';\n  import { selectionRequest } from '$lib/state/selection';\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "    kind: 'none' | 'save-create' | 'save-update' | 'save-delete' | 'bulk-selected' | 'bulk-remove-selected' | 'untrack-file' | 'delete-file';\n",
    "    kind: 'none' | 'save-create' | 'save-update' | 'save-delete' | 'bulk-selected' | 'bulk-remove-selected' | 'bulk-untrack-selected' | 'bulk-delete-selected' | 'untrack-file' | 'delete-file';\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "  const fileRemovalMutation = createFileRemovalMutation(() => $authState.csrfToken, queryClient);\n",
    "  const fileRemovalMutation = createFileRemovalMutation(() => $authState.csrfToken, queryClient);\n  const filesRemovalMutation = createFilesRemovalMutation(() => $authState.csrfToken, queryClient);\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "      const action = libraryShortcutAction(event.key, selectedCount);\n",
    "      const action = libraryShortcutAction(event.key, selectedCount, event.shiftKey);\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "        if (action === 'select-all') library.selectAll();\n        else if (action === 'tag-selected') bulkTagSelected();\n        else bulkUntagSelected();\n",
    "        if (action === 'select-all') library.selectAll();\n        else if (action === 'tag-selected') bulkTagSelected();\n        else if (action === 'untag-selected') bulkUntagSelected();\n        else if (action === 'untrack-selected') bulkUntrackSelected();\n        else bulkDeleteSelected();\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    '''  function bulkUntagSelected() {
    actionDialog = { kind: 'bulk-remove-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }


  async function submitActionDialog() {
''',
    '''  function bulkUntagSelected() {
    actionDialog = { kind: 'bulk-remove-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  function bulkUntrackSelected() {
    actionDialog = { kind: 'bulk-untrack-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  function bulkDeleteSelected() {
    actionDialog = { kind: 'bulk-delete-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  function isBulkTagDialog(kind = actionDialog.kind) {
    return kind === 'bulk-selected' || kind === 'bulk-remove-selected';
  }

  async function submitActionDialog() {
''',
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "    if ((actionDialog.kind === 'save-create' || actionDialog.kind === 'save-update' || actionDialog.kind.startsWith('bulk-')) && !value) {\n      actionDialog = { ...actionDialog, error: actionDialog.kind.startsWith('bulk-') ? 'Enter at least one tag.' : 'Enter a name.' };\n",
    "    if ((actionDialog.kind === 'save-create' || actionDialog.kind === 'save-update' || isBulkTagDialog()) && !value) {\n      actionDialog = { ...actionDialog, error: isBulkTagDialog() ? 'Enter at least one tag.' : 'Enter a name.' };\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    '''      } else if (actionDialog.kind === 'untrack-file' || actionDialog.kind === 'delete-file') {
        await fileRemovalMutation.mutateAsync({ id: actionDialog.id, mode: actionDialog.kind === 'delete-file' ? 'delete' : 'untrack' });
        if (library.activeFile?.id === actionDialog.id) library.closePreview();
      }
''',
    '''      } else if (actionDialog.kind === 'bulk-untrack-selected' || actionDialog.kind === 'bulk-delete-selected') {
        await filesRemovalMutation.mutateAsync({
          ...selectionRequest(library.selection),
          mode: actionDialog.kind === 'bulk-delete-selected' ? 'delete' : 'untrack'
        });
        library.clearSelection();
      } else if (actionDialog.kind === 'untrack-file' || actionDialog.kind === 'delete-file') {
        await fileRemovalMutation.mutateAsync({ id: actionDialog.id, mode: actionDialog.kind === 'delete-file' ? 'delete' : 'untrack' });
        if (library.activeFile?.id === actionDialog.id) library.closePreview();
      }
''',
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "      case 'bulk-remove-selected': return 'Untag selected files';\n",
    "      case 'bulk-remove-selected': return 'Untag selected files';\n      case 'bulk-untrack-selected': return 'Untrack selected files';\n      case 'bulk-delete-selected': return 'Delete selected files';\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "      case 'bulk-remove-selected': return `Remove tags from ${selectedCount} selected file${selectedCount === 1 ? '' : 's'}.`;\n",
    "      case 'bulk-remove-selected': return `Remove tags from ${selectedCount} selected file${selectedCount === 1 ? '' : 's'}.`;\n      case 'bulk-untrack-selected': return `Untrack ${selectedCount} selected file${selectedCount === 1 ? '' : 's'} from the library. Files remain on disk.`;\n      case 'bulk-delete-selected': return `Permanently delete ${selectedCount} selected file${selectedCount === 1 ? '' : 's'} from disk and remove them from the library. Only files in managed upload targets can be deleted.`;\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "    return actionDialog.kind.startsWith('bulk-') ? 'Tags' : 'Name';\n",
    "    return isBulkTagDialog() ? 'Tags' : 'Name';\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "      case 'bulk-remove-selected': return 'Remove tags';\n",
    "      case 'bulk-remove-selected': return 'Remove tags';\n      case 'bulk-untrack-selected': return 'Untrack';\n      case 'bulk-delete-selected': return 'Delete files';\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "        onBulkUntag={bulkUntagSelected}\n",
    "        onBulkUntag={bulkUntagSelected}\n        onBulkUntrack={bulkUntrackSelected}\n        onBulkDelete={bulkDeleteSelected}\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "      destructive={actionDialog.kind === 'save-delete' || actionDialog.kind === 'untrack-file' || actionDialog.kind === 'delete-file'}\n",
    "      destructive={actionDialog.kind === 'save-delete' || actionDialog.kind === 'bulk-untrack-selected' || actionDialog.kind === 'bulk-delete-selected' || actionDialog.kind === 'untrack-file' || actionDialog.kind === 'delete-file'}\n",
)
replace(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    "      input={actionDialog.kind !== 'save-delete' && actionDialog.kind !== 'untrack-file' && actionDialog.kind !== 'delete-file'}\n      tagInput={actionDialog.kind.startsWith('bulk-')}\n",
    "      input={actionDialog.kind !== 'save-delete' && actionDialog.kind !== 'bulk-untrack-selected' && actionDialog.kind !== 'bulk-delete-selected' && actionDialog.kind !== 'untrack-file' && actionDialog.kind !== 'delete-file'}\n      tagInput={isBulkTagDialog()}\n",
)

# E2E regression: real keyboard -> dialog -> scalable selector request.
replace(
    "frontend/tests/shortcut-focus.spec.ts",
    "test('Escape clears selection even when the select-all checkbox owns focus', async ({ page }) => {\n",
    r'''test('Delete and Shift+Delete remove a query-wide selection through confirmation dialogs', async ({ page }) => {
	await mockApp(page);
	const removals: unknown[] = [];
	await page.route('**/api/v1/files', async (route) => {
		if (route.request().method() !== 'DELETE') return route.fallback();
		removals.push(route.request().postDataJSON());
		await route.fulfill({
			contentType: 'application/json',
			body: JSON.stringify({ mode: (removals.at(-1) as { mode: string }).mode, selector: removals.at(-1), removed_locations: 3 })
		});
	});

	const selectAll = page.getByLabel('Select all files in current view');
	await selectAll.click();
	await expect(page.getByRole('button', { name: 'Untrack…' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Delete…' })).toBeVisible();

	await page.keyboard.press('Delete');
	await expect(page.getByRole('dialog', { name: 'Untrack selected files' })).toBeVisible();
	await page.getByRole('button', { name: 'Untrack', exact: true }).click();
	await expect.poll(() => removals.length).toBe(1);
	expect(removals[0]).toEqual({ mode: 'untrack', query: '*' });
	await expect(page.getByText('3 of 3 selected')).toHaveCount(0);

	await selectAll.click();
	await page.keyboard.press('Shift+Delete');
	await expect(page.getByRole('dialog', { name: 'Delete selected files' })).toBeVisible();
	await page.getByRole('button', { name: 'Delete files' }).click();
	await expect.poll(() => removals.length).toBe(2);
	expect(removals[1]).toEqual({ mode: 'delete', query: '*' });
});

test('Escape clears selection even when the select-all checkbox owns focus', async ({ page }) => {
''',
)
