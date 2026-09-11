package serve

import (
	"errors"
	"io"
	"time"

	core "gooru.local/gooru"
)

const (
	uploadReceivingProgressSteps       int64 = 50
	uploadReceivingUnknownByteStep     int64 = 4 << 20
	uploadReceivingProgressMaxInterval       = 250 * time.Millisecond
)

var errUploadReceivingCanceled = errors.New("upload operation was canceled while receiving request body")

type uploadReceivingProgressStore interface {
	SetBackgroundOperationCheckpoint(string, any) error
	GetBackgroundOperation(string) (core.BackgroundOperationState, bool, error)
}

type uploadReceivingProgressReadCloser struct {
	io.ReadCloser
	operationID   string
	total         int64
	received      int64
	lastPersisted int64
	lastAt        time.Time
	store         uploadReceivingProgressStore
}

func newUploadReceivingProgressReadCloser(body io.ReadCloser, operationID string, total int64, store uploadReceivingProgressStore) *uploadReceivingProgressReadCloser {
	if total < 0 {
		total = 0
	}
	return &uploadReceivingProgressReadCloser{ReadCloser: body, operationID: operationID, total: total, lastAt: time.Now(), store: store}
}

func (r *uploadReceivingProgressReadCloser) Read(p []byte) (int, error) {
	n, readErr := r.ReadCloser.Read(p)
	if n > 0 {
		r.received += int64(n)
	}
	force := errors.Is(readErr, io.EOF)
	if n > 0 || force {
		if err := r.persist(force); err != nil {
			if n > 0 {
				return n, err
			}
			return 0, err
		}
	}
	return n, readErr
}

func (r *uploadReceivingProgressReadCloser) persist(force bool) error {
	step := uploadReceivingUnknownByteStep
	if r.total > 0 {
		step = r.total / uploadReceivingProgressSteps
		if step < 1 {
			step = 1
		}
	}
	if !force && r.received-r.lastPersisted < step && time.Since(r.lastAt) < uploadReceivingProgressMaxInterval {
		return nil
	}
	checkpoint := backgroundUploadReceivingCheckpoint(r.total, r.received)
	if err := r.store.SetBackgroundOperationCheckpoint(r.operationID, checkpoint); err != nil {
		state, found, stateErr := r.store.GetBackgroundOperation(r.operationID)
		if stateErr == nil && found && state.Status == core.BackgroundWorkCanceled {
			return errUploadReceivingCanceled
		}
		// Progress publication is observability, not the upload commit boundary.
		// A transient checkpoint failure must not corrupt an otherwise valid body;
		// later reads retry and the staged checkpoint replaces this state atomically.
		return nil
	}
	r.lastPersisted = r.received
	r.lastAt = time.Now()
	return nil
}
