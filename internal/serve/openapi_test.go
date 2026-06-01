package serve

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIDocumentsCurrentDTOFields(t *testing.T) {
	data, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI spec: %v", err)
	}
	var spec map[string]interface{}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("OpenAPI spec must be valid YAML: %v", err)
	}

	fileSchema := schema(t, spec, "File")
	required := stringSlice(t, fileSchema["required"])
	for _, field := range []string{"id", "content_id", "safe_display_path", "media_kind", "metadata", "media_urls"} {
		if !containsString(required, field) {
			t.Fatalf("File schema required fields missing %q in %+v", field, required)
		}
	}
	fileProps := stringMap(t, fileSchema["properties"])
	if _, ok := fileProps["safe_display_path"]; !ok {
		t.Fatal("File schema missing safe_display_path")
	}
	assertRef(t, stringMap(t, fileProps["metadata"]), "#/components/schemas/MediaMetadata")
	assertRef(t, stringMap(t, fileProps["media_urls"]), "#/components/schemas/MediaURLs")

	metadataProps := stringMap(t, schema(t, spec, "MediaMetadata")["properties"])
	for _, field := range []string{"image_width", "image_height", "video_width", "video_height", "video_duration", "audio_duration", "frame_count"} {
		if _, ok := metadataProps[field]; !ok {
			t.Fatalf("MediaMetadata schema missing %q", field)
		}
	}
	mediaURLRequired := stringSlice(t, schema(t, spec, "MediaURLs")["required"])
	if !containsString(mediaURLRequired, "download") {
		t.Fatalf("MediaURLs schema must require download, got %+v", mediaURLRequired)
	}
	listRequired := stringSlice(t, schema(t, spec, "FileListResponse")["required"])
	for _, field := range []string{"total_count", "library_count"} {
		if !containsString(listRequired, field) {
			t.Fatalf("FileListResponse schema missing required %q in %+v", field, listRequired)
		}
	}

	jobSchema := schema(t, spec, "Job")
	jobRequired := stringSlice(t, jobSchema["required"])
	if !containsString(jobRequired, "submitted_at") {
		t.Fatalf("Job schema must require submitted_at, got %+v", jobRequired)
	}
	jobProps := stringMap(t, jobSchema["properties"])
	for _, field := range []string{"started_at", "finished_at", "result", "error"} {
		if _, ok := jobProps[field]; !ok {
			t.Fatalf("Job schema missing %q", field)
		}
	}

	assertResponseRef(t, spec, "/files/tags", "post", "503", "#/components/responses/ServiceUnavailable")
	assertResponseRef(t, spec, "/files/tags", "put", "503", "#/components/responses/ServiceUnavailable")
	assertResponseRef(t, spec, "/files/tags", "delete", "503", "#/components/responses/ServiceUnavailable")
	assertResponseRef(t, spec, "/uploads", "post", "503", "#/components/responses/ServiceUnavailable")
	assertResponseSchemaRef(t, spec, "/upload-targets", "get", "200", "#/components/schemas/UploadTargetsResponse")
	assertResponseSchemaRef(t, spec, "/search/suggestions", "get", "200", "#/components/schemas/SuggestionsResponse")
	assertResponseSchemaRef(t, spec, "/tags/namespaces", "get", "200", "#/components/schemas/NamespacesResponse")
	assertResponseSchemaRef(t, spec, "/saved-searches", "get", "200", "#/components/schemas/SavedSearchesResponse")
	assertResponseSchemaRef(t, spec, "/saved-searches", "post", "201", "#/components/schemas/SavedSearch")
	assertResponseSchemaRef(t, spec, "/saved-searches/{id}", "put", "200", "#/components/schemas/SavedSearch")
	assertResponseSchemaRef(t, spec, "/files/{id}", "delete", "200", "#/components/schemas/DeleteFileResponse")
	assertResponseSchemaRef(t, spec, "/jobs", "get", "200", "#/components/schemas/JobListResponse")

	uploadFileProps := stringMap(t, stringMap(t, schema(t, spec, "UploadImportResponse")["properties"])["files"])
	uploadFileItems := stringMap(t, uploadFileProps["items"])
	uploadFileRequired := stringSlice(t, uploadFileItems["required"])
	for _, field := range []string{"target_id", "status"} {
		if !containsString(uploadFileRequired, field) {
			t.Fatalf("UploadImportResponse file item required fields missing %q in %+v", field, uploadFileRequired)
		}
	}
}

func schema(t *testing.T, spec map[string]interface{}, name string) map[string]interface{} {
	t.Helper()
	components := stringMap(t, spec["components"])
	schemas := stringMap(t, components["schemas"])
	return stringMap(t, schemas[name])
}

func assertResponseRef(t *testing.T, spec map[string]interface{}, path string, method string, status string, want string) {
	t.Helper()
	paths := stringMap(t, spec["paths"])
	pathItem := stringMap(t, paths[path])
	operation := stringMap(t, pathItem[method])
	responses := stringMap(t, operation["responses"])
	assertRef(t, stringMap(t, responses[status]), want)
}

func assertResponseSchemaRef(t *testing.T, spec map[string]interface{}, path string, method string, status string, want string) {
	t.Helper()
	paths := stringMap(t, spec["paths"])
	pathItem := stringMap(t, paths[path])
	operation := stringMap(t, pathItem[method])
	responses := stringMap(t, operation["responses"])
	response := stringMap(t, responses[status])
	content := stringMap(t, response["content"])
	jsonContent := stringMap(t, content["application/json"])
	assertRef(t, stringMap(t, jsonContent["schema"]), want)
}

func assertRef(t *testing.T, node map[string]interface{}, want string) {
	t.Helper()
	if got, _ := node["$ref"].(string); got != want {
		t.Fatalf("expected ref %q, got %q in %+v", want, got, node)
	}
}

func stringMap(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	out, ok := value.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T: %#v", value, value)
	}
	return out
}

func stringSlice(t *testing.T, value interface{}) []string {
	t.Helper()
	raw, ok := value.([]interface{})
	if !ok {
		t.Fatalf("expected list, got %T: %#v", value, value)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("expected string list item, got %T: %#v", item, item)
		}
		out = append(out, text)
	}
	return out
}
