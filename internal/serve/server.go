package serve

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	cfg  Config
	jobs *JobManager
}

func NewServer(cfg Config) *Server {
	return &Server{
		cfg:  cfg,
		jobs: NewJobManager(64),
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
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.Handle("GET /api/v1/jobs/{id}", authMiddleware(s.cfg.Auth.Token, http.HandlerFunc(s.handleGetJob)))
	mux.Handle("DELETE /api/v1/jobs/{id}", authMiddleware(s.cfg.Auth.Token, http.HandlerFunc(s.handleCancelJob)))
	mux.HandleFunc("/", s.handleNotFound)

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

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.jobs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
