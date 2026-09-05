package serve

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"gooru.local/internal/securekey"
)

const (
	opaqueURLStateTTL        = 30 * 24 * time.Hour
	opaqueURLStatePurpose    = "gooru/protected-url-state/v1"
	maxOpaqueURLStateToken   = 8192
	maxOpaqueURLStateRequest = 16 << 10
)

var (
	errInvalidURLStateToken = errors.New("invalid URL state token")
	errExpiredURLStateToken = errors.New("expired URL state token")
	errURLStateTooLarge     = errors.New("URL state is too large")
)

type browserURLState struct {
	Query  string `json:"query,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Sort   string `json:"sort,omitempty"`
	Order  string `json:"order,omitempty"`
	FileID string `json:"file_id,omitempty"`
	Page   int    `json:"page,omitempty"`
}

type sealedBrowserURLState struct {
	State     browserURLState `json:"state"`
	UserID    string          `json:"user_id,omitempty"`
	ExpiresAt int64           `json:"expires_at"`
}

type urlStateCodec struct {
	aead cipher.AEAD
	now  func() time.Time
}

func newURLStateCodec(cfg Config) *urlStateCodec {
	if !cfg.Encryption.Enabled || !cfg.Encryption.OpaqueURLState || len(cfg.Encryption.Key) == 0 {
		return nil
	}
	key, err := securekey.Derive(cfg.Encryption.Key, opaqueURLStatePurpose)
	if err != nil {
		return nil
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil
	}
	return &urlStateCodec{aead: aead, now: func() time.Time { return time.Now().UTC() }}
}

func (c *urlStateCodec) seal(state browserURLState, userID string) (string, error) {
	if c == nil {
		return "", errInvalidURLStateToken
	}
	payload, err := json.Marshal(sealedBrowserURLState{State: state, UserID: userID, ExpiresAt: c.now().Add(opaqueURLStateTTL).Unix()})
	if err != nil {
		return "", err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := c.aead.Seal(nil, nonce, payload, []byte(opaqueURLStatePurpose))
	token := base64.RawURLEncoding.EncodeToString(append(nonce, ciphertext...))
	if len(token) > maxOpaqueURLStateToken {
		return "", errURLStateTooLarge
	}
	return token, nil
}

func (c *urlStateCodec) open(token, userID string) (browserURLState, error) {
	if c == nil {
		return browserURLState{}, errInvalidURLStateToken
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxOpaqueURLStateToken {
		return browserURLState{}, errInvalidURLStateToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= c.aead.NonceSize() {
		return browserURLState{}, errInvalidURLStateToken
	}
	plaintext, err := c.aead.Open(nil, raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():], []byte(opaqueURLStatePurpose))
	if err != nil {
		return browserURLState{}, errInvalidURLStateToken
	}
	var payload sealedBrowserURLState
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return browserURLState{}, errInvalidURLStateToken
	}
	if payload.ExpiresAt <= c.now().Unix() {
		return browserURLState{}, errExpiredURLStateToken
	}
	if payload.UserID != userID {
		return browserURLState{}, errInvalidURLStateToken
	}
	return payload.State, nil
}

func requestURLStateUserID(r *http.Request) string {
	if auth, ok := currentAuth(r.Context()); ok {
		return auth.User.ID
	}
	return ""
}

func (s *Server) handleUIState(w http.ResponseWriter, r *http.Request) {
	if s.urlState == nil {
		writeError(w, http.StatusNotFound, "not_found", "protected URL state is not enabled", nil)
		return
	}
	switch r.Method {
	case http.MethodPost:
		if r.URL.Path != "/api/v1/ui-state" {
			writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxOpaqueURLStateRequest)
		var state browserURLState
		if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid URL state", nil)
			return
		}
		token, err := s.urlState.seal(state, requestURLStateUserID(r))
		if errors.Is(err, errURLStateTooLarge) {
			writeError(w, http.StatusBadRequest, "url_state_too_large", "URL state is too large to protect", nil)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "url_state_failed", "failed to protect URL state", nil)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"token": token})
	case http.MethodGet:
		token := strings.TrimPrefix(r.URL.Path, "/api/v1/ui-state/")
		if token == "" || strings.Contains(token, "/") {
			writeError(w, http.StatusNotFound, "not_found", "URL state not found", nil)
			return
		}
		state, err := s.urlState.open(token, requestURLStateUserID(r))
		if errors.Is(err, errExpiredURLStateToken) {
			writeError(w, http.StatusGone, "url_state_expired", "protected URL state has expired", nil)
			return
		}
		if err != nil {
			writeError(w, http.StatusNotFound, "url_state_invalid", "protected URL state is invalid", nil)
			return
		}
		writeJSON(w, http.StatusOK, state)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}
