package serve

import (
	"context"
	"errors"
	"fmt"
	"strings"

	core "gooru.local/gooru"
	"gooru.local/types"
)

const backgroundMediaMetadataTaskKind = "media.metadata"

type deferUploadMediaMetadataContextKey struct{}

func withDeferredUploadMediaMetadata(ctx context.Context) context.Context {
	return context.WithValue(ctx, deferUploadMediaMetadataContextKey{}, true)
}

func uploadMediaMetadataDeferred(ctx context.Context) bool {
	deferred, _ := ctx.Value(deferUploadMediaMetadataContextKey{}).(bool)
	return deferred
}

func backgroundMediaMetadataTaskRequest(location types.LocationInfo) core.BackgroundTaskRequest {
	return core.BackgroundTaskRequest{
		DedupeKey:     "metadata:" + location.Hash,
		Kind:          backgroundMediaMetadataTaskKind,
		SubjectKind:   "content",
		SubjectID:     location.Hash,
		InputKey:      location.Hash,
		ResourceClass: backgroundThumbnailResourceClass,
		MaxAttempts:   5,
	}
}

func (m *MediaService) backgroundUploadTaskRequests(location types.LocationInfo) []core.BackgroundTaskRequest {
	tasks := append([]core.BackgroundTaskRequest(nil), m.backgroundTaskRequests(location)...)
	return append(tasks, backgroundMediaMetadataTaskRequest(location))
}

func (l *GooruLibrary) cacheMediaMetadataForFile(ctx context.Context, file types.FileInfo, analysisPath string) error {
	if file.Metadata != nil {
		return nil
	}
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
	if metadata.ImageWidth == nil && metadata.ImageHeight == nil && metadata.VideoWidth == nil && metadata.VideoHeight == nil && metadata.VideoDuration == nil && metadata.FrameCount == nil && metadata.PageCount == nil {
		return nil
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
	if task.SubjectKind != "content" || strings.TrimSpace(task.SubjectID) == "" {
		return fmt.Errorf("media metadata task has invalid content identity")
	}
	file, err := library.GetFileByContentHash(ctx, task.SubjectID)
	if errors.Is(err, core.ErrContentNotTracked) {
		return nil
	}
	if err != nil {
		return err
	}
	return library.cacheMediaMetadataForFile(ctx, file, fileStoragePath(file))
}
