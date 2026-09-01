package serve

import (
	"log/slog"
	"net/http"
	"time"
)

// requestLoggingMiddleware emits request lifecycle logs without query strings,
// headers, request bodies, file names, or other user-controlled content that
// could unnecessarily leak library metadata into logs.
func requestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		slog.DebugContext(r.Context(), "http request started",
			"method", r.Method,
			"path", r.URL.Path,
		)
		next.ServeHTTP(w, r)
		slog.DebugContext(r.Context(), "http request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}
