package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	core "gooru.local/gooru"
	"gooru.local/types"
)

const (
	backgroundThumbnailTaskKind      = "media.thumbnail"
	backgroundThumbnailResourceClass = "media"
)

type contentHashLibrary interface {
	GetFileByContentHash(context.Context, string) (types.FileInfo, error)
}

func (m *MediaService) browsingThumbnailSpec(file types.FileInfo) (size int, format string, relativePath string, ok bool) {
	if m == nil || len(m.cfg.Media.ThumbnailSizes) == 0 || m.thumbnailer == nil {
		return 0, "", "", false
	}
	mediaKind := mediaKindForType(mediaTypeForPath(file.Path))
	if mediaKind != "photo" && mediaKind != "gif" && mediaKind != "video" && !strings.EqualFold(filepath.Ext(file.Path), ".cbz") {
		return 0, "", "", false
	}
	size = m.cfg.Media.ThumbnailSizes[0]
	format = strings.ToLower(strings.TrimSpace(m.cfg.Media.ThumbnailFormat))
	if format == "" {
		format = "jpeg"
	}
	return size, format, m.derivativeRelativePath(file, "thumbnail", size, format), true
}

func (m *MediaService) backgroundTaskRequests(location types.LocationInfo) []core.BackgroundTaskRequest {
	file := types.FileInfo{Path: location.Path, Hash: location.Hash}
	_, _, inputKey, ok := m.browsingThumbnailSpec(file)
	if !ok {
		return nil
	}
	return []core.BackgroundTaskRequest{{
		DedupeKey:     "thumbnail:" + inputKey,
		Kind:          backgroundThumbnailTaskKind,
		SubjectKind:   "content",
		SubjectID:     location.Hash,
		InputKey:      inputKey,
		ResourceClass: backgroundThumbnailResourceClass,
		MaxAttempts:   5,
	}}
}

func (m *MediaService) ensureBrowsingThumbnail(file types.FileInfo) error {
	size, format, relativePath, ok := m.browsingThumbnailSpec(file)
	if !ok {
		return nil
	}
	if m.derivativeStoreErr != nil {
		return m.derivativeStoreErr
	}
	if m.derivatives == nil {
		return fmt.Errorf("media derivative store is not configured")
	}
	artifact, err := m.derivatives.GetOrGenerate(relativePath, func(dst io.Writer) error {
		return m.generateDerivative(file, dst, size, format, "thumbnail")
	})
	if err != nil {
		return err
	}
	return artifact.Close()
}

func (s *Server) backgroundThumbnailHandler(ctx context.Context, task core.BackgroundTask) error {
	if s.backgroundContent == nil {
		return fmt.Errorf("content hash lookup is not configured")
	}
	if task.SubjectKind != "content" || strings.TrimSpace(task.SubjectID) == "" {
		return fmt.Errorf("thumbnail task has invalid content identity")
	}
	file, err := s.backgroundContent.GetFileByContentHash(ctx, task.SubjectID)
	if errors.Is(err, core.ErrContentNotTracked) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.media.ensureBrowsingThumbnail(file)
}

func (s *Server) NewBackgroundRuntime(client *core.Client, workerID string) (BackgroundRuntime, error) {
	if client == nil {
		return nil, fmt.Errorf("background client is required")
	}
	client.SetFileRegistrationHooks(backgroundMediaMetadataRegistrationHook)
	if err := recoverBackgroundOperationReservations(client, s.cfg.Uploads.Targets); err != nil {
		return nil, err
	}
	mediaRuntime, err := client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
		ResourceClass: backgroundThumbnailResourceClass,
		WorkerID:      workerID + "-media",
		Handlers: map[string]core.BackgroundTaskHandler{
			backgroundThumbnailTaskKind:          s.backgroundThumbnailHandler,
			backgroundMediaMetadataTaskKind:      s.backgroundMediaMetadataHandler,
			backgroundMediaMetadataSweepTaskKind: s.backgroundMediaMetadataSweepHandler,
		},
	})
	if err != nil {
		return nil, err
	}
	storageRuntime, err := client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
		ResourceClass: backgroundFileRemovalResourceClass,
		WorkerID:      workerID + "-storage",
		Handlers: map[string]core.BackgroundTaskHandler{
			backgroundFileRemovalTaskKind:        s.backgroundFileRemovalHandler,
			backgroundFileRemovalCleanupTaskKind: s.backgroundFileRemovalCleanupHandler(client),
		},
	})
	if err != nil {
		return nil, err
	}
	uploadRuntime, err := client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
		ResourceClass: backgroundUploadResourceClass,
		WorkerID:      workerID + "-upload",
		Handlers: map[string]core.BackgroundTaskHandler{
			backgroundUploadTaskKind:        s.backgroundUploadHandler(client),
			backgroundUploadCleanupTaskKind: s.backgroundUploadCleanupHandlerV2(client),
		},
	})
	if err != nil {
		return nil, err
	}
	metadataRuntime, err := client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
		ResourceClass: core.BackgroundTagMutationResourceClass,
		WorkerID:      workerID + "-metadata",
		Handlers: map[string]core.BackgroundTaskHandler{
			core.BackgroundTagMutationTaskKind: s.backgroundTagMutationHandler,
		},
	})
	if err != nil {
		return nil, err
	}
	return multiBackgroundRuntime{runtimes: []BackgroundRuntime{mediaRuntime, storageRuntime, uploadRuntime, metadataRuntime}}, nil
}
