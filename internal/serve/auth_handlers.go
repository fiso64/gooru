package serve

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type authMeResponse struct {
	User         userDTO         `json:"user"`
	Capabilities capabilitiesDTO `json:"capabilities"`
	CSRFToken    string          `json:"csrf_token,omitempty"`
}

type userDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type capabilitiesDTO struct {
	Upload bool `json:"upload"`
	Tag    bool `json:"tag"`
	Delete bool `json:"delete"`
	Admin  bool `json:"admin"`
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !s.cfg.Auth.Enabled {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		return
	}
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "auth_unavailable", "authentication store is not configured", nil)
		return
	}
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request body", nil)
		return
	}
	auth, err := s.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		status, message := authHTTPStatus(err)
		if status == http.StatusUnauthorized {
			slog.InfoContext(r.Context(), "authentication rejected")
		}
		writeError(w, status, "unauthorized", message, nil)
		return
	}
	csrfToken, err := s.auth.StableCSRF(r.Context(), auth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to issue CSRF token", nil)
		return
	}
	slog.InfoContext(r.Context(), "authentication succeeded")
	s.setSessionCookie(w, r, auth.Token, auth.Session.ExpiresAt)
	writeJSON(w, http.StatusOK, s.authResponse(auth, csrfToken))
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Auth.Enabled {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	auth, _ := currentAuth(r.Context())
	if s.auth != nil {
		if err := s.auth.RevokeSession(r.Context(), auth.Session.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to revoke session", nil)
			return
		}
	}
	slog.InfoContext(r.Context(), "session revoked")
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Auth.Enabled {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	auth, _ := currentAuth(r.Context())
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "auth_unavailable", "authentication store is not configured", nil)
		return
	}
	csrfToken, err := s.auth.StableCSRF(r.Context(), auth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to issue CSRF token", nil)
		return
	}
	writeJSON(w, http.StatusOK, s.authResponse(auth, csrfToken))
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Auth.Enabled {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	auth, _ := currentAuth(r.Context())
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "auth_unavailable", "authentication store is not configured", nil)
		return
	}
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request body", nil)
		return
	}
	if err := s.auth.ChangePasswordAndRevokeOtherSessions(r.Context(), auth.User.ID, auth.Session.ID, req.CurrentPassword, req.NewPassword); err != nil {
		status, message := authHTTPStatus(err)
		if status == http.StatusUnauthorized {
			slog.InfoContext(r.Context(), "password change rejected")
		}
		if status == http.StatusInternalServerError {
			writeError(w, status, "internal_error", message, nil)
		} else {
			writeError(w, status, "unauthorized", message, nil)
		}
		return
	}
	slog.InfoContext(r.Context(), "password changed")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) authResponse(auth AuthSession, csrfToken string) authMeResponse {
	admin := auth.User.Role == adminRole
	return authMeResponse{
		User: userDTO{ID: auth.User.ID, Username: auth.User.Username, Role: auth.User.Role},
		Capabilities: capabilitiesDTO{
			Upload: admin,
			Tag:    admin,
			Delete: admin,
			Admin:  admin,
		},
		CSRFToken: csrfToken,
	}
}
