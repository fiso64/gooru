package serve

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gooru.local/types"
)

const (
	DefaultPageLimit = 50
	MaxPageLimit     = 200
)

type Page struct {
	Limit  int
	Offset int
	Cursor *types.PageCursor
}

type PageResult[T any] struct {
	Items         []T
	NextPageToken string
}

func ParsePage(limitRaw string, token string) (Page, error) {
	limit := DefaultPageLimit
	if limitRaw != "" {
		parsed, err := strconv.Atoi(limitRaw)
		if err != nil || parsed <= 0 {
			return Page{}, fmt.Errorf("limit must be a positive integer")
		}
		if parsed > MaxPageLimit {
			parsed = MaxPageLimit
		}
		limit = parsed
	}
	offset := 0
	if token != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			return Page{}, fmt.Errorf("page_token is invalid")
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) == 2 && parts[0] == "offset" {
			parsed, err := strconv.Atoi(parts[1])
			if err != nil || parsed < 0 {
				return Page{}, fmt.Errorf("page_token is invalid")
			}
			offset = parsed
		} else {
			var cursor types.PageCursor
			if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.ID <= 0 {
				return Page{}, fmt.Errorf("page_token is invalid")
			}
			return Page{Limit: limit, Cursor: &cursor}, nil
		}
	}
	return Page{Limit: limit, Offset: offset}, nil
}

func CursorPageToken(sort string, order string, id int64) string {
	if id <= 0 {
		return ""
	}
	payload, err := json.Marshal(types.PageCursor{Sort: sort, Order: order, ID: id})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func ValidateCursorForSort(cursor *types.PageCursor, sort string, order string) (*types.PageCursor, error) {
	if cursor == nil {
		return nil, nil
	}
	if cursor.Sort != sort || cursor.Order != order {
		return nil, fmt.Errorf("page_token sort does not match request")
	}
	if cursor.ID <= 0 {
		return nil, fmt.Errorf("page_token is invalid")
	}
	return cursor, nil
}

func NextPageToken(offset int, limit int, returned int) string {
	if returned < limit {
		return ""
	}
	next := offset + returned
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("offset:%d", next)))
}

func PageOffsetToken(offset int) string {
	if offset < 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("offset:%d", offset)))
}

// PaginateInMemory preserves the current offset-token API while isolating the
// compatibility shortcut from handlers. Replace this with store-backed cursor
// pagination when the library layer can provide stable database cursors.
func PaginateInMemory[T any](items []T, page Page) PageResult[T] {
	start := page.Offset
	if start > len(items) {
		start = len(items)
	}
	end := start + page.Limit
	if end > len(items) {
		end = len(items)
	}
	result := PageResult[T]{Items: items[start:end]}
	if end < len(items) {
		result.NextPageToken = NextPageToken(page.Offset, page.Limit, len(result.Items))
	}
	return result
}
