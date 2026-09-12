from pathlib import Path

Path('internal/database/migrations/034_background_operation_producer_claim.up.sql').write_text('''ALTER TABLE background_operations\n    ADD COLUMN producer_claimed INTEGER NOT NULL DEFAULT 0 CHECK (producer_claimed IN (0, 1));\n''')
Path('internal/database/migrations/034_background_operation_producer_claim.down.sql').write_text('''ALTER TABLE background_operations DROP COLUMN producer_claimed;\n''')
Path('internal/database/durable_work_producer_claim.go').write_text(r'''package database

import "errors"

// ClaimBackgroundOperationProducer atomically grants one producer the right
// to stage and attach the first child task for an admitted operation.
func (s *Store) ClaimBackgroundOperationProducer(operationID string) (bool, error) {
	if s == nil || s.DB == nil {
		return false, errors.New("background operation store is required")
	}
	if operationID == "" {
		return false, errors.New("background operation id is required")
	}
	result, err := s.DB.Exec(`
		UPDATE background_operations
		SET producer_claimed = 1
		WHERE id = ?
		  AND status = 'pending'
		  AND producer_claimed = 0
		  AND NOT EXISTS (
			SELECT 1 FROM background_tasks
			WHERE operation_id = background_operations.id
		  )
	`, operationID)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return changed == 1, nil
}
''')
Path('internal/database/durable_work_producer_claim_test.go').write_text(r'''package database

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func newProducerClaimTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "gooru.db"), false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := RunMigrations(store.DB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return store
}

func TestClaimBackgroundOperationProducerAllowsExactlyOneConcurrentClaim(t *testing.T) {
	store := newProducerClaimTestStore(t)
	op, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: true})
	if err != nil {
		t.Fatal(err)
	}

	var wins atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := store.ClaimBackgroundOperationProducer(op.ID)
			if err != nil {
				errs <- err
				return
			}
			if claimed {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("claim producer: %v", err)
	}
	if got := wins.Load(); got != 1 {
		t.Fatalf("successful producer claims = %d, want 1", got)
	}
}

func TestClaimBackgroundOperationProducerRejectsAttachedOperation(t *testing.T) {
	store := newProducerClaimTestStore(t)
	op, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{ID: "existing", OperationID: op.ID, DedupeKey: "existing", Kind: "upload.import"}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimBackgroundOperationProducer(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("attached operation unexpectedly accepted a producer claim")
	}
}
''')

p = Path('gooru/background.go')
s = p.read_text()
needle = '// AttachBackgroundTaskAndRevealOperation finishes producer admission after\n'
insert = '''// ClaimBackgroundOperationProducer atomically grants one producer the right to\n// stage and attach the first child task for an admitted operation.\nfunc (c *Client) ClaimBackgroundOperationProducer(operationID string) (bool, error) {\n\tif operationID == "" {\n\t\treturn false, fmt.Errorf("background operation id is required")\n\t}\n\treturn c.store.ClaimBackgroundOperationProducer(operationID)\n}\n\n'''
if needle not in s:
    raise SystemExit('background.go insertion point missing')
p.write_text(s.replace(needle, insert + needle, 1))

p = Path('internal/serve/uploads_durable.go')
s = p.read_text()
old = '''\tSetBackgroundOperationVisible(string, bool) error\n\tAttachBackgroundTaskAndRevealOperation(string, any, core.BackgroundTaskRequest) (core.BackgroundTask, error)\n'''
new = '''\tSetBackgroundOperationVisible(string, bool) error\n\tClaimBackgroundOperationProducer(string) (bool, error)\n\tAttachBackgroundTaskAndRevealOperation(string, any, core.BackgroundTaskRequest) (core.BackgroundTask, error)\n'''
if old not in s:
    raise SystemExit('durable interface insertion point missing')
s = s.replace(old, new, 1)
old = '''func (l *GooruLibrary) AttachBackgroundTaskAndRevealOperation(operationID string, checkpoint any, request core.BackgroundTaskRequest) (core.BackgroundTask, error) {\n\treturn l.client.AttachBackgroundTaskAndRevealOperation(operationID, checkpoint, request)\n}\n'''
new = '''func (l *GooruLibrary) ClaimBackgroundOperationProducer(operationID string) (bool, error) {\n\treturn l.client.ClaimBackgroundOperationProducer(operationID)\n}\n\n''' + old
if old not in s:
    raise SystemExit('library wrapper insertion point missing')
s = s.replace(old, new, 1)
old = '''\tif tasks, ok := operations.(durableUploadTaskReader); ok {\n\t\tif _, found, err := tasks.GetBackgroundOperationTask(operationID); err != nil {\n\t\t\treturn core.BackgroundOperation{}, false, err\n\t\t} else if found {\n\t\t\treturn core.BackgroundOperation{}, false, errors.New("upload reservation was already claimed")\n\t\t}\n\t}\n\treturn core.BackgroundOperation{ID: state.ID, Kind: state.Kind, Visible: state.Visible, ProgressTotal: state.ProgressTotal, CreatedAt: state.CreatedAt}, false, nil\n'''
new = '''\tclaimed, err := operations.ClaimBackgroundOperationProducer(operationID)\n\tif err != nil {\n\t\treturn core.BackgroundOperation{}, false, err\n\t}\n\tif !claimed {\n\t\treturn core.BackgroundOperation{}, false, errors.New("upload reservation was already claimed")\n\t}\n\treturn core.BackgroundOperation{ID: state.ID, Kind: state.Kind, Visible: state.Visible, ProgressTotal: state.ProgressTotal, CreatedAt: state.CreatedAt}, false, nil\n'''
if old not in s:
    raise SystemExit('claim replacement point missing')
p.write_text(s.replace(old, new, 1))

p = Path('internal/serve/uploads_durable_test.go')
s = p.read_text()
old = 'type durableUploadTestStore struct {\n'
if old not in s:
    raise SystemExit('test store type missing')
s = s.replace(old, old + '\tproducerClaimed     bool\n', 1)
needle = '''func (s *durableUploadTestStore) SetBackgroundOperationCheckpoint(operationID string, checkpoint any) error {\n'''
insert = '''func (s *durableUploadTestStore) ClaimBackgroundOperationProducer(operationID string) (bool, error) {\n\tif operationID != s.operation.ID || s.state.Status != core.BackgroundWorkPending {\n\t\treturn false, nil\n\t}\n\tif s.producerClaimed {\n\t\treturn false, nil\n\t}\n\ts.producerClaimed = true\n\treturn true, nil\n}\n\n'''
if needle not in s:
    raise SystemExit('test store method insertion point missing')
s = s.replace(needle, insert + needle, 1)
# Add a focused server-boundary regression before the first existing Test function.
needle = 'func TestDurableUpload'
idx = s.find(needle)
if idx < 0:
    raise SystemExit('durable upload test insertion point missing')
test = r'''func TestClaimDurableUploadOperationRejectsDuplicateProducerClaim(t *testing.T) {
	store := newDurableUploadTestStore()
	store.state.Visible = true
	request := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
	request.Header.Set(uploadOperationHeader, store.operation.ID)
	server := &Server{}

	if _, created, err := server.claimDurableUploadOperation(request, store); err != nil || created {
		t.Fatalf("first reservation claim: created=%v err=%v", created, err)
	}
	if _, _, err := server.claimDurableUploadOperation(request, store); err == nil {
		t.Fatal("duplicate reservation claim unexpectedly succeeded")
	}
	if store.cancelCalls != 0 {
		t.Fatalf("duplicate claim canceled operation %d times", store.cancelCalls)
	}
}

'''
s = s[:idx] + test + s[idx:]
p.write_text(s)
