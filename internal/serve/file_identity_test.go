package serve

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

func fallbackPublicFileID(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("loc:%d", id)))
}

func fallbackResolveFileID(encoded string) (int64, error) {
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
