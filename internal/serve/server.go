package serve

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"gooru.local/internal/managedfile"
)

const metadataRequestBodyLimit int64 = 1 << 20

type Server struct {
	cfg                  Config
	library              Library
	media                *MediaService
	meta                 MediaMetadataProvider
	auth                 *AuthStore
	urlState             *urlStateCodec
	fileSelections       *fileSelectionStore
	fileDownloads        *fileDownloadStore
	managedFiles         *managedfile.Writer
	backgroundContent    contentHashLibrary
	backgroundOperations backgroundOperationReader
	maintenanceJobs      maintenanceJobRunner
}

func NewServer(cfg Config) *Server {
	return NewServerWithLibrary(cfg, nil)
}

func NewServerWithLibrary(cfg Config, library Library) *Server {
	metadata := NewMediaMetadataProvider(cfg)
	media := newComposedMediaServiceFromConfig(cfg)
	var backgroundContent contentHashLibrary
	var backgroundOperations backgroundOperationReader
	var maintenanceJobs maintenanceJobRunner
	if gooruLibrary, ok := library.(*GooruLibrary); ok {
		backgroundContent = gooruLibrary
		backgroundOperations = &durableFileRemovalOperationStore{GooruLibrary: gooruLibrary}
		maintenanceJobs = gooruLibrary
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
		library:              library,
		media:                media,
		meta:                 metadata,
		urlState:             newURLStateCodec(cfg),
		fileSelections:       newFileSelectionStore(),
		fileDownloads:        newFileDownloadStore(),
		managedFiles:         managedFiles,
		backgroundContent:    backgroundContent,
		backgroundOperations: backgroundOperations,
		maintenanceJobs:      maintenanceJobs,
	}
}

func (s *Server) SetAuthStore(store *AuthStore) {
	s.auth = store
}

func (s *Server) HTTPServer() *http.Server {
	requestCtx, cancelRequests := context.WithCancel(context.Background())
	server := &http.Server{
		Addr:              s.cfg.Server.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: s.cfg.Server.ReadTimeout,
		WriteTimeout:      s.cfg.Server.WriteTimeout,
		IdleTimeout:       s.cfg.Server.IdleTimeout,
		BaseContext: func(net.Listener) context.Context {
			return requestCtx
		},
	}
	server.RegisterOnShutdown(cancelRequests)
	return server
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/build", methodHandler(http.MethodGet, s.handleBuildInfo))
	mux.HandleFunc("/api/v1/health", methodHandler(http.MethodGet, s.handleHealth))
	mux.HandleFunc("/api/v1/ui-config", methodHandler(http.MethodGet, s.handleUIConfig))
	mux.Handle("/api/v1/auth/login", requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleAuthLogin)))
	mux.Handle("/api/v1/auth/logout", s.protected(http.HandlerFunc(s.handleAuthLogout)))
	mux.Handle("/api/v1/auth/me", authMiddleware(s.cfg, s.auth, http.HandlerFunc(s.handleAuthMe)))
	mux.Handle("/api/v1/auth/change-password", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleChangePassword))))
	mux.Handle("/api/v1/ui-state/", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleUIState))))
	mux.Handle("/api/v1/ui-state", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleUIState))))
	mux.Handle("/api/v1/upload-targets", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleUploadTargets)))
	mux.Handle("/api/v1/uploads", s.adminProtected(http.HandlerFunc(s.handleUploadEndpoint)))
	mux.Handle("/api/v1/file-selections/", s.adminProtected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFileSelection))))
	mux.Handle("/api/v1/file-selections", s.adminProtected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFileSelections))))
	mux.Handle("/api/v1/file-downloads/", s.adminProtected(http.HandlerFunc(s.handleFileDownload)))
	mux.Handle("/api/v1/file-downloads", s.adminProtected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFileDownloads))))
	mux.Handle("/api/v1/files/tags", s.adminProtected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleMutateTags))))
	mux.Handle("/api/v1/files/", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, s.fileRouteHandler())))
	mux.Handle("/api/v1/files", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFiles))))
	mux.Handle("/api/v1/files/around", authMiddleware(s.cfg, s.auth, requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFilesAround))))
	mux.Handle("/api/v1/files/search", authMiddleware(s.cfg, s.auth, requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFileSearch))))
	mux.Handle("/api/v1/comics/", s.protected(http.HandlerFunc(s.handleComic)))
	mux.Handle("/api/v1/search/suggestions", authMiddleware(s.cfg, s.auth, requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleSearchSuggestions))))
	mux.Handle("/api/v1/saved-searches/", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleSavedSearch))))
	mux.Handle("/api/v1/saved-searches", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleSavedSearches))))
	mux.Handle("/api/v1/tags/namespaces", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleTagNamespaces)))
	mux.Handle("/api/v1/tags", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleListTags)))
	mux.Handle("/api/v1/maintenance-jobs", s.adminProtected(http.HandlerFunc(s.handleMaintenanceJobs)))
	mux.Handle("/api/v1/maintenance-jobs/", s.adminProtected(http.HandlerFunc(s.handleMaintenanceJob)))
	mux.Handle("/api/v1/operations/cancel-all", s.adminProtected(http.HandlerFunc(s.handleCancelAllOperations)))
	mux.Handle("/api/v1/operations/events", s.adminProtected(http.HandlerFunc(s.handleOperationEvents)))
	mux.Handle("/api/v1/operations", s.adminProtected(http.HandlerFunc(s.handleOperations)))
	mux.Handle("/api/v1/operations/", s.adminProtected(http.HandlerFunc(s.handleOperation)))
	mux.HandleFunc("/", s.handleFrontend)

	var h http.Handler = mux
	h = securityHeadersMiddleware(h)
	h = protectedAPICacheMiddleware(s.cfg.Encryption.Enabled, h)
	h = requestSizeMiddleware(s.cfg.Server.MaxRequestBodyBytes, h)
	h = corsMiddleware(s.cfg.Server.CORSOrigins, h)
	h = requestLoggingMiddleware(h)
	h = requestWriteTimeoutMiddleware(s.cfg.Server.WriteTimeout, h)
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
		// The frontend build owns resource-loading CSP so SvelteKit can hash its
		// generated inline bootstrap at build time. Keep directives that require
		// an HTTP header (notably frame-ancestors) at the server boundary.
		header.Set("Content-Security-Policy", contentSecurityPolicy())
		next.ServeHTTP(w, r)
	})
}

func contentSecurityPolicy() string {
	return "frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
}

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
