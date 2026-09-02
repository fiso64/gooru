package serve

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggingConfigSlogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  slog.Level
	}{
		{name: "debug", level: "debug", want: slog.LevelDebug},
		{name: "info", level: "info", want: slog.LevelInfo},
		{name: "warn", level: "warn", want: slog.LevelWarn},
		{name: "error", level: "error", want: slog.LevelError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (LoggingConfig{Level: tt.level}).SlogLevel(); got != tt.want {
				t.Fatalf("SlogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigRejectsUnknownLoggingLevel(t *testing.T) {
	cfg := DefaultConfig(t.TempDir() + "/gooru.db")
	cfg.Logging.Level = "verbose"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "logging.level must be one of") {
		t.Fatalf("Validate() error = %v, want logging level validation error", err)
	}
}

func TestRequestLoggingMiddlewareDebugOmitsUserControlledURL(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/files/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := requestLoggingMiddleware(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/private-filename?query=private-tag", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	logs := output.String()
	if !strings.Contains(logs, "http request started") || !strings.Contains(logs, "GET /api/v1/files/{id}") {
		t.Fatalf("request debug log missing expected lifecycle/route fields: %s", logs)
	}
	if !strings.Contains(logs, "status=204") {
		t.Fatalf("request debug log missing response status: %s", logs)
	}
	if strings.Contains(logs, "private-filename") || strings.Contains(logs, "private-tag") || strings.Contains(logs, "query=") {
		t.Fatalf("request debug log leaked user-controlled URL content: %s", logs)
	}
}

func TestRequestLoggingMiddlewareCompletionLevels(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		status    int
		wantLevel string
	}{
		{name: "read success stays debug", method: http.MethodGet, status: http.StatusOK, wantLevel: "DEBUG"},
		{name: "mutation success is info", method: http.MethodPost, status: http.StatusCreated, wantLevel: "INFO"},
		{name: "mutation client error is info", method: http.MethodDelete, status: http.StatusConflict, wantLevel: "INFO"},
		{name: "read server error is warning", method: http.MethodGet, status: http.StatusInternalServerError, wantLevel: "WARN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
			previous := slog.Default()
			slog.SetDefault(logger)
			defer slog.SetDefault(previous)

			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/example", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			})
			req := httptest.NewRequest(tt.method, "/api/v1/example", nil)
			requestLoggingMiddleware(mux).ServeHTTP(httptest.NewRecorder(), req)

			logs := output.String()
			if !strings.Contains(logs, "level="+tt.wantLevel+" msg=\"http request completed\"") {
				t.Fatalf("completion log level mismatch: %s", logs)
			}
			if !strings.Contains(logs, "status="+http.StatusText(tt.status)) && !strings.Contains(logs, "status="+statusString(tt.status)) {
				t.Fatalf("completion log missing status %d: %s", tt.status, logs)
			}
		})
	}
}

func TestLoggingResponseWriterPreservesFirstStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &loggingResponseWriter{ResponseWriter: recorder}
	writer.WriteHeader(http.StatusCreated)
	writer.WriteHeader(http.StatusInternalServerError)

	if writer.status != http.StatusCreated {
		t.Fatalf("status = %d, want %d", writer.status, http.StatusCreated)
	}
}

func statusString(status int) string {
	return string(rune('0'+status/100)) + string(rune('0'+status/10%10)) + string(rune('0'+status%10))
}
