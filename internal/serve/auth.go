package serve

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
)

const authCookieName = "gooru_auth"

func authMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := requestAuthToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token", nil)
			return
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid bearer token", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestAuthToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header != "" {
		scheme, credentials, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(credentials) == "" {
			return "", false
		}
		return strings.TrimSpace(credentials), true
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/files/") {
		cookie, err := r.Cookie(authCookieName)
		if err == nil && strings.TrimSpace(cookie.Value) != "" {
			value, decodeErr := url.QueryUnescape(cookie.Value)
			if decodeErr != nil {
				value = cookie.Value
			}
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func requestSizeMiddleware(limit int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(origins []string, next http.Handler) http.Handler {
	if len(origins) == 0 {
		return next
	}
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if _, ok := allowed[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Prefer")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
