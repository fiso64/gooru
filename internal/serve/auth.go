package serve

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type authContextKey struct{}

func authMiddleware(cfg Config, store *AuthStore, next http.Handler) http.Handler {
	if !cfg.Auth.Enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, "auth_unavailable", "authentication store is not configured", nil)
			return
		}
		token, ok := requestSessionToken(r, cfg.Auth.CookieName)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "login required", nil)
			return
		}
		auth, err := store.LookupSession(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "login required", nil)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, auth)))
	})
}

func csrfMiddleware(cfg Config, store *AuthStore, next http.Handler) http.Handler {
	if !cfg.Auth.Enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutatingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		auth, ok := currentAuth(r.Context())
		if !ok || store == nil || !store.VerifyCSRFToken(auth, r.Header.Get("X-Gooru-CSRF")) {
			writeError(w, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func adminMiddleware(cfg Config, next http.Handler) http.Handler {
	if !cfg.Auth.Enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ok := currentAuth(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "login required", nil)
			return
		}
		if auth.User.Role != adminRole {
			writeError(w, http.StatusForbidden, "forbidden", "admin privileges required", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentAuth(ctx context.Context) (AuthSession, bool) {
	auth, ok := ctx.Value(authContextKey{}).(AuthSession)
	return auth, ok
}

func requestSessionToken(r *http.Request, cookieName string) (string, bool) {
	cookie, err := r.Cookie(cookieName)
	if err == nil && strings.TrimSpace(cookie.Value) != "" {
		value, decodeErr := url.QueryUnescape(cookie.Value)
		if decodeErr != nil {
			value = cookie.Value
		}
		return strings.TrimSpace(value), true
	}
	return "", false
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.Auth.CookieName,
		Value:    url.QueryEscape(token),
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		SameSite: sameSiteMode(s.cfg.Auth.CookieSameSite),
		Secure:   s.cookieSecure(r),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.Auth.CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: sameSiteMode(s.cfg.Auth.CookieSameSite),
		Secure:   s.cookieSecure(r),
	})
}

func (s *Server) cookieSecure(r *http.Request) bool {
	switch strings.ToLower(s.cfg.Auth.CookieSecure) {
	case "true":
		return true
	case "false":
		return false
	default:
		return r.TLS != nil || strings.HasPrefix(s.cfg.Server.PublicURL, "https://")
	}
}

func sameSiteMode(value string) http.SameSite {
	switch strings.ToLower(value) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func authHTTPStatus(err error) (int, string) {
	switch {
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrDisabledUser):
		return http.StatusUnauthorized, "invalid username or password"
	default:
		return http.StatusInternalServerError, "authentication failed"
	}
}

func requestSizeMiddleware(limit int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Uploads have their own per-file limit and stream large bodies to disk.
		// Applying the generic API-body cap here makes large media impossible
		// even when uploads.max_file_size_bytes is intentionally left unset.
		if r.URL.Path != "/api/v1/uploads" {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Prefer, X-Gooru-CSRF")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
