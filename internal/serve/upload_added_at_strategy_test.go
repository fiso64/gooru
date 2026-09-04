package serve

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestUploadAddedAtStrategyUsesTargetDefaultAndRequestOverride(t *testing.T) {
	base := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	source := time.Date(2020, 7, 8, 9, 10, 11, 0, time.UTC)
	for _, tc := range []struct {
		name, targetStrategy, requestStrategy string
		want                                  time.Time
	}{
		{"target reverse", "reverse_queue", "", base.Add(2 * time.Second)},
		{"request modtime", "reverse_queue", "modtime", source},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			library := &recordingUploadLibrary{}
			server := newUploadTestServer(t, dir, true, library)
			server.cfg.Uploads.Targets[0].AddedAtStrategy = tc.targetStrategy
			rec := httptest.NewRecorder()
			req := uploadAddedAtRequest(t, base, source, 0, 3, tc.requestStrategy)
			server.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if len(library.files) != 1 || !library.files[0].AddedAt.Equal(tc.want) {
				t.Fatalf("added_at=%v want=%v files=%+v", library.files[0].AddedAt, tc.want, library.files)
			}
		})
	}
}

func TestUploadAddedAtStrategyModtimeFallsBackToQueue(t *testing.T) {
	base := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	got := resolveUploadAddedAt("modtime", time.Time{}, base, 1, 3)
	if want := base.Add(time.Second); !got.Equal(want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestUploadAddedAtStrategyRejectsInvalidOverride(t *testing.T) {
	dir := t.TempDir()
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, dir, true, library)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadAddedAtRequest(t, time.Now(), time.Time{}, 0, 1, "random"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func uploadAddedAtRequest(t *testing.T, queue, source time.Time, index, total int, strategy string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("files", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("hello"))
	if !source.IsZero() {
		_ = w.WriteField("source_modtime_ms", strconv.FormatInt(source.UnixMilli(), 10))
	}
	_ = w.WriteField("queue_time_ms", strconv.FormatInt(queue.UnixMilli(), 10))
	_ = w.WriteField("queue_index", strconv.Itoa(index))
	_ = w.WriteField("queue_total", strconv.Itoa(total))
	if strategy != "" {
		_ = w.WriteField("added_at_strategy", strategy)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}
