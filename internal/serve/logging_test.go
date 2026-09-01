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

func TestRequestLoggingMiddlewareDebugOmitsQueryString(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })

	handler := requestLoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files?query=private-tag", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	logs := output.String()
	if !strings.Contains(logs, "http request started") || !strings.Contains(logs, "path=/api/v1/files") {
		t.Fatalf("request debug log missing expected lifecycle/path fields: %s", logs)
	}
	if strings.Contains(logs, "private-tag") || strings.Contains(logs, "query=") {
		t.Fatalf("request debug log leaked query string: %s", logs)
	}
}
