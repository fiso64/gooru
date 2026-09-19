package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	core "gooru.local/gooru"
)

func (s *Server) importDurableLocalUpload(ctx context.Context, request *http.Request, reader *io.PipeReader) (UploadImportResponse, error) {
	store, ok := s.backgroundOperations.(*durableFileRemovalOperationStore)
	if !ok || store.GooruLibrary == nil || store.GooruLibrary.client == nil {
		return UploadImportResponse{}, errors.New("durable import store unavailable")
	}
	client := store.GooruLibrary.client
	runtime, err := client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
		ResourceClass: backgroundUploadResourceClass,
		WorkerID: fmt.Sprintf("local-import-%d-%d", os.Getpid(), time.Now().UnixNano()),
		Handlers: map[string]core.BackgroundTaskHandler{
			backgroundUploadTaskKind: s.backgroundUploadHandler(client),
			backgroundUploadCleanupTaskKind: s.backgroundUploadCleanupHandlerV2(client),
		},
	})
	if err != nil {
		return UploadImportResponse{}, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		runErr := runtime.Run(runCtx)
		if runCtx.Err() == nil {
			if runErr == nil { runErr = errors.New("local import worker stopped unexpectedly") }
			_ = reader.CloseWithError(runErr)
			cancel()
		}
		done <- runErr
	}()
	recorder := httptest.NewRecorder()
	s.handleDurableUpload(recorder, request.WithContext(runCtx))
	cancel()
	workerErr := <-done
	if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
		return UploadImportResponse{}, workerErr
	}
	if recorder.Code != http.StatusOK {
		return UploadImportResponse{}, fmt.Errorf("local import returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	var result UploadImportResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		return UploadImportResponse{}, err
	}
	return result, nil
}
