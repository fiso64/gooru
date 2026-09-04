from pathlib import Path


def rep(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if new in text:
        return
    if old not in text:
        raise SystemExit(f"missing anchor in {path}: {old[:120]!r}")
    p.write_text(text.replace(old, new, 1))

# Server accepts a common first/last queue-time envelope so one-file async worker
# requests can reverse a queue whose items were staged at different times.
rep(
    'internal/serve/upload_stream.go',
    '''\tvar targetID, conflictRequested, addedAtStrategyRequested string\n\tvar targetSeen, conflictSeen, addedAtStrategySeen bool''',
    '''\tvar targetID, conflictRequested, addedAtStrategyRequested string\n\tvar queueFirstTimeValue, queueLastTimeValue string\n\tvar targetSeen, conflictSeen, addedAtStrategySeen, queueFirstTimeSeen, queueLastTimeSeen bool'''
)
rep(
    'internal/serve/upload_stream.go',
    '''\t\t\tcase "queue_time_ms":\n\t\t\t\tqueueTimeValues = append(queueTimeValues, value)\n\t\t\tcase "queue_index":''',
    '''\t\t\tcase "queue_time_ms":\n\t\t\t\tqueueTimeValues = append(queueTimeValues, value)\n\t\t\tcase "queue_first_time_ms":\n\t\t\t\tif !queueFirstTimeSeen {\n\t\t\t\t\tqueueFirstTimeValue, queueFirstTimeSeen = value, true\n\t\t\t\t}\n\t\t\tcase "queue_last_time_ms":\n\t\t\t\tif !queueLastTimeSeen {\n\t\t\t\t\tqueueLastTimeValue, queueLastTimeSeen = value, true\n\t\t\t\t}\n\t\t\tcase "queue_index":'''
)
rep(
    'internal/serve/upload_stream.go',
    '''\tqueueFallback := time.Now().UTC()\n\tfor i := range streamed {''',
    '''\tqueueFallback := time.Now().UTC()\n\tqueueFirstTime := parseUploadSourceModTime(queueFirstTimeValue)\n\tqueueLastTime := parseUploadSourceModTime(queueLastTimeValue)\n\tfor i := range streamed {'''
)
rep(
    'internal/serve/upload_stream.go',
    '''\t\tstreamed[i].addedAt = resolveUploadAddedAt(addedAtStrategy, streamed[i].sourceModTime, queueTime, queueIndex, queueTotal)''',
    '''\t\tstreamed[i].addedAt = resolveUploadAddedAt(addedAtStrategy, streamed[i].sourceModTime, queueTime, queueFirstTime, queueLastTime, queueIndex, queueTotal)'''
)
rep(
    'internal/serve/upload_stream.go',
    '''func resolveUploadAddedAt(strategy string, sourceModTime, queueTime time.Time, queueIndex, queueTotal int) time.Time {''',
    '''func resolveUploadAddedAt(strategy string, sourceModTime, queueTime, queueFirstTime, queueLastTime time.Time, queueIndex, queueTotal int) time.Time {'''
)
rep(
    'internal/serve/upload_stream.go',
    '''\tqueueOffset := queueIndex\n\tif strategy == "reverse_queue" {\n\t\tqueueOffset = queueTotal - 1 - queueIndex\n\t}\n\tif strategy == "modtime" && !sourceModTime.IsZero() {\n\t\treturn sourceModTime.UTC()\n\t}\n\t// locations.added_at is second-granularity; offset equal-time batch items by one\n\t// second so async worker completion order cannot affect stable queue ordering.\n\treturn queueTime.UTC().Truncate(time.Second).Add(time.Duration(queueOffset) * time.Second)''',
    '''\tqueueOffset := queueIndex\n\tif strategy == "modtime" && !sourceModTime.IsZero() {\n\t\treturn sourceModTime.UTC()\n\t}\n\tif strategy == "reverse_queue" {\n\t\tqueueOffset = queueTotal - 1 - queueIndex\n\t\tif !queueFirstTime.IsZero() && !queueLastTime.IsZero() && !queueLastTime.Before(queueFirstTime) && !queueTime.Before(queueFirstTime) && !queueTime.After(queueLastTime) {\n\t\t\t// Reflect each item's real queue-entry time across the batch envelope, then\n\t\t\t// use the reversed ordinal as the second-granularity tie breaker. This\n\t\t\t// keeps one-file async worker requests globally reversed even when files\n\t\t\t// were appended to the UI queue in separate selections.\n\t\t\tqueueTime = queueFirstTime.Add(queueLastTime.Sub(queueTime))\n\t\t}\n\t}\n\t// locations.added_at is second-granularity; offset equal-time batch items by one\n\t// second so async worker completion order cannot affect stable queue ordering.\n\treturn queueTime.UTC().Truncate(time.Second).Add(time.Duration(queueOffset) * time.Second)'''
)

# Existing function-level call uses empty bounds; add an HTTP-boundary regression for
# separate one-file worker requests with different queue-entry times.
rep(
    'internal/serve/upload_added_at_strategy_test.go',
    '''\tgot := resolveUploadAddedAt("modtime", time.Time{}, base, 1, 3)''',
    '''\tgot := resolveUploadAddedAt("modtime", time.Time{}, base, time.Time{}, time.Time{}, 1, 3)'''
)
insert_test = r'''
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

'''
p = Path('internal/serve/upload_added_at_strategy_test.go')
text = p.read_text()
anchor = 'func TestUploadAddedAtStrategyModtimeFallsBackToQueue(t *testing.T) {'
if insert_test not in text:
    if anchor not in text:
        raise SystemExit('test anchor missing')
    text = text.replace(anchor, insert_test + anchor, 1)
p.write_text(text)
rep(
    'internal/serve/upload_added_at_strategy_test.go',
    '''func uploadAddedAtRequest(t *testing.T, queue, source time.Time, index, total int, strategy string) *http.Request {\n\tt.Helper()''',
    '''func uploadAddedAtRequest(t *testing.T, queue, source time.Time, index, total int, strategy string) *http.Request {\n\tt.Helper()\n\treturn uploadAddedAtRequestWithBounds(t, queue, source, time.Time{}, time.Time{}, index, total, strategy)\n}\n\nfunc uploadAddedAtRequestWithBounds(t *testing.T, queue, source, first, last time.Time, index, total int, strategy string) *http.Request {\n\tt.Helper()'''
)
rep(
    'internal/serve/upload_added_at_strategy_test.go',
    '''\t_ = w.WriteField("queue_time_ms", strconv.FormatInt(queue.UnixMilli(), 10))\n\t_ = w.WriteField("queue_index", strconv.Itoa(index))''',
    '''\t_ = w.WriteField("queue_time_ms", strconv.FormatInt(queue.UnixMilli(), 10))\n\tif !first.IsZero() {\n\t\t_ = w.WriteField("queue_first_time_ms", strconv.FormatInt(first.UnixMilli(), 10))\n\t}\n\tif !last.IsZero() {\n\t\t_ = w.WriteField("queue_last_time_ms", strconv.FormatInt(last.UnixMilli(), 10))\n\t}\n\t_ = w.WriteField("queue_index", strconv.Itoa(index))'''
)

# Document the optional batch envelope fields.
rep(
    'docs/openapi.yaml',
    '''                queue_index:\n                  type: array''',
    '''                queue_first_time_ms:\n                  type: integer\n                  format: int64\n                  description: Earliest client-captured queue timestamp in this submission; used with `queue_last_time_ms` to reverse queues consistently across one-file async worker requests.\n                queue_last_time_ms:\n                  type: integer\n                  format: int64\n                  description: Latest client-captured queue timestamp in this submission; used with `queue_first_time_ms` to reverse queues consistently across one-file async worker requests.\n                queue_index:\n                  type: array'''
)

# Frontend workflow snapshots a shared envelope before workers and carries it through
# mutation/client layers.
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''\tconst batchQueueTimes = items.map((item) => item.queueTimeMs ?? fallbackQueueTimeMs);'''.replace('\t','  '),
    '''  const batchQueueTimes = items.map((item) => item.queueTimeMs ?? fallbackQueueTimeMs);\n    const batchQueueFirstTimeMs = Math.min(...batchQueueTimes);\n    const batchQueueLastTimeMs = Math.max(...batchQueueTimes);'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''            queueTimeMs: batchQueueTimes[index],\n            queueIndex: index,''',
    '''            queueTimeMs: batchQueueTimes[index],\n            queueFirstTimeMs: batchQueueFirstTimeMs,\n            queueLastTimeMs: batchQueueLastTimeMs,\n            queueIndex: index,'''
)
rep(
    'frontend/src/lib/queries/library.ts',
    '''  queueTimeMs: number;\n  queueIndex: number;''',
    '''  queueTimeMs: number;\n  queueFirstTimeMs: number;\n  queueLastTimeMs: number;\n  queueIndex: number;'''
)
rep(
    'frontend/src/lib/queries/library.ts',
    '''    mutationFn: ({ files, tags, preferAsync, targetID, conflictPolicy, addedAtStrategy, queueTimeMs, queueIndex, queueTotal, onProgress }) =>''',
    '''    mutationFn: ({ files, tags, preferAsync, targetID, conflictPolicy, addedAtStrategy, queueTimeMs, queueFirstTimeMs, queueLastTimeMs, queueIndex, queueTotal, onProgress }) =>'''
)
rep(
    'frontend/src/lib/queries/library.ts',
    '''        queueTimeMs: [queueTimeMs],\n        queueIndex: [queueIndex],''',
    '''        queueTimeMs: [queueTimeMs],\n        queueFirstTimeMs,\n        queueLastTimeMs,\n        queueIndex: [queueIndex],'''
)
rep(
    'frontend/src/lib/api/client.ts',
    '''    for (const value of ordering.queueTimeMs ?? []) if (Number.isFinite(value) && value > 0) form.append('queue_time_ms', String(Math.trunc(value)));\n    for (const value of ordering.queueIndex ?? [])''',
    '''    for (const value of ordering.queueTimeMs ?? []) if (Number.isFinite(value) && value > 0) form.append('queue_time_ms', String(Math.trunc(value)));\n    if (Number.isFinite(ordering.queueFirstTimeMs) && (ordering.queueFirstTimeMs ?? 0) > 0) form.append('queue_first_time_ms', String(Math.trunc(ordering.queueFirstTimeMs!)));\n    if (Number.isFinite(ordering.queueLastTimeMs) && (ordering.queueLastTimeMs ?? 0) > 0) form.append('queue_last_time_ms', String(Math.trunc(ordering.queueLastTimeMs!)));\n    for (const value of ordering.queueIndex ?? [])'''
)
rep(
    'frontend/src/lib/api/client.ts',
    '''  queueTimeMs?: number[];\n  queueIndex?: number[];''',
    '''  queueTimeMs?: number[];\n  queueFirstTimeMs?: number;\n  queueLastTimeMs?: number;\n  queueIndex?: number[];'''
)

# Update regressions with distinct queue-entry times and assert shared envelope transport.
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.test.ts',
    '''    now.mockReturnValueOnce(1_700_000_000_000).mockReturnValue(1_800_000_000_000);\n    const workflow = createUploadWorkflow();\n    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);\n    workflow.setTarget('archive', 'reverse_queue');''',
    '''    now.mockReturnValueOnce(1_700_000_000_000).mockReturnValueOnce(1_700_000_010_000).mockReturnValue(1_800_000_000_000);\n    const workflow = createUploadWorkflow();\n    workflow.select([uploadFile('first.jpg')]);\n    workflow.select([uploadFile('second.jpg')]);\n    workflow.setTarget('archive', 'reverse_queue');'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.test.ts',
    '''    const calls: Array<{ name: string; queueTimeMs: number; queueIndex: number; queueTotal: number; strategy: string }> = [];''',
    '''    const calls: Array<{ name: string; queueTimeMs: number; queueFirstTimeMs: number; queueLastTimeMs: number; queueIndex: number; queueTotal: number; strategy: string }> = [];'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.test.ts',
    '''        queueTimeMs: variables.queueTimeMs,\n        queueIndex: variables.queueIndex,''',
    '''        queueTimeMs: variables.queueTimeMs,\n        queueFirstTimeMs: variables.queueFirstTimeMs,\n        queueLastTimeMs: variables.queueLastTimeMs,\n        queueIndex: variables.queueIndex,'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.test.ts',
    '''      { name: 'first.jpg', queueTimeMs: 1_700_000_000_000, queueIndex: 0, queueTotal: 2, strategy: 'reverse_queue' },\n      { name: 'second.jpg', queueTimeMs: 1_700_000_000_000, queueIndex: 1, queueTotal: 2, strategy: 'reverse_queue' }\n    ]);\n    expect(workflow.items.map((item) => item.queueTimeMs)).toEqual([1_700_000_000_000, 1_700_000_000_000]);''',
    '''      { name: 'first.jpg', queueTimeMs: 1_700_000_000_000, queueFirstTimeMs: 1_700_000_000_000, queueLastTimeMs: 1_700_000_010_000, queueIndex: 0, queueTotal: 2, strategy: 'reverse_queue' },\n      { name: 'second.jpg', queueTimeMs: 1_700_000_010_000, queueFirstTimeMs: 1_700_000_000_000, queueLastTimeMs: 1_700_000_010_000, queueIndex: 1, queueTotal: 2, strategy: 'reverse_queue' }\n    ]);\n    expect(workflow.items.map((item) => item.queueTimeMs)).toEqual([1_700_000_000_000, 1_700_000_010_000]);'''
)
rep(
    'frontend/src/lib/api/client.test.ts',
    '''      queueTimeMs: [1_700_000_000_000],\n      queueIndex: [2],''',
    '''      queueTimeMs: [1_700_000_000_000],\n      queueFirstTimeMs: 1_699_999_990_000,\n      queueLastTimeMs: 1_700_000_010_000,\n      queueIndex: [2],'''
)
rep(
    'frontend/src/lib/api/client.test.ts',
    '''    expect((xhr.body as FormData).get('queue_time_ms')).toBe('1700000000000');\n    expect((xhr.body as FormData).get('queue_index')).toBe('2');''',
    '''    expect((xhr.body as FormData).get('queue_time_ms')).toBe('1700000000000');\n    expect((xhr.body as FormData).get('queue_first_time_ms')).toBe('1699999990000');\n    expect((xhr.body as FormData).get('queue_last_time_ms')).toBe('1700000010000');\n    expect((xhr.body as FormData).get('queue_index')).toBe('2');'''
)
