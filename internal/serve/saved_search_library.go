package serve

import (
	"context"

	"gooru.local/types"
)

func (l *GooruLibrary) ListSavedSearches(ctx context.Context, userID string) ([]types.SavedSearch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return l.client.ListSavedSearchesOrdered(userID)
}

func (l *GooruLibrary) CreateSavedSearch(ctx context.Context, userID string, req savedSearchRequest) (types.SavedSearch, error) {
	if err := ctx.Err(); err != nil {
		return types.SavedSearch{}, err
	}
	return l.client.CreateSavedSearchForUser(userID, req.Name, req.Query, req.Sort, req.Order)
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

func (l *GooruLibrary) ReorderSavedSearches(ctx context.Context, userID string, ids []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return l.client.ReorderSavedSearches(userID, ids)
}

func (l *GooruLibrary) DeleteSavedSearch(ctx context.Context, userID string, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return l.client.DeleteSavedSearch(userID, id)
}
