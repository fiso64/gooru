package serve

import (
	"fmt"
	"strconv"
	"strings"
)

func fallbackPublicFileID(id int64) string {
	return fmt.Sprintf("test_file_%d", id)
}

func fallbackResolveFileID(encoded string) (int64, error) {
	value := strings.TrimSpace(encoded)
	if !strings.HasPrefix(value, "test_file_") {
		return 0, fmt.Errorf("invalid file id")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, "test_file_"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid file id")
	}
	return id, nil
}
