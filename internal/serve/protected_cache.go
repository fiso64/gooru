package serve

import (
	"net/http"
	"strings"
)

const protectedAPICacheControl = "private, no-store"

func protectedAPICacheMiddleware(enabled bool, next http.Handler) http.Handler {
	if !enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", protectedAPICacheControl)
		}
		next.ServeHTTP(w, r)
	})
}
