package serve

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	core "gooru.local/gooru"
)

const uploadSegmentIndexHeader = "X-Gooru-Upload-Segment-Index"

type durableUploadSegmentStore interface {
	GetBackgroundTask(string) (core.BackgroundTaskState, bool, error)
	AttachBackgroundTaskToOperation(string, string, any, core.BackgroundTaskRequest) (core.BackgroundTask, bool, error)
}

type durableUploadAdmission struct {
	operation       core.BackgroundOperation
	created         bool
	segmented       bool
	segmentIndex    int64
	taskID          string
	alreadyAttached bool
}

func (l *GooruLibrary) GetBackgroundTask(taskID string) (core.BackgroundTaskState, bool, error) {
	return l.client.GetBackgroundTask(taskID)
}

func (l *GooruLibrary) AttachBackgroundTaskToOperation(operationID, taskID string, checkpoint any, request core.BackgroundTaskRequest) (core.BackgroundTask, bool, error) {
	return l.client.AttachBackgroundTaskToOperation(operationID, taskID, checkpoint, request)
}

func (s *Server) admitDurableUpload(r *http.Request, operations durableUploadOperationStore) (durableUploadAdmission, error) {
	segmentIndex, segmented, err := durableUploadSegmentIndex(r)
	if err != nil {
		return durableUploadAdmission{}, err
	}
	if !segmented {
		operation, created, err := s.claimDurableUploadOperation(r, operations)
		return durableUploadAdmission{operation: operation, created: created}, err
	}
	if !PreferAsync(r) {
		return durableUploadAdmission{}, errors.New("segmented uploads require Prefer: respond-async")
	}
	operationID := strings.TrimSpace(r.Header.Get(uploadOperationHeader))
	if !validDurableUploadOperationID(operationID) {
		return durableUploadAdmission{}, errors.New("upload reservation is invalid")
	}
	segmentStore, ok := any(operations).(durableUploadSegmentStore)
	if !ok {
		return durableUploadAdmission{}, errors.New("segmented upload service is not configured")
	}
	state, found, err := operations.GetBackgroundOperation(operationID)
	if err != nil {
		return durableUploadAdmission{}, err
	}
	if !found || state.Kind != backgroundUploadImportOperationKind || !state.Visible {
		return durableUploadAdmission{}, errors.New("upload reservation was not found")
	}
	if state.ProgressTotal <= 0 || segmentIndex >= state.ProgressTotal {
		return durableUploadAdmission{}, fmt.Errorf("upload segment index %d is outside reservation size %d", segmentIndex, state.ProgressTotal)
	}
	taskID := durableUploadSegmentTaskID(operationID, segmentIndex)
	if task, found, err := segmentStore.GetBackgroundTask(taskID); err != nil {
		return durableUploadAdmission{}, err
	} else if found {
		if !durableUploadSegmentTaskMatches(task, operationID) {
			return durableUploadAdmission{}, errors.New("upload segment identity conflicts with existing durable task")
		}
		return durableUploadAdmission{
			operation: core.BackgroundOperation{ID: state.ID, Kind: state.Kind, Visible: state.Visible, ProgressTotal: state.ProgressTotal, CreatedAt: state.CreatedAt},
			segmented: true, segmentIndex: segmentIndex, taskID: taskID, alreadyAttached: true,
		}, nil
	}
	if state.Status != core.BackgroundWorkPending && state.Status != core.BackgroundWorkRunning {
		return durableUploadAdmission{}, errors.New("upload reservation is not active")
	}
	return durableUploadAdmission{
		operation: core.BackgroundOperation{ID: state.ID, Kind: state.Kind, Visible: state.Visible, ProgressTotal: state.ProgressTotal, CreatedAt: state.CreatedAt},
		segmented: true, segmentIndex: segmentIndex, taskID: taskID,
	}, nil
}

func durableUploadSegmentIndex(r *http.Request) (int64, bool, error) {
	values := r.Header.Values(uploadSegmentIndexHeader)
	if len(values) == 0 {
		return 0, false, nil
	}
	if len(values) != 1 {
		return 0, true, errors.New("upload segment index must be one non-negative integer")
	}
	value := strings.TrimSpace(values[0])
	segmentIndex, err := strconv.ParseInt(value, 10, 64)
	if err != nil || segmentIndex < 0 {
		return 0, true, errors.New("upload segment index must be a non-negative integer")
	}
	return segmentIndex, true, nil
}

func durableUploadSegmentTaskID(operationID string, segmentIndex int64) string {
	return "task-upload-segment-" + operationID + "-" + strconv.FormatInt(segmentIndex, 10)
}

func durableUploadSegmentTaskMatches(task core.BackgroundTaskState, operationID string) bool {
	return task.OperationID == operationID && task.Kind == backgroundUploadTaskKind && task.SubjectKind == "operation" && task.SubjectID == operationID
}

func writeDurableUploadAccepted(w http.ResponseWriter, s *Server, operations durableUploadOperationStore, operationID string) error {
	state, found, err := operations.GetBackgroundOperation(operationID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("upload operation disappeared")
	}
	w.Header().Set("Location", "/api/v1/operations/"+operationID)
	writeJSON(w, http.StatusAccepted, s.backgroundOperationDTO(state))
	return nil
}
