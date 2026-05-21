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
}

func NewServer(cfg Config) *Server {
	return NewServerWithLibrary(cfg, nil)
}

func NewServerWithLibrary(cfg Config, library Library) *Server {
	return &Server{
		cfg:     cfg,
		jobs:    NewJobManager(64, cfg.Jobs.CompletedTTL),
		library: library,
		media:   NewMediaService(cfg),
	}
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
	mux.Handle("/api/v1/uploads", authMiddleware(s.cfg.Auth.Token, http.HandlerFunc(s.handleUpload)))
	mux.Handle("/api/v1/files/tags", authMiddleware(s.cfg.Auth.Token, http.HandlerFunc(s.handleMutateTags)))
	mux.Handle("/api/v1/files/", authMiddleware(s.cfg.Auth.Token, http.HandlerFunc(s.handleFile)))
	mux.Handle("/api/v1/files", authMiddleware(s.cfg.Auth.Token, methodHandler(http.MethodGet, s.handleListFiles)))
	mux.Handle("/api/v1/tags", authMiddleware(s.cfg.Auth.Token, methodHandler(http.MethodGet, s.handleListTags)))
	mux.Handle("/api/v1/jobs/", authMiddleware(s.cfg.Auth.Token, http.HandlerFunc(s.handleJob)))
	mux.HandleFunc("/", s.handleFrontend)

	var h http.Handler = mux
	h = requestSizeMiddleware(s.cfg.Server.MaxRequestBodyBytes, h)
	h = corsMiddleware(s.cfg.Server.CORSOrigins, h)
	return h
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
