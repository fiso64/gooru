package serve

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	cfg     Config
	jobs    *JobManager
	library Library
	media   *MediaService
	meta    MediaMetadataProvider
	auth    *AuthStore
}

func NewServer(cfg Config) *Server {
	return NewServerWithLibrary(cfg, nil)
}

func NewServerWithLibrary(cfg Config, library Library) *Server {
	return &Server{
		cfg:     cfg,
		jobs:    NewJobManagerWithLimits(cfg.Jobs.MaxQueued, cfg.Jobs.MaxRunning, cfg.Jobs.MaxResultBytes, cfg.Jobs.CompletedTTL),
		library: library,
		media:   NewMediaService(cfg),
		meta:    BasicMediaMetadataProvider{},
	}
}

func (s *Server) SetAuthStore(store *AuthStore) {
	s.auth = store
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:         s.cfg.Server.Listen,
		Handler:      s.Handler(),
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
		IdleTimeout:  s.cfg.Server.IdleTimeout,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", methodHandler(http.MethodGet, s.handleHealth))
	mux.HandleFunc("/api/v1/auth/login", s.handleAuthLogin)
	mux.Handle("/api/v1/auth/logout", s.protected(http.HandlerFunc(s.handleAuthLogout)))
	mux.Handle("/api/v1/auth/me", authMiddleware(s.cfg, s.auth, http.HandlerFunc(s.handleAuthMe)))
	mux.Handle("/api/v1/auth/change-password", s.protected(http.HandlerFunc(s.handleChangePassword)))
	mux.Handle("/api/v1/upload-targets", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleUploadTargets)))
	mux.Handle("/api/v1/uploads", s.protected(http.HandlerFunc(s.handleUpload)))
	mux.Handle("/api/v1/files/tags", s.protected(http.HandlerFunc(s.handleMutateTags)))
	mux.Handle("/api/v1/files/", s.protected(http.HandlerFunc(s.handleFile)))
	mux.Handle("/api/v1/files", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleListFiles)))
	mux.Handle("/api/v1/search/suggestions", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleSearchSuggestions)))
	mux.Handle("/api/v1/saved-searches/", s.protected(http.HandlerFunc(s.handleSavedSearch)))
	mux.Handle("/api/v1/saved-searches", s.protected(http.HandlerFunc(s.handleSavedSearches)))
	mux.Handle("/api/v1/tags/namespaces", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleTagNamespaces)))
	mux.Handle("/api/v1/tags", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleListTags)))
	mux.Handle("/api/v1/jobs", s.protected(http.HandlerFunc(s.handleJobs)))
	mux.Handle("/api/v1/jobs/", s.protected(http.HandlerFunc(s.handleJob)))
	mux.HandleFunc("/", s.handleFrontend)

	var h http.Handler = mux
	h = securityHeadersMiddleware(h)
	h = requestSizeMiddleware(s.cfg.Server.MaxRequestBodyBytes, h)
	h = corsMiddleware(s.cfg.Server.CORSOrigins, h)
	return h
}

func (s *Server) protected(next http.Handler) http.Handler {
	return authMiddleware(s.cfg, s.auth, csrfMiddleware(s.cfg, s.auth, next))
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	server := s.HTTPServer()
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

type JobListResponse struct {
	Items []*Job `json:"items"`
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !JobStatus(status).Valid() {
		writeError(w, http.StatusBadRequest, "invalid_request", "status is invalid", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, JobListResponse{Items: s.jobs.List(status)})
	case http.MethodDelete:
		if status == "" {
			status = string(JobCompleted)
		}
		if status == string(JobPending) || status == string(JobRunning) {
			writeError(w, http.StatusBadRequest, "invalid_request", "only finished jobs can be cleared", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"removed": s.jobs.Clear(status)})
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetJob(w, r, id)
	case http.MethodDelete:
		s.handleCancelJob(w, r, id)
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request, id string) {
	job, ok := s.jobs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request, id string) {
	job, ok := s.jobs.Cancel(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "resource not found", nil)
}

func PreferAsync(r *http.Request) bool {
	for _, value := range r.Header.Values("Prefer") {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "respond-async") {
				return true
			}
		}
	}
	return false
}

func methodHandler(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		next(w, r)
	}
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: data:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func writeJobSubmitError(w http.ResponseWriter, err error, fallback string) bool {
	switch {
	case errors.Is(err, ErrJobQueueFull):
		writeError(w, http.StatusServiceUnavailable, "job_queue_full", "job queue is full", nil)
		return true
	case errors.Is(err, context.Canceled):
		writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
		return true
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", fallback, nil)
		return true
	}
}
