package gooru

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gooru.local/internal/query"
	"gooru.local/types"
)

var ErrSavedSearchNotFound = errors.New("saved search not found")

func NormalizeSavedSearchSort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "modified", "name", "size", "kind":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "name"
	}
}

func NormalizeSavedSearchOrder(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "desc") {
		return "desc"
	}
	return "asc"
}

func ValidateSavedSearchQuery(expression string) error {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil
	}
	ast, err := query.Parse(expression)
	if err != nil {
		return fmt.Errorf("%w: could not parse query: %v", ErrInvalidQuery, err)
	}
	if err := query.ValidateAST(ast); err != nil {
		return fmt.Errorf("%w: invalid tag in query: %v", ErrInvalidQuery, err)
	}
	return nil
}

func (c *Client) SavedSearchUserID(username string) (string, error) {
	id, err := c.store.GetUserIDByUsername(username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("user %q not found", strings.TrimSpace(username))
	}
	return id, err
}

func (c *Client) ListSavedSearchesForUsername(username string) ([]types.SavedSearch, error) {
	userID, err := c.SavedSearchUserID(username)
	if err != nil {
		return nil, err
	}
	return c.ListSavedSearches(userID)
}

func (c *Client) CreateSavedSearchForUser(userID, name, expression, sort, order string) (types.SavedSearch, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return types.SavedSearch{}, errors.New("saved search name is required")
	}
	expression = strings.TrimSpace(expression)
	if err := ValidateSavedSearchQuery(expression); err != nil {
		return types.SavedSearch{}, err
	}
	return c.CreateSavedSearch(types.SavedSearch{
		ID:     newSavedSearchID(),
		UserID: userID,
		Name:   name,
		Query:  expression,
		Sort:   NormalizeSavedSearchSort(sort),
		Order:  NormalizeSavedSearchOrder(order),
	})
}

func (c *Client) CreateSavedSearchForUsername(username, name, expression, sort, order string) (types.SavedSearch, error) {
	userID, err := c.SavedSearchUserID(username)
	if err != nil {
		return types.SavedSearch{}, err
	}
	return c.CreateSavedSearchForUser(userID, name, expression, sort, order)
}

func (c *Client) SavedSearchForUsername(username, reference string) (types.SavedSearch, error) {
	items, err := c.ListSavedSearchesForUsername(username)
	if err != nil {
		return types.SavedSearch{}, err
	}
	reference = strings.TrimSpace(reference)
	for _, item := range items {
		if item.ID == reference || strings.EqualFold(item.Name, reference) {
			return item, nil
		}
	}
	return types.SavedSearch{}, fmt.Errorf("%w: %q", ErrSavedSearchNotFound, reference)
}

func (c *Client) SearchSavedSearchForUsername(username, reference string, verbose bool) ([]types.FileInfo, error) {
	item, err := c.SavedSearchForUsername(username, reference)
	if err != nil {
		return nil, err
	}
	count, err := c.CountFileLocationsByQuery(item.Query, verbose)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return []types.FileInfo{}, nil
	}
	return c.GetFilesInfoByQueryPageSortedOffset(item.Query, count, 0, NormalizeSavedSearchSort(item.Sort), NormalizeSavedSearchOrder(item.Order), verbose)
}

func newSavedSearchID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "srch_" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return "srch_" + hex.EncodeToString(b[:])
}
