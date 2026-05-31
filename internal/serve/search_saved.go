package serve

import (
	"crypto/rand"
	"encoding/hex"
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

type SuggestionsResponse struct {
	Items []TagDTO `json:"items"`
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

func (s *Server) handleSearchSuggestions(w http.ResponseWriter, r *http.Request) {
	search, ok := s.library.(SearchLibrary)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "search service is not configured", nil)
		return
	}
	limit, _ := strconvAtoiDefault(r.URL.Query().Get("limit"), 20)
	items, err := search.TagSuggestions(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load suggestions", nil)
		return
	}
	writeJSON(w, http.StatusOK, SuggestionsResponse{Items: items})
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
	library, ok := s.library.(*GooruLibrary)
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
		items, err := library.client.ListSavedSearches(userID)
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
		item, err := library.client.UpsertSavedSearch(types.SavedSearch{ID: newSavedSearchID(), UserID: userID, Name: req.Name, Query: req.Query, Sort: req.Sort, Order: req.Order})
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
	library, ok := s.library.(*GooruLibrary)
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
	switch r.Method {
	case http.MethodPut:
		req, err := decodeSavedSearchRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		item, err := library.client.UpsertSavedSearch(types.SavedSearch{ID: id, UserID: userID, Name: req.Name, Query: req.Query, Sort: req.Sort, Order: req.Order})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to save search", nil)
			return
		}
		writeJSON(w, http.StatusOK, savedSearchDTO(item))
	case http.MethodDelete:
		ok, err := library.client.DeleteSavedSearch(userID, id)
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

func newSavedSearchID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "srch_" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return "srch_" + hex.EncodeToString(b[:])
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
