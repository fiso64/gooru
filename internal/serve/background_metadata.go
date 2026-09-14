package serve

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	core "gooru.local/gooru"
	"gooru.local/types"
)

const backgroundMediaMetadataTaskKind = "upload.metadata-finalize"

type deferUploadMediaMetadataContextKey struct{}

func withDeferredUploadMediaMetadata(ctx context.Context) context.Context {
	return context.WithValue(ctx, deferUploadMediaMetadataContextKey{}, true)
}

func uploadMediaMetadataDeferred(ctx context.Context) bool {
	deferred, _ := ctx.Value(deferUploadMediaMetadataContextKey{}).(bool)
	return deferred
}

type backgroundTaskEnqueuer interface {
	EnqueueBackgroundTask(core.BackgroundTaskRequest) (core.BackgroundTask, bool, error)
}

type backgroundUploadFinalizingStore struct {
	backgroundUploadWorkerStore
	tasks backgroundTaskEnqueuer
}

type backgroundUploadTaskFinalizingStore struct {
	backgroundUploadFinalizingStore
	backgroundUploadTaskStateStore
}

func newBackgroundUploadFinalizingStore(store backgroundUploadWorkerStore, tasks backgroundTaskEnqueuer) backgroundUploadWorkerStore {
	finalizing := backgroundUploadFinalizingStore{
		backgroundUploadWorkerStore: store,
		tasks:                       tasks,
	}
	taskStore, ok := store.(backgroundUploadTaskStateStore)
	if !ok {
		return finalizing
	}
	return backgroundUploadTaskFinalizingStore{
		backgroundUploadFinalizingStore: finalizing,
		backgroundUploadTaskStateStore:  taskStore,
	}
}

func (s backgroundUploadFinalizingStore) SetBackgroundOperationResult(operationID string, result any) error {
	if s.tasks != nil {
		if _, _, err := s.tasks.EnqueueBackgroundTask(backgroundMediaMetadataTaskRequest(operationID)); err != nil {
			return fmt.Errorf("enqueue upload metadata finalizer: %w", err)
		}
	}
	return s.backgroundUploadWorkerStore.SetBackgroundOperationResult(operationID, result)
}

func backgroundMediaMetadataTaskRequest(operationID string) core.BackgroundTaskRequest {
	return core.BackgroundTaskRequest{
		OperationID:   operationID,
		DedupeKey:     operationID + ":metadata-finalize",
		Kind:          backgroundMediaMetadataTaskKind,
		SubjectKind:   "operation",
		SubjectID:     operationID,
		InputKey:      operationID,
		ResourceClass: backgroundThumbnailResourceClass,
		MaxAttempts:   5,
	}
}

func (l *GooruLibrary) cacheMediaMetadataForFile(ctx context.Context, file types.FileInfo, analysisPath string) error {
	provider := l.metadata
	if provider == nil {
		provider = BasicMediaMetadataProvider{}
	}
	mediaType := mediaTypeForPath(file.Path)
	mediaKind := mediaKindForType(mediaType)
	metadata, err := l.importedMediaMetadata(ctx, provider, file, analysisPath, mediaType, mediaKind)
	if err != nil {
		return err
	}
	return l.client.UpsertMediaMetadata(types.MediaMetadata{
		LocationID:      file.ID,
		MediaKind:       mediaKind,
		MimeType:        mediaType,
		ImageWidth:      metadata.ImageWidth,
		ImageHeight:     metadata.ImageHeight,
		VideoWidth:      metadata.VideoWidth,
		VideoHeight:     metadata.VideoHeight,
		DurationSeconds: metadata.VideoDuration,
		FrameCount:      metadata.FrameCount,
		PageCount:       metadata.PageCount,
	})
}

func (s *Server) backgroundMediaMetadataHandler(ctx context.Context, task core.BackgroundTask) error {
	library, ok := s.backgroundContent.(*GooruLibrary)
	if !ok || library == nil {
		return fmt.Errorf("media metadata library is not configured")
	}
	if task.SubjectKind != "operation" || strings.TrimSpace(task.SubjectID) == "" || task.SubjectID != task.OperationID {
		return fmt.Errorf("media metadata task has invalid operation identity")
	}
	var checkpoint backgroundUploadCheckpoint
	found, err := library.client.GetBackgroundOperationCheckpoint(task.OperationID, &checkpoint)
	if err != nil {
		return err
	}
	if !found || checkpoint.Phase != backgroundUploadPhaseImported || checkpoint.Response == nil {
		return fmt.Errorf("media metadata task is missing imported upload checkpoint")
	}
	for _, uploaded := range checkpoint.Response.Files {
		if uploaded.Status != "imported" {
			continue
		}
		target, err := s.uploadTarget(uploaded.TargetID)
		if err != nil {
			return err
		}
		file, err := library.client.GetFileInfoByPath(filepath.Join(target.Path, uploaded.Name))
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		file, err = library.client.ResolveManagedStorage(file)
		if err != nil {
			return fmt.Errorf("resolve media metadata storage: %w", err)
		}
		if err := library.cacheMediaMetadataForFile(ctx, file, fileStoragePath(file)); err != nil {
			return err
		}
	}
	return nil
}
