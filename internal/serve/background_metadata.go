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

const (
	// backgroundMediaMetadataTaskKind is retained for persisted
	// upload.metadata-finalize tasks created by older versions. New uploads use
	// the library-wide metadata sweep instead.
	backgroundMediaMetadataTaskKind       = "upload.metadata-finalize"
	backgroundMediaMetadataSweepBatchSize = 64
)

type deferUploadMediaMetadataContextKey struct{}

func withDeferredUploadMediaMetadata(ctx context.Context) context.Context {
	return context.WithValue(ctx, deferUploadMediaMetadataContextKey{}, true)
}

func uploadMediaMetadataDeferred(ctx context.Context) bool {
	deferred, _ := ctx.Value(deferUploadMediaMetadataContextKey{}).(bool)
	return deferred
}

func (l *GooruLibrary) mediaMetadataForFile(ctx context.Context, file types.FileInfo, analysisPath string) (types.MediaMetadata, error) {
	provider := l.metadata
	if provider == nil {
		provider = BasicMediaMetadataProvider{}
	}
	mediaType := mediaTypeForPath(file.Path)
	mediaKind := mediaKindForType(mediaType)
	metadata, err := l.importedMediaMetadata(ctx, provider, file, analysisPath, mediaType, mediaKind)
	if err != nil {
		return types.MediaMetadata{}, err
	}
	return types.MediaMetadata{
		MediaKind:       mediaKind,
		MimeType:        mediaType,
		ImageWidth:      metadata.ImageWidth,
		ImageHeight:     metadata.ImageHeight,
		VideoWidth:      metadata.VideoWidth,
		VideoHeight:     metadata.VideoHeight,
		DurationSeconds: metadata.VideoDuration,
		FrameCount:      metadata.FrameCount,
		PageCount:       metadata.PageCount,
	}, nil
}

func (l *GooruLibrary) cacheMediaMetadataForFile(ctx context.Context, file types.FileInfo, analysisPath string) error {
	metadata, err := l.mediaMetadataForFile(ctx, file, analysisPath)
	if err != nil {
		return err
	}
	metadata.LocationID = file.ID
	return l.client.UpsertMediaMetadata(metadata)
}

func (s *Server) backgroundMediaMetadataSweepHandler(ctx context.Context, task core.BackgroundTask) error {
	library, ok := s.backgroundContent.(*GooruLibrary)
	if !ok || library == nil {
		return fmt.Errorf("media metadata library is not configured")
	}
	if task.SubjectKind != "library" || task.SubjectID != "media-metadata" || strings.TrimSpace(task.OperationID) == "" {
		return fmt.Errorf("media metadata sweep task has invalid subject")
	}

	afterLocationID, err := core.MediaMetadataSweepAfterLocationID(task)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// One-row lookahead tells us whether another quantum is needed without
	// keeping this worker's shared media resource across the whole library.
	files, err := library.client.ListPendingMediaMetadataFiles(afterLocationID, backgroundMediaMetadataSweepBatchSize+1)
	if err != nil {
		return fmt.Errorf("list pending media metadata: %w", err)
	}
	if len(files) == 0 {
		return nil
	}

	hasMore := len(files) > backgroundMediaMetadataSweepBatchSize
	if hasMore {
		files = files[:backgroundMediaMetadataSweepBatchSize]
	}
	var firstErr error
	for _, file := range files {
		// Advance the keyset cursor before doing fallible work so one bad file
		// cannot prevent later locations from being attempted by a continuation.
		afterLocationID = file.ID
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := processPendingMediaMetadataFile(ctx, library, file); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if hasMore {
		// Persist the sibling before this task returns. Background task completion
		// observes active siblings transactionally, so the visible parent operation
		// cannot finish in the gap between quanta. A crash/retry may find the same
		// continuation already active; that is a successful idempotent handoff.
		if _, err := library.client.EnqueueMediaMetadataSweepContinuation(task.OperationID, afterLocationID); err != nil {
			return err
		}
	}
	return firstErr
}

func processPendingMediaMetadataFile(ctx context.Context, library *GooruLibrary, file types.FileInfo) error {
	resolved, err := library.client.ResolveManagedStorage(file)
	if err != nil {
		return fmt.Errorf("resolve media metadata storage for location %d: %w", file.ID, err)
	}
	metadata, err := library.mediaMetadataForFile(ctx, resolved, fileStoragePath(resolved))
	if err != nil {
		return fmt.Errorf("extract media metadata for location %d: %w", file.ID, err)
	}
	if _, err := library.client.UpsertMediaMetadataForLocation(file.ID, file.Hash, file.Path, metadata); err != nil {
		return fmt.Errorf("persist media metadata for location %d: %w", file.ID, err)
	}
	// A false guarded write means the location was removed, rehashed, or renamed
	// while metadata was being extracted. Its current identity remains eligible
	// for a later sweep, so there is no error to retry for the stale snapshot.
	return nil
}

// backgroundMediaMetadataHandler consumes legacy upload.metadata-finalize tasks
// that may already exist in upgraded databases. No current producer enqueues new
// tasks of this kind; removing the consumer would strand persisted recovery work.
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
