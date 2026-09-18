package gooru

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gooru.local/types"
)

var (
	ErrSavedSearchNotFound     = errors.New("saved search not found")
	ErrInvalidSavedSearchOrder = errInvalidSavedSearchOrder
)

func NormalizeSavedSearchSort(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "modified", "name", "size", "kind":
		return normalized
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
	_, err := parseAndValidateQuery(expression)
	return err
}

func (c *Client) SavedSearchUserID(username string) (string, error) {
	id, err := c.store.GetUserIDByUsername(username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("user %q not found", strings.TrimSpace(username))
	}
	return id, err
}

// ListSavedSearchesOrdered returns the saved-search order used by user-facing
// surfaces. It is separate from the legacy low-level list method so ordering is
// owned explicitly instead of piggybacking on metadata update timestamps.
func (c *Client) ListSavedSearchesOrdered(userID string) ([]types.SavedSearch, error) {
	return c.store.ListSavedSearchesOrdered(userID)
}

func (c *Client) ListSavedSearchesForUsername(username string) ([]types.SavedSearch, error) {
	userID, err := c.SavedSearchUserID(username)
	if err != nil {
		return nil, err
	}
	return c.ListSavedSearchesOrdered(userID)
}

func (c *Client) ReorderSavedSearches(userID string, ids []string) error {
	return c.store.ReorderSavedSearches(userID, ids)
}

func (c *Client) ReorderSavedSearchesForUsername(username string, references []string) error {
	userID, err := c.SavedSearchUserID(username)
	if err != nil {
		return err
	}
	items, err := c.ListSavedSearchesOrdered(userID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(references))
	for _, reference := range references {
		reference = strings.TrimSpace(reference)
		matched := ""
		for _, item := range items {
			if item.ID == reference || strings.EqualFold(item.Name, reference) {
				matched = item.ID
				break
			}
		}
		if matched == "" {
			return fmt.Errorf("%w: %q", ErrSavedSearchNotFound, reference)
		}
		ids = append(ids, matched)
	}
	return c.ReorderSavedSearches(userID, ids)
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
	files, err := c.GetFilesInfoByQuerySorted(item.Query, NormalizeSavedSearchSort(item.Sort), NormalizeSavedSearchOrder(item.Order), verbose)
	if err != nil {
		return nil, err
	}
	if files == nil {
		return []types.FileInfo{}, nil
	}
	return files, nil
}

func newSavedSearchID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "srch_" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return "srch_" + hex.EncodeToString(b[:])
}
