package serve

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIBulkDownloadMethods(t *testing.T) {
	data, err := os.ReadFile("../../docs/openapi.yaml")
	require.NoError(t, err)

	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(data, &spec))

	create, ok := spec.Paths["/file-downloads"]
	require.True(t, ok, "missing bulk download creation path")
	require.Contains(t, create, "post")
	require.NotContains(t, create, "get")

	stream, ok := spec.Paths["/file-downloads/{id}"]
	require.True(t, ok, "missing bulk download stream path")
	require.Contains(t, stream, "get")
	require.NotContains(t, stream, "post")
}

func TestOpenAPIOriginalMediaMethods(t *testing.T) {
	data, err := os.ReadFile("../../docs/openapi.yaml")
	require.NoError(t, err)

	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(data, &spec))

	for _, path := range []string{"/files/{id}/content", "/files/{id}/download"} {
		methods, ok := spec.Paths[path]
		require.True(t, ok, "missing OpenAPI path %s", path)
		require.Contains(t, methods, "get", "GET must remain documented for %s", path)
		require.Contains(t, methods, "head", "HEAD must be documented for %s", path)
	}

	for _, path := range []string{"/files/{id}/thumbnail", "/files/{id}/preview"} {
		methods, ok := spec.Paths[path]
		require.True(t, ok, "missing OpenAPI path %s", path)
		require.NotContains(t, methods, "head", "derivative route %s must remain GET-only", path)
	}
}
