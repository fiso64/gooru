package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestDurableUploadOversizedProducerFailureIsErrorNotCancellation(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	cfg := DefaultConfig(dbPath)
	cfg.Auth.Enabled = false
	cfg.Uploads.Enabled = true
	cfg.Uploads.MaxFileSizeBytes = 4
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: filepath.Join(root, "uploads")}}
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))

	upload := uploadRequest(t, map[string]string{"oversized.jpg": "hello"}, nil)
	uploadRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(uploadRec, upload)
	assertAPIError(t, uploadRec, http.StatusRequestEntityTooLarge, "payload_too_large")

	jobsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(jobsRec, httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=20", nil))
	if jobsRec.Code != http.StatusOK {
		t.Fatalf("jobs status = %d: %s", jobsRec.Code, jobsRec.Body.String())
	}
	var payload struct {
		Items []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Outcome string `json:"outcome"`
			Error   string `json:"error_message"`
		} `json:"items"`
	}
	if err := json.NewDecoder(jobsRec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode jobs response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("jobs = %+v, want one operation", payload.Items)
	}
	job := payload.Items[0]
	if job.Status != string(core.BackgroundWorkFailed) || job.Outcome != "error" {
		t.Fatalf("oversized job = %+v, want failed/error", job)
	}
	if job.Error == "" {
		t.Fatalf("oversized job has no durable error detail: %+v", job)
	}
}
