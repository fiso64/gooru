package serve

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/query"
)

const (
	fileSelectionTTL            = 30 * time.Minute
	maxFileSelectionSnapshots   = 256
	maxFileSelectionRetainedIDs = 2_000_000
)

var errFileSelectionNotFound = errors.New("file selection not found")

type fileSelectionSnapshot struct {
	OwnerID   string
	FileIDs   []string
	ExpiresAt time.Time
}

type fileSelectionStore struct {
	mu             sync.Mutex
	snapshots      map[string]fileSelectionSnapshot
	totalFileIDs   int
	maxSnapshots   int
	maxRetainedIDs int
	now            func() time.Time
}

func newFileSelectionStore() *fileSelectionStore {
	return &fileSelectionStore{
		snapshots:      make(map[string]fileSelectionSnapshot),
		maxSnapshots:   maxFileSelectionSnapshots,
		maxRetainedIDs: maxFileSelectionRetainedIDs,
		now:            func() time.Time { return time.Now().UTC() },
	}
}

func (s *fileSelectionStore) create(ownerID string, fileIDs []string) (string, int, error) {
	idBytes := make([]byte, 24)
	if _, err := rand.Read(idBytes); err != nil {
		return "", 0, err
	}
	id := base64.RawURLEncoding.EncodeToString(idBytes)

	ids := append([]string(nil), fileIDs...)
	sort.Strings(ids)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	for len(s.snapshots) >= s.maxSnapshots && len(s.snapshots) > 0 {
		s.evictOldestLocked()
	}
	// The aggregate budget limits amplification from many simultaneous large
	// snapshots, but a single selection is always allowed to exceed it. That
	// keeps Select all usable for libraries with several million files while
	// bounding retained memory to the larger of one snapshot or the soft budget.
	for s.totalFileIDs+len(ids) > s.maxRetainedIDs && len(s.snapshots) > 0 {
		s.evictOldestLocked()
	}
	s.snapshots[id] = fileSelectionSnapshot{
		OwnerID:   ownerID,
		FileIDs:   ids,
		ExpiresAt: s.now().Add(fileSelectionTTL),
	}
	s.totalFileIDs += len(ids)
	return id, len(ids), nil
}

func (s *fileSelectionStore) resolve(ownerID, id string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	snapshot, ok := s.snapshots[id]
	if !ok || snapshot.OwnerID != ownerID {
		return nil, errFileSelectionNotFound
	}
	return append([]string(nil), snapshot.FileIDs...), nil
}

func (s *fileSelectionStore) members(ownerID, id string, candidates []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	snapshot, ok := s.snapshots[id]
	if !ok || snapshot.OwnerID != ownerID {
		return nil, errFileSelectionNotFound
	}
	matched := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		index := sort.SearchStrings(snapshot.FileIDs, candidate)
		if index < len(snapshot.FileIDs) && snapshot.FileIDs[index] == candidate {
			matched = append(matched, candidate)
		}
	}
	return matched, nil
}

func (s *fileSelectionStore) remove(ownerID, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, ok := s.snapshots[id]
	if ok && snapshot.OwnerID == ownerID {
		s.deleteLocked(id, snapshot)
	}
}

func (s *fileSelectionStore) sweepLocked() {
	now := s.now()
	for id, snapshot := range s.snapshots {
		if !snapshot.ExpiresAt.After(now) {
			s.deleteLocked(id, snapshot)
		}
	}
}

func (s *fileSelectionStore) evictOldestLocked() {
	var oldestID string
	var oldestSnapshot fileSelectionSnapshot
	for id, snapshot := range s.snapshots {
		if oldestID == "" || snapshot.ExpiresAt.Before(oldestSnapshot.ExpiresAt) {
			oldestID = id
			oldestSnapshot = snapshot
		}
	}
	if oldestID != "" {
		s.deleteLocked(oldestID, oldestSnapshot)
	}
}

func (s *fileSelectionStore) deleteLocked(id string, snapshot fileSelectionSnapshot) {
	delete(s.snapshots, id)
	s.totalFileIDs -= len(snapshot.FileIDs)
	if s.totalFileIDs < 0 {
		s.totalFileIDs = 0
	}
}

type fileSelectionCreateRequest struct {
	Query string `json:"query"`
}

type fileSelectionCreateResponse struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type fileSelectionMembersRequest struct {
	FileIDs []string `json:"file_ids"`
}

type fileSelectionMembersResponse struct {
	FileIDs []string `json:"file_ids"`
}

func (s *Server) handleFileSelections(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/file-selections" {
		writeError(w, http.StatusNotFound, "not_found", "file selection not found", nil)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	selectionLibrary, ok := s.library.(FileSelectionLibrary)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file selection service is not configured", nil)
		return
	}

	var request fileSelectionCreateRequest
	if err := decodeSingleJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query != "" {
		if err := validateFileSelectionQuery(request.Query); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
			return
		}
	}

	fileIDs, err := selectionLibrary.ListPublicFileIDs(r.Context(), request.Query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve file selection", nil)
		return
	}
	id, count, err := s.fileSelections.create(fileSelectionOwnerID(r), fileIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create file selection", nil)
		return
	}
	writeJSON(w, http.StatusCreated, fileSelectionCreateResponse{ID: id, Count: count})
}

func (s *Server) handleFileSelection(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/file-selections/")
	if path == "" {
		writeError(w, http.StatusNotFound, "not_found", "file selection not found", nil)
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	ownerID := fileSelectionOwnerID(r)

	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.fileSelections.remove(ownerID, id)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 2 && parts[1] == "members" && r.Method == http.MethodPost {
		var request fileSelectionMembersRequest
		if err := decodeSingleJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		request.FileIDs = normalizeStrings(request.FileIDs)
		members, err := s.fileSelections.members(ownerID, id, request.FileIDs)
		if errors.Is(err, errFileSelectionNotFound) {
			writeError(w, http.StatusGone, "selection_expired", "file selection has expired", nil)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve file selection membership", nil)
			return
		}
		writeJSON(w, http.StatusOK, fileSelectionMembersResponse{FileIDs: members})
		return
	}

	if len(parts) == 1 {
		w.Header().Set("Allow", "DELETE")
	} else if len(parts) == 2 && parts[1] == "members" {
		w.Header().Set("Allow", "POST")
	}
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
}

func validateFileSelectionQuery(expression string) error {
	ast, err := query.Parse(expression)
	if err != nil {
		return errors.Join(core.ErrInvalidQuery, err)
	}
	if err := query.ValidateAST(ast); err != nil {
		return errors.Join(core.ErrInvalidQuery, err)
	}
	return nil
}

func fileSelectionOwnerID(r *http.Request) string {
	if auth, ok := currentAuth(r.Context()); ok {
		return auth.User.ID
	}
	return ""
}

func decodeSingleJSON(r *http.Request, destination interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}
