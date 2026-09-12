package serve

import (
	"encoding/json"
	"gooru.local/internal/buildinfo"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildInfoEndpoint(t *testing.T) {
	oldVersion, oldRevision, oldDirty, oldDevelopment := buildinfo.Version, buildinfo.Revision, buildinfo.Dirty, buildinfo.Development
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Revision, buildinfo.Dirty, buildinfo.Development = oldVersion, oldRevision, oldDirty, oldDevelopment
	})
	buildinfo.Version, buildinfo.Revision, buildinfo.Dirty, buildinfo.Development = "1.2.3", "0123456789abcdef", "false", "false"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/build", nil)
	rec := httptest.NewRecorder()
	NewServer(Config{}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got buildinfo.Info
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.2.3" || got.Revision != "0123456789abcdef" || got.Dirty || got.Development {
		t.Fatalf("unexpected build info: %+v", got)
	}
}
