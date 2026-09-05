package serve

import (
	"net/http"
	"strings"
)

const protectedAPICacheControl = "private, no-store"

func applyProtectedAPICacheHeaders(header http.Header) {
	header.Set("Cache-Control", protectedAPICacheControl)
	header.Set("Pragma", "no-cache")
	header.Set("Expires", "0")
}

type protectedAPICacheResponseWriter struct {
	http.ResponseWriter
}

// Unwrap keeps ResponseController and other standard-library response helpers
// able to reach the underlying writer while cache policy remains centralized.
func (w *protectedAPICacheResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *protectedAPICacheResponseWriter) WriteHeader(statusCode int) {
	applyProtectedAPICacheHeaders(w.Header())
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *protectedAPICacheResponseWriter) Write(body []byte) (int, error) {
	applyProtectedAPICacheHeaders(w.Header())
	return w.ResponseWriter.Write(body)
}

func protectedAPICacheMiddleware(enabled bool, next http.Handler) http.Handler {
	if !enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Set the default immediately, then enforce it again when headers are
		// committed. The second application prevents a future feature handler
		// from accidentally replacing protected no-store with a cacheable value.
		applyProtectedAPICacheHeaders(w.Header())
		next.ServeHTTP(&protectedAPICacheResponseWriter{ResponseWriter: w}, r)
	})
}
