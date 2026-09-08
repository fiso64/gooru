package serve

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gooru.local/internal/managedfile"
)

const metadataRequestBodyLimit int64 = 1 << 20

type Server struct {
	cfg                  Config
	jobs                 *JobManager
	library              Library
	media                *MediaService
	meta                 MediaMetadataProvider
	auth                 *AuthStore
	urlState             *urlStateCodec
	fileSelections       *fileSelectionStore
	managedFiles         *managedfile.Writer
	backgroundContent    contentHashLibrary
	backgroundOperations backgroundOperationReader
}

func NewServer(cfg Config) *Server {
	return NewServerWithLibrary(cfg, nil)
}

func NewServerWithLibrary(cfg Config, library Library) *Server {
	metadata := NewMediaMetadataProvider(cfg)
	media := newComposedMediaServiceFromConfig(cfg)
	var backgroundContent contentHashLibrary
	var backgroundOperations backgroundOperationReader
	if gooruLibrary, ok := library.(*GooruLibrary); ok {
		backgroundContent = gooruLibrary
		backgroundOperations = gooruLibrary
		gooruLibrary.backgroundTasks = media.backgroundTaskRequests
		gooruLibrary.metadata = metadata
		gooruLibrary.encryption = cfg.Encryption
		if err := gooruLibrary.configureManagedUploadRoots(cfg.Uploads.Targets); err != nil {
			slog.Error("failed to reconcile managed upload roots", "error", err)
		}
		if len(cfg.UI.HiddenTags) > 0 {
			library = newHiddenTagLibrary(gooruLibrary, cfg.UI.HiddenTags)
		}
	}
	managedFiles := managedfile.NewFilesystem()
	if cfg.Encryption.Enabled {
		managedFiles = managedfile.NewProtected(cfg.Encryption.Key)
	}
	return &Server{
		cfg:                  cfg,
		jobs:                 NewJobManagerWithLimits(cfg.Jobs.MaxQueued, cfg.Jobs.MaxRunning, cfg.Jobs.MaxResultBytes, cfg.Jobs.CompletedTTL),
		library:              library,
		media:                media,
		meta:                 metadata,
		urlState:             newURLStateCodec(cfg),
		fileSelections:       newFileSelectionStore(),
		managedFiles:         managedFiles,
		backgroundContent:    backgroundContent,
		backgroundOperations: backgroundOperations,
	}
}

func (s *Server) SetAuthStore(store *AuthStore) {
	s.auth = store
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:              s.cfg.Server.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: s.cfg.Server.ReadTimeout,
		WriteTimeout:      s.cfg.Server.WriteTimeout,
		IdleTimeout:       s.cfg.Server.IdleTimeout,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", methodHandler(http.MethodGet, s.handleHealth))
	mux.HandleFunc("/api/v1/ui-config", methodHandler(http.MethodGet, s.handleUIConfig))
	mux.Handle("/api/v1/auth/login", requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleAuthLogin)))
	mux.Handle("/api/v1/auth/logout", s.protected(http.HandlerFunc(s.handleAuthLogout)))
	mux.Handle("/api/v1/auth/me", authMiddleware(s.cfg, s.auth, http.HandlerFunc(s.handleAuthMe)))
	mux.Handle("/api/v1/auth/change-password", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleChangePassword))))
	mux.Handle("/api/v1/ui-state/", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleUIState))))
	mux.Handle("/api/v1/ui-state", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleUIState))))
	mux.Handle("/api/v1/upload-targets", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleUploadTargets)))
	mux.Handle("/api/v1/uploads", s.adminProtected(http.HandlerFunc(s.handleUpload)))
	mux.Handle("/api/v1/file-selections/", s.adminProtected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFileSelection))))
	mux.Handle("/api/v1/file-selections", s.adminProtected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFileSelections))))
	mux.Handle("/api/v1/files/tags", s.adminProtected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleMutateTags))))
	mux.Handle("/api/v1/files/", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFile))))
	mux.Handle("/api/v1/files", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFiles))))
	mux.Handle("/api/v1/files/search", authMiddleware(s.cfg, s.auth, requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFileSearch))))
	mux.Handle("/api/v1/comics/", s.protected(http.HandlerFunc(s.handleComic)))
	mux.Handle("/api/v1/search/suggestions", authMiddleware(s.cfg, s.auth, requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleSearchSuggestions))))
	mux.Handle("/api/v1/saved-searches/", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleSavedSearch))))
	mux.Handle("/api/v1/saved-searches", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleSavedSearches))))
	mux.Handle("/api/v1/tags/namespaces", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleTagNamespaces)))
	mux.Handle("/api/v1/tags", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleListTags)))
	mux.Handle("/api/v1/operations", s.adminProtected(http.HandlerFunc(s.handleOperations)))
	mux.Handle("/api/v1/operations/", s.adminProtected(http.HandlerFunc(s.handleOperation)))
	mux.Handle("/api/v1/jobs", s.adminProtected(http.HandlerFunc(s.handleJobs)))
	mux.Handle("/api/v1/jobs/", s.adminProtected(http.HandlerFunc(s.handleJob)))
	mux.HandleFunc("/", s.handleFrontend)

	var h http.Handler = mux
	h = securityHeadersMiddleware(h)
	h = protectedAPICacheMiddleware(s.cfg.Encryption.Enabled, h)
	h = requestSizeMiddleware(s.cfg.Server.MaxRequestBodyBytes, h)
	h = corsMiddleware(s.cfg.Server.CORSOrigins, h)
	h = requestLoggingMiddleware(h)
	h = requestReadTimeoutMiddleware(s.cfg.Server.ReadTimeout, h)
	return h
}

func (s *Server) protected(next http.Handler) http.Handler {
	return authMiddleware(s.cfg, s.auth, csrfMiddleware(s.cfg, s.auth, next))
}

func (s *Server) adminProtected(next http.Handler) http.Handler {
	return authMiddleware(s.cfg, s.auth, csrfMiddleware(s.cfg, s.auth, adminMiddleware(s.cfg, next)))
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.cfg.Auth.Enabled {
		return true
	}
	auth, ok := currentAuth(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required", nil)
		return false
	}
	if auth.User.Role != adminRole {
		writeError(w, http.StatusForbidden, "forbidden", "admin privileges required", nil)
		return false
	}
	return true
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
	Items         []*Job `json:"items"`
	ActiveCount   int    `json:"active_count"`
	NextPageToken string `json:"next_page_token,omitempty"`
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !JobStatus(status).Valid() {
		writeError(w, http.StatusBadRequest, "invalid_request", "status is invalid", nil)
		return
	}
	ids, err := jobIDsFromQuery(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if len(ids) > 0 {
			writeJSON(w, http.StatusOK, JobListResponse{Items: s.jobs.ListIDs(ids, status), ActiveCount: s.jobs.ActiveCount()})
			return
		}
		page, err := ParsePage(r.URL.Query().Get("limit"), r.URL.Query().Get("page_token"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		result, activeCount := s.jobs.ListPage(status, page)
		writeJSON(w, http.StatusOK, JobListResponse{Items: result.Items, ActiveCount: activeCount, NextPageToken: result.NextPageToken})
	case http.MethodDelete:
		if len(ids) > 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "job id filtering is only supported for GET", nil)
			return
		}
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
		header.Set("Content-Security-Policy", contentSecurityPolicy(nil))
		next.ServeHTTP(w, r)
	})
}

func contentSecurityPolicy(scriptHashes []string) string {
	scriptSrc := "script-src 'self'"
	for _, hash := range scriptHashes {
		scriptSrc += " 'sha256-" + hash + "'"
	}
	return "default-src 'self'; img-src 'self' blob: data:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; " + scriptSrc + "; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
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

// requestReadTimeoutMiddleware treats ServerConfig.ReadTimeout as an inactivity
// limit for request bodies. http.Server.ReadTimeout is an absolute deadline from
// accept through the entire body, which makes healthy large uploads fail merely
// because they take longer than the timeout. Header reads retain the same hard
// limit through ReadHeaderTimeout; each body read refreshes the network deadline.
func requestReadTimeoutMiddleware(timeout time.Duration, next http.Handler) http.Handler {
	if timeout <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controller := http.NewResponseController(w)
		// http.Server.WriteTimeout starts after the request headers are read,
		// so it can expire while a large upload body is still arriving and
		// prevent the handler from returning its small JSON response. Uploads
		// are already bounded by an idle read deadline and optional size limit.
		if r.URL.Path == "/api/v1/uploads" {
			_ = controller.SetWriteDeadline(time.Time{})
		}
		if r.Body == nil || r.Body == http.NoBody || controller.SetReadDeadline(time.Time{}) != nil {
			next.ServeHTTP(w, r)
			return
		}

		body := r.Body
		r.Body = &refreshingReadDeadlineBody{
			ReadCloser: body,
			refresh: func() error {
				return controller.SetReadDeadline(time.Now().Add(timeout))
			},
		}
		defer controller.SetReadDeadline(time.Time{})
		next.ServeHTTP(w, r)
	})
}

type refreshingReadDeadlineBody struct {
	io.ReadCloser
	refresh func() error
}

func (b *refreshingReadDeadlineBody) Read(p []byte) (int, error) {
	if err := b.refresh(); err != nil {
		return 0, err
	}
	return b.ReadCloser.Read(p)
}
