package gooru

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// PublicFileID returns the public API identifier for a tracked file location.
func (c *Client) PublicFileID(locationID int64) string {
	return encodePublicFileID(locationID)
}

// ResolvePublicFileID resolves a public API file identifier to an internal location ID.
func (c *Client) ResolvePublicFileID(id string) (int64, error) {
	return decodePublicFileID(id)
}

func encodePublicFileID(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("loc:%d", id)))
}

func decodePublicFileID(encoded string) (int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, err
	}
	value := string(raw)
	if !strings.HasPrefix(value, "loc:") {
		return 0, fmt.Errorf("invalid file id")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, "loc:"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid file id")
	}
	return id, nil
}
