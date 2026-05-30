package serve

import (
	"os"
	"strings"
	"testing"
)

func TestOpenAPIDocumentsCurrentDTOFields(t *testing.T) {
	data, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI spec: %v", err)
	}
	spec := string(data)
	for _, field := range []string{
		"MediaMetadata:",
		"image_width:",
		"video_duration:",
		"submitted_at:",
		"finished_at:",
		"ServiceUnavailable:",
	} {
		if !strings.Contains(spec, field) {
			t.Fatalf("OpenAPI spec is missing %s", field)
		}
	}
}
