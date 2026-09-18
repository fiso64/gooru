package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	core "gooru.local/gooru"
)

const defaultDurableTagMutationPendingLimit = 64

type durableTagMutationLibrary interface {
	createBackgroundTagMutation(context.Context, TagOperation, TagMutationSelector, TagMutationRequest, int) (core.BackgroundOperation, error)
	executeBackgroundTagMutation(context.Context, core.BackgroundTask) error
}

type backgroundTagMutationResultReader interface {
	backgroundTagMutationResponse(string) (TagMutationResponse, bool, error)
}

func (l *GooruLibrary) createBackgroundTagMutation(
	ctx context.Context,
	operation TagOperation,
	selector TagMutationSelector,
	request TagMutationRequest,
	maxPending int,
) (core.BackgroundOperation, error) {
	if err := ctx.Err(); err != nil {
		return core.BackgroundOperation{}, err
	}
	excludedHashes := make([]string, 0, len(request.ExcludeFileIDs))
	for _, encoded := range request.ExcludeFileIDs {
		file, err := l.GetFileByPublicID(ctx, encoded)
		if err != nil {
			return core.BackgroundOperation{}, err
		}
		excludedHashes = append(excludedHashes, file.Hash)
	}
	return l.client.CreateBackgroundTagMutation(core.BackgroundTagMutationRequest{
		Mutation:       string(operation),
		Selector:       selector,
		Tags:           request.Tags,
		FileIDs:        request.FileIDs,
		Query:          request.Query,
		FileIDSelector: request.Query == "",
		ExcludedHashes: excludedHashes,
		MaxPending:     maxPending,
	})
}

func (l *GooruLibrary) executeBackgroundTagMutation(ctx context.Context, task core.BackgroundTask) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if task.Kind != core.BackgroundTagMutationTaskKind ||
		task.SubjectKind != "operation" ||
		task.SubjectID == "" ||
		task.SubjectID != task.OperationID ||
		task.InputKey != "v1" {
		return errors.New("tag mutation background task has invalid identity")
	}
	state, found, err := l.client.GetBackgroundTagMutation(task.OperationID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("tag mutation background state is missing")
	}
	if state.ResultReady {
		return nil
	}
	switch state.TargetKind {
	case core.BackgroundTagMutationTargetContentHash:
		return l.client.ExecuteClaimedBackgroundTagMutationQuery(task)
	case core.BackgroundTagMutationTargetFileID:
		targets, err := l.client.ListBackgroundTagMutationTargets(task.OperationID)
		if err != nil {
			return err
		}
		files, err := l.GetFilesByPublicIDs(ctx, targets)
		if err != nil {
			return err
		}
		return l.client.ExecuteClaimedBackgroundTagMutationFiles(task, files)
	default:
		return fmt.Errorf("tag mutation background state has invalid target kind %q", state.TargetKind)
	}
}

func (l *GooruLibrary) backgroundTagMutationResponse(operationID string) (TagMutationResponse, bool, error) {
	state, found, err := l.client.GetBackgroundTagMutation(operationID)
	if err != nil || !found {
		return TagMutationResponse{}, found, err
	}
	if !state.ResultReady {
		return TagMutationResponse{}, false, nil
	}
	var selector TagMutationSelector
	if err := json.Unmarshal(state.SelectorJSON, &selector); err != nil {
		return TagMutationResponse{}, false, fmt.Errorf("decode background tag mutation selector: %w", err)
	}
	return TagMutationResponse{
		Operation:     TagOperation(state.Mutation),
		Selector:      selector,
		MatchedFiles:  state.MatchedFiles,
		AffectedCount: state.AffectedCount,
		Notifications: notificationDTOs(state.Notifications),
	}, true, nil
}

func (s *Server) backgroundTagMutationHandler(ctx context.Context, task core.BackgroundTask) error {
	mutator, ok := s.library.(durableTagMutationLibrary)
	if !ok || mutator == nil {
		return errors.New("durable tag mutation service is not configured")
	}
	return mutator.executeBackgroundTagMutation(ctx, task)
}

func waitForDurableTagMutation(ctx context.Context, operations backgroundOperationReader, operationID string) (TagMutationResponse, error) {
	var changes <-chan struct{}
	var unsubscribe func()
	if subscriber, ok := operations.(backgroundOperationChangeSubscriber); ok {
		changes, unsubscribe = subscriber.SubscribeBackgroundOperationChanges()
		if unsubscribe != nil {
			defer unsubscribe()
		}
	}
	pollInterval := 25 * time.Millisecond
	if changes != nil {
		pollInterval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	resultReader, _ := operations.(backgroundTagMutationResultReader)
	for {
		state, found, err := operations.GetBackgroundOperation(operationID)
		if err != nil {
			return TagMutationResponse{}, err
		}
		if !found {
			return TagMutationResponse{}, errors.New("tag mutation operation disappeared")
		}
		switch state.Status {
		case core.BackgroundWorkCompleted:
			if resultReader != nil {
				response, ready, err := resultReader.backgroundTagMutationResponse(operationID)
				if err != nil {
					return TagMutationResponse{}, err
				}
				if ready {
					return response, nil
				}
				return TagMutationResponse{}, errors.New("completed tag mutation operation has no durable tag result")
			}
			var response TagMutationResponse
			found, err := operations.GetBackgroundOperationResult(operationID, &response)
			if err != nil {
				return TagMutationResponse{}, err
			}
			if !found {
				return TagMutationResponse{}, errors.New("completed tag mutation operation has no result")
			}
			return response, nil
		case core.BackgroundWorkFailed:
			if state.ErrorMessage != "" {
				return TagMutationResponse{}, errors.New(state.ErrorMessage)
			}
			return TagMutationResponse{}, errors.New("tag mutation operation failed")
		case core.BackgroundWorkCanceled:
			return TagMutationResponse{}, context.Canceled
		}
		select {
		case <-ctx.Done():
			return TagMutationResponse{}, ctx.Err()
		case _, open := <-changes:
			if !open {
				changes = nil
			}
		case <-ticker.C:
		}
	}
}

func writeDurableTagMutationAdmissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrBackgroundOperationPendingLimit):
		writeError(w, http.StatusServiceUnavailable, "job_queue_full", "job queue is full", nil)
	case errors.Is(err, core.ErrInvalidQuery):
		writeError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to accept tag mutation", nil)
	}
}
