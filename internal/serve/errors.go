package serve

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string, message string, details interface{}) {
	writeJSON(w, status, ErrorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
