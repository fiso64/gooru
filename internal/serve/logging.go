package serve

import (
	"log/slog"
	"net/http"
	"time"
)

// requestLoggingMiddleware emits request lifecycle logs without query strings,
// headers, request bodies, URL path values, file names, or other user-controlled
// content that could unnecessarily leak library metadata into logs.
//
// Reads stay at debug level to avoid noisy info logs during normal browsing.
// State-changing requests are info-level operational events, and server failures
// are warnings regardless of method. Route patterns are safe to log because they
// do not contain concrete file IDs, names, tags, or search expressions.
func requestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		slog.DebugContext(r.Context(), "http request started", "method", r.Method)

		logged := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(logged, r)

		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		level := requestCompletionLogLevel(r.Method, logged.status)
		slog.Log(r.Context(), level, "http request completed",
			"method", r.Method,
			"route", route,
			"status", logged.status,
			"duration", time.Since(start),
		)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// Unwrap lets http.ResponseController reach optional capabilities implemented by
// the underlying writer (flush, hijack, deadlines, and full-duplex support).
func (w *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func requestCompletionLogLevel(method string, status int) slog.Level {
	if status >= http.StatusInternalServerError {
		return slog.LevelWarn
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}
