package serve

import (
	"context"
	"errors"
)

type batchPublicFileRemovalLibrary interface {
	DeleteFilesByPublicIDs(context.Context, []string) (int, error)
}

func (l *GooruLibrary) DeleteFilesByPublicIDs(ctx context.Context, publicIDs []string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return l.client.DeleteLocationsByPublicIDs(publicIDs)
}

func (s *Server) deleteFilesByPublicIDs(ctx context.Context, publicIDs []string) (int, error) {
	library, ok := s.library.(batchPublicFileRemovalLibrary)
	if !ok || library == nil {
		return 0, errors.New("batch file removal service is not configured")
	}
	return library.DeleteFilesByPublicIDs(ctx, publicIDs)
}
