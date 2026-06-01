package serve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"gooru.local/types"
)

func (l *GooruLibrary) ListSavedSearches(ctx context.Context, userID string) ([]types.SavedSearch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return l.client.ListSavedSearches(userID)
}

func (l *GooruLibrary) CreateSavedSearch(ctx context.Context, userID string, req savedSearchRequest) (types.SavedSearch, error) {
	if err := ctx.Err(); err != nil {
		return types.SavedSearch{}, err
	}
	return l.client.CreateSavedSearch(types.SavedSearch{
		ID:     newSavedSearchID(),
		UserID: userID,
		Name:   req.Name,
		Query:  req.Query,
		Sort:   req.Sort,
		Order:  req.Order,
	})
}

func (l *GooruLibrary) UpdateSavedSearch(ctx context.Context, userID string, id string, req savedSearchRequest) (types.SavedSearch, error) {
	if err := ctx.Err(); err != nil {
		return types.SavedSearch{}, err
	}
	return l.client.UpdateSavedSearch(types.SavedSearch{
		ID:     id,
		UserID: userID,
		Name:   req.Name,
		Query:  req.Query,
		Sort:   req.Sort,
		Order:  req.Order,
	})
}

func (l *GooruLibrary) DeleteSavedSearch(ctx context.Context, userID string, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return l.client.DeleteSavedSearch(userID, id)
}

func newSavedSearchID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "srch_" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return "srch_" + hex.EncodeToString(b[:])
}
