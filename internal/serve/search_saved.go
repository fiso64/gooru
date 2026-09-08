package serve

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/query"
	"gooru.local/types"
)

type MetaTagDTO struct {
	Name          string `json:"name"`
	Syntax        string `json:"syntax"`
	Hint          string `json:"hint"`
	RequiresValue bool   `json:"requires_value"`
}

type SuggestionsResponse struct {
	Items    []TagDTO     `json:"items"`
	MetaTags []MetaTagDTO `json:"meta_tags"`
}

type NamespacesResponse struct {
	Items []string `json:"items"`
}

type SavedSearchDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	Sort      string    `json:"sort"`
	Order     string    `json:"order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SavedSearchesResponse struct {
	Items []SavedSearchDTO `json:"items"`
}

type savedSearchRequest struct {
	Name  string `json:"name"`
	Query string `json:"query"`
	Sort  string `json:"sort"`
	Order string `json:"order"`
}

type savedSearchReorderRequest struct {
	IDs []string `json:"ids"`
}

type SavedSearchLibrary interface {
	ListSavedSearches(ctx context.Context, userID string) ([]types.SavedSearch, error)
	CreateSavedSearch(ctx context.Context, userID string, req savedSearchRequest) (types.SavedSearch, error)
	UpdateSavedSearch(ctx context.Context, userID string, id string, req savedSearchRequest) (types.SavedSearch, error)
	ReorderSavedSearches(ctx context.Context, userID string, ids []string) error
	DeleteSavedSearch(ctx context.Context, userID string, id string) (bool, error)
}

func metaTagDTOs() []MetaTagDTO {
	definitions := query.MetaTags()
	items := make([]MetaTagDTO, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, MetaTagDTO{Name: definition.Name, Syntax: definition.Syntax, Hint: definition.Hint, RequiresValue: definition.RequiresValue})
	}
	return items
}

func matchingMetaTagSuggestions(prefix string) []TagDTO {
	candidate := strings.ToLower(strings.TrimSpace(prefix))
	candidate = strings.TrimPrefix(candidate, "-")
	if !strings.HasPrefix(candidate, "@") {
		return nil
	}
	items := make([]TagDTO, 0, len(query.MetaTags()))
	for _, definition := range query.MetaTags() {
		syntax := strings.ToLower(definition.Syntax)
		if strings.HasPrefix(syntax, candidate) || (definition.RequiresValue && strings.HasPrefix(candidate, syntax)) {
			items = append(items, TagDTO{Name: definition.Syntax, Value: definition.Hint})
		}
	}
	return items
}

type suggestionRequest struct {
	Query    string `json:"q"`
	Existing string `json:"existing"`
	Limit    int    `json:"limit"`
}

func (s *Server) handleSearchSuggestions(w http.ResponseWriter, r *http.Request) {
	var req suggestionRequest
	switch r.Method {
	case http.MethodGet:
		req.Query = r.URL.Query().Get("q")
		req.Existing = r.URL.Query().Get("existing")
		req.Limit, _ = strconvAtoiDefault(r.URL.Query().Get("limit"), 20)
	case http.MethodPost:
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request body", nil)
			return
		}
		if req.Limit == 0 {
			req.Limit = 20
		}
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	search, ok := s.library.(SearchLibrary)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "search service is not configured", nil)
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	prefix := strings.TrimSpace(req.Query)
	savedItems, err := s.savedSearchNameSuggestionsForRequest(r.Context(), prefix, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load saved search suggestions", nil)
		return
	}
	items, err := search.TagSuggestions(r.Context(), prefix, strings.TrimSpace(req.Existing), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load suggestions", nil)
		return
	}
	items = append(savedItems, append(matchingMetaTagSuggestions(prefix), items...)...)
	if len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, SuggestionsResponse{Items: items, MetaTags: metaTagDTOs()})
}

func (s *Server) handleTagNamespaces(w http.ResponseWriter, r *http.Request) {
	search, ok := s.library.(SearchLibrary)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "tag service is not configured", nil)
		return
	}
	items, err := search.TagNamespaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load tag namespaces", nil)
		return
	}
	writeJSON(w, http.StatusOK, NamespacesResponse{Items: items})
}

func (s *Server) handleSavedSearches(w http.ResponseWriter, r *http.Request) {
	library, ok := s.library.(SavedSearchLibrary)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "saved search service is not configured", nil)
		return
	}
	auth, ok := currentAuth(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required", nil)
		return
	}
	userID := auth.User.ID
	switch r.Method {
	case http.MethodGet:
		items, err := library.ListSavedSearches(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load saved searches", nil)
			return
		}
		writeJSON(w, http.StatusOK, SavedSearchesResponse{Items: savedSearchDTOs(items)})
	case http.MethodPost:
		req, err := decodeSavedSearchRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		item, err := library.CreateSavedSearch(r.Context(), userID, req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to save search", nil)
			return
		}
		writeJSON(w, http.StatusCreated, savedSearchDTO(item))
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleSavedSearch(w http.ResponseWriter, r *http.Request) {
	library, ok := s.library.(SavedSearchLibrary)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "saved search service is not configured", nil)
		return
	}
	auth, ok := currentAuth(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required", nil)
		return
	}
	userID := auth.User.ID
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/saved-searches/"), "/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not_found", "saved search not found", nil)
		return
	}
	if id == "reorder" {
		handleSavedSearchReorder(w, r, library, userID)
		return
	}
	switch r.Method {
	case http.MethodPut:
		req, err := decodeSavedSearchRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		item, err := library.UpdateSavedSearch(r.Context(), userID, id, req)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "not_found", "saved search not found", nil)
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to save search", nil)
			return
		}
		writeJSON(w, http.StatusOK, savedSearchDTO(item))
	case http.MethodDelete:
		ok, err := library.DeleteSavedSearch(r.Context(), userID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete saved search", nil)
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "saved search not found", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "PUT, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func handleSavedSearchReorder(w http.ResponseWriter, r *http.Request, library SavedSearchLibrary, userID string) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", "PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	var req savedSearchReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IDs == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "ids is required", nil)
		return
	}
	if err := library.ReorderSavedSearches(r.Context(), userID, req.IDs); err != nil {
		if errors.Is(err, core.ErrInvalidSavedSearchOrder) {
			writeError(w, http.StatusBadRequest, "invalid_request", "saved search order must contain every saved search exactly once", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to reorder saved searches", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func decodeSavedSearchRequest(r *http.Request) (savedSearchRequest, error) {
	var req savedSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, errors.New("invalid JSON request body")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Query = strings.TrimSpace(req.Query)
	req.Sort = normalizeFileSort(req.Sort)
	req.Order = normalizeSortOrder(req.Order)
	if req.Name == "" {
		return req, errors.New("name is required")
	}
	if req.Query != "" {
		ast, err := query.Parse(req.Query)
		if err != nil {
			return req, core.ErrInvalidQuery
		}
		if err := query.ValidateAST(ast); err != nil {
			return req, core.ErrInvalidQuery
		}
	}
	return req, nil
}

func savedSearchDTOs(items []types.SavedSearch) []SavedSearchDTO {
	out := make([]SavedSearchDTO, 0, len(items))
	for _, item := range items {
		out = append(out, savedSearchDTO(item))
	}
	return out
}

func savedSearchDTO(item types.SavedSearch) SavedSearchDTO {
	return SavedSearchDTO{
		ID:        item.ID,
		Name:      item.Name,
		Query:     item.Query,
		Sort:      item.Sort,
		Order:     item.Order,
		CreatedAt: time.Unix(item.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(item.UpdatedAt, 0).UTC(),
	}
}

func strconvAtoiDefault(raw string, fallback int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback, err
	}
	return value, nil
}
