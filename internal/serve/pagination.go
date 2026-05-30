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
