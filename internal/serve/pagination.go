package serve

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	DefaultPageLimit = 50
	MaxPageLimit     = 200
)

type Page struct {
	Limit  int
	Offset int
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
		if len(parts) != 2 || parts[0] != "offset" {
			return Page{}, fmt.Errorf("page_token is invalid")
		}
		parsed, err := strconv.Atoi(parts[1])
		if err != nil || parsed < 0 {
			return Page{}, fmt.Errorf("page_token is invalid")
		}
		offset = parsed
	}
	return Page{Limit: limit, Offset: offset}, nil
}

func NextPageToken(offset int, limit int, returned int) string {
	if returned < limit {
		return ""
	}
	next := offset + returned
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("offset:%d", next)))
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
