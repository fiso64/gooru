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
	base := time.Date(2024, 1, 2, 3, 4, 5, 123_000_000, time.UTC)
	source := time.Date(2020, 7, 8, 9, 10, 11, 456_000_000, time.UTC)
	for _, tc := range []struct {
		name, targetStrategy, requestStrategy string
		want                                  time.Time
	}{
		{"target reverse", "reverse_queue", "", base},
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

type uploadAddedSortItem struct {
	index      int
	addedAt    time.Time
	addedOrder int64
}

func newestUploadItems(items []uploadAddedSortItem) {
	sort.Slice(items, func(i, j int) bool {
		if !items[i].addedAt.Equal(items[j].addedAt) {
			return items[i].addedAt.After(items[j].addedAt)
		}
		if items[i].addedOrder != items[j].addedOrder {
			return items[i].addedOrder > items[j].addedOrder
		}
		return items[i].index < items[j].index
	})
}

func TestUploadQueueOrderMakesLastQueuedNewestWithoutChangingTime(t *testing.T) {
	base := time.Date(2024, 1, 2, 3, 4, 5, 400_000_000, time.UTC)
	got := make([]uploadAddedSortItem, 3)
	for index := range got {
		got[index] = uploadAddedSortItem{
			index:      index,
			addedAt:    resolveUploadAddedAt("queue", time.Time{}, base, base, base, index, len(got)),
			addedOrder: resolveUploadAddedOrder("queue", index, len(got)),
		}
		if !got[index].addedAt.Equal(base) {
			t.Fatalf("queue index %d changed honest timestamp: got=%v want=%v", index, got[index].addedAt, base)
		}
	}

	newestUploadItems(got)
	for position, item := range got {
		want := len(got) - 1 - position
		if item.index != want {
			t.Fatalf("newest-first position %d has queue index %d, want %d: %+v", position, item.index, want, got)
		}
	}
}

func TestUploadReverseQueueOrderMakesFirstQueuedNewestWithoutChangingTime(t *testing.T) {
	base := time.Date(2024, 1, 2, 3, 4, 5, 400_000_000, time.UTC)
	got := make([]uploadAddedSortItem, 3)
	for index := range got {
		got[index] = uploadAddedSortItem{
			index:      index,
			addedAt:    resolveUploadAddedAt("reverse_queue", time.Time{}, base, base, base, index, len(got)),
			addedOrder: resolveUploadAddedOrder("reverse_queue", index, len(got)),
		}
		if !got[index].addedAt.Equal(base) {
			t.Fatalf("queue index %d changed honest timestamp: got=%v want=%v", index, got[index].addedAt, base)
		}
	}

	newestUploadItems(got)
	for position, item := range got {
		if item.index != position {
			t.Fatalf("newest-first position %d has queue index %d: %+v", position, item.index, got)
		}
	}
}

func TestUploadQueueUsesOneBatchTimestampAcrossRapidSelections(t *testing.T) {
	first := time.Date(2024, 1, 2, 3, 4, 5, 100_000_000, time.UTC)
	queueTimes := []time.Time{
		first,
		first.Add(5 * time.Second),
		first.Add(10 * time.Second),
	}
	for index, queueTime := range queueTimes {
		added := resolveUploadAddedAt("queue", time.Time{}, queueTime, queueTimes[0], queueTimes[len(queueTimes)-1], index, len(queueTimes))
		if !added.Equal(first) {
			t.Fatalf("queue index %d added_at=%v want shared batch time %v", index, added, first)
		}
		if order := resolveUploadAddedOrder("queue", index, len(queueTimes)); order != int64(index) {
			t.Fatalf("queue index %d added_order=%d", index, order)
		}
	}
}

func TestUploadQueueUsesSharedBatchTimestampAcrossDistinctWorkers(t *testing.T) {
	first := time.Date(2024, 1, 2, 3, 4, 5, 123_000_000, time.UTC)
	last := first.Add(10 * time.Second)
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
		if got := library.files[0].AddedAt; !got.Equal(first) {
			t.Fatalf("worker %d added_at=%v want=%v", index, got, first)
		}
	}
}

func TestUploadQueueKeepsNewerBatchAheadOfOlderBatch(t *testing.T) {
	base := time.Date(2024, 1, 2, 3, 4, 5, 100_000_000, time.UTC)
	olderFirst := base
	newerFirst := base.Add(20 * time.Second)
	older := resolveUploadAddedAt("queue", time.Time{}, base.Add(5*time.Second), olderFirst, base.Add(5*time.Second), 1, 2)
	newer := resolveUploadAddedAt("queue", time.Time{}, base.Add(25*time.Second), newerFirst, base.Add(25*time.Second), 1, 2)
	if !newer.After(older) {
		t.Fatalf("newer batch added_at=%v must be after older batch added_at=%v", newer, older)
	}
}

func TestUploadReverseQueueUsesSharedBatchTimestampAcrossDistinctWorkers(t *testing.T) {
	first := time.Date(2024, 1, 2, 3, 4, 5, 321_000_000, time.UTC)
	last := first.Add(10 * time.Second)
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
		if got := library.files[0].AddedAt; !got.Equal(first) {
			t.Fatalf("worker %d added_at=%v want=%v", index, got, first)
		}
	}
	if firstOrder, lastOrder := resolveUploadAddedOrder("reverse_queue", 0, 2), resolveUploadAddedOrder("reverse_queue", 1, 2); firstOrder <= lastOrder {
		t.Fatalf("reverse queue order not inverted: first=%d last=%d", firstOrder, lastOrder)
	}
}

func TestUploadAddedAtStrategyModtimeFallsBackToBatchTime(t *testing.T) {
	base := time.Date(2024, 1, 2, 3, 4, 5, 234_000_000, time.UTC)
	got := resolveUploadAddedAt("modtime", time.Time{}, base.Add(time.Second), base, base.Add(time.Second), 1, 3)
	if !got.Equal(base) {
		t.Fatalf("got=%v want=%v", got, base)
	}
}

func TestUploadAddedOrderNormalizesInvalidOrdinals(t *testing.T) {
	if got := resolveUploadAddedOrder("queue", -1, 0); got != 0 {
		t.Fatalf("negative queue index normalized to %d, want 0", got)
	}
	if got := resolveUploadAddedOrder("reverse_queue", 10, 3); got != 0 {
		t.Fatalf("out-of-range reverse queue index normalized to %d, want 0", got)
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
