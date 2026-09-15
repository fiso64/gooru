package serve

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
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

func TestUploadQueueAddedAtMakesLastQueuedNewest(t *testing.T) {
	base := time.Date(2024, 1, 2, 3, 4, 5, 400_000_000, time.UTC)
	got := make([]struct {
		index   int
		addedAt time.Time
	}, 3)
	for index := range got {
		got[index] = struct {
			index   int
			addedAt time.Time
		}{index: index, addedAt: resolveUploadAddedAt("queue", time.Time{}, base, time.Time{}, time.Time{}, index, len(got))}
	}

	sort.Slice(got, func(i, j int) bool { return got[i].addedAt.After(got[j].addedAt) })
	for position, item := range got {
		want := len(got) - 1 - position
		if item.index != want {
			t.Fatalf("newest-first position %d has queue index %d, want %d: %+v", position, item.index, want, got)
		}
	}
}

func TestUploadReverseQueueAddedAtMakesFirstQueuedNewest(t *testing.T) {
	base := time.Date(2024, 1, 2, 3, 4, 5, 400_000_000, time.UTC)
	got := make([]struct {
		index   int
		addedAt time.Time
	}, 3)
	for index := range got {
		got[index] = struct {
			index   int
			addedAt time.Time
		}{index: index, addedAt: resolveUploadAddedAt("reverse_queue", time.Time{}, base, time.Time{}, time.Time{}, index, len(got))}
	}

	sort.Slice(got, func(i, j int) bool { return got[i].addedAt.After(got[j].addedAt) })
	for position, item := range got {
		if item.index != position {
			t.Fatalf("newest-first position %d has queue index %d: %+v", position, item.index, got)
		}
	}
}

func TestUploadQueueAddedAtUsesAdmissionOrderAcrossRapidSelections(t *testing.T) {
	first := time.Date(2024, 1, 2, 3, 4, 5, 100_000_000, time.UTC)
	queueTimes := []time.Time{
		first,
		first.Add(5 * time.Second),
		first.Add(10 * time.Second),
	}
	added := make([]time.Time, len(queueTimes))
	for index, queueTime := range queueTimes {
		added[index] = resolveUploadAddedAt("queue", time.Time{}, queueTime, queueTimes[0], queueTimes[len(queueTimes)-1], index, len(queueTimes))
	}
	for index := 1; index < len(added); index++ {
		if !added[index].After(added[index-1]) {
			t.Fatalf("later queue item is not newer at %d: added=%v", index, added)
		}
	}
}

func TestUploadQueueUsesSharedBoundsAcrossDistinctWorkerTimes(t *testing.T) {
	first := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	last := first.Add(10 * time.Second)
	results := make([]time.Time, 2)
	for index, queueTime := range []time.Time{first, last} {
		dir := t.TempDir()
		library := &recordingUploadLibrary{}
		server := newUploadTestServer(t, dir, true, library)
		rec := httptest.NewRecorder()
		req := uploadAddedAtRequestWithBounds(t, queueTime, time.Time{}, first, last, index, 2, "queue")
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("worker %d status=%d body=%s", index, rec.Code, rec.Body.String())
		}
		if len(library.files) != 1 {
			t.Fatalf("worker %d imported %d files", index, len(library.files))
		}
		results[index] = library.files[0].AddedAt
	}
	if !results[1].After(results[0]) {
		t.Fatalf("later queue item should be newer across distinct worker times: first=%v last=%v", results[0], results[1])
	}
}

func TestUploadQueueKeepsNewerBatchAheadOfOlderBatch(t *testing.T) {
	dir := t.TempDir()
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, dir, true, library)
	base := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	type queuedItem struct {
		name       string
		queueTime  time.Time
		batchFirst time.Time
		batchLast  time.Time
		queueIndex int
		queueTotal int
	}
	items := []queuedItem{
		{name: "a1", queueTime: base, batchFirst: base, batchLast: base.Add(5 * time.Second), queueIndex: 0, queueTotal: 2},
		{name: "a2", queueTime: base.Add(5 * time.Second), batchFirst: base, batchLast: base.Add(5 * time.Second), queueIndex: 1, queueTotal: 2},
		{name: "b1", queueTime: base.Add(20 * time.Second), batchFirst: base.Add(20 * time.Second), batchLast: base.Add(25 * time.Second), queueIndex: 0, queueTotal: 2},
		{name: "b2", queueTime: base.Add(25 * time.Second), batchFirst: base.Add(20 * time.Second), batchLast: base.Add(25 * time.Second), queueIndex: 1, queueTotal: 2},
	}

	ordered := make([]struct {
		name    string
		addedAt time.Time
	}, 0, len(items))
	for _, item := range items {
		rec := httptest.NewRecorder()
		req := uploadAddedAtRequestWithBounds(t, item.queueTime, time.Time{}, item.batchFirst, item.batchLast, item.queueIndex, item.queueTotal, "queue")
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", item.name, rec.Code, rec.Body.String())
		}
		if len(library.files) != 1 {
			t.Fatalf("%s imported file count=%d want=1", item.name, len(library.files))
		}
		ordered = append(ordered, struct {
			name    string
			addedAt time.Time
		}{name: item.name, addedAt: library.files[0].AddedAt})
	}

	sort.Slice(ordered, func(i, j int) bool { return ordered[i].addedAt.After(ordered[j].addedAt) })
	want := []string{"b2", "b1", "a2", "a1"}
	for index, name := range want {
		if ordered[index].name != name {
			t.Fatalf("newest-first order[%d]=%s want=%s; got=%+v", index, ordered[index].name, name, ordered)
		}
	}
}

func TestUploadReverseQueueUsesSharedBoundsAcrossDistinctWorkerTimes(t *testing.T) {
	first := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	last := first.Add(10 * time.Second)
	results := make([]time.Time, 2)
	for index, queueTime := range []time.Time{first, last} {
		dir := t.TempDir()
		library := &recordingUploadLibrary{}
		server := newUploadTestServer(t, dir, true, library)
		rec := httptest.NewRecorder()
		req := uploadAddedAtRequestWithBounds(t, queueTime, time.Time{}, first, last, index, 2, "reverse_queue")
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("worker %d status=%d body=%s", index, rec.Code, rec.Body.String())
		}
		if len(library.files) != 1 {
			t.Fatalf("worker %d imported %d files", index, len(library.files))
		}
		results[index] = library.files[0].AddedAt
	}
	if !results[0].After(results[1]) {
		t.Fatalf("reverse queue did not invert distinct queue times: first=%v last=%v", results[0], results[1])
	}
}

func TestUploadAddedAtStrategyModtimeFallsBackToQueue(t *testing.T) {
	base := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	got := resolveUploadAddedAt("modtime", time.Time{}, base, time.Time{}, time.Time{}, 1, 3)
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
	return uploadAddedAtRequestWithBounds(t, queue, source, time.Time{}, time.Time{}, index, total, strategy)
}

func uploadAddedAtRequestWithBounds(t *testing.T, queue, source, first, last time.Time, index, total int, strategy string) *http.Request {
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
	if !first.IsZero() {
		_ = w.WriteField("queue_first_time_ms", strconv.FormatInt(first.UnixMilli(), 10))
	}
	if !last.IsZero() {
		_ = w.WriteField("queue_last_time_ms", strconv.FormatInt(last.UnixMilli(), 10))
	}
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
