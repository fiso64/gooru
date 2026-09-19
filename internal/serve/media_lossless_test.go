package serve

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gooru.local/types"
)

const progressiveJPEGFixtureBase64 = "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJCQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjL/wgARCAAMAAwDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAX/xAAVAQEBAAAAAAAAAAAAAAAAAAAEBf/aAAwDAQACEAMQAAABmimv/8QAFBABAAAAAAAAAAAAAAAAAAAAIP/aAAgBAQABBQIf/8QAFBEBAAAAAAAAAAAAAAAAAAAAAP/aAAgBAwEBPwF//8QAFBEBAAAAAAAAAAAAAAAAAAAAAP/aAAgBAgEBPwF//8QAFBABAAAAAAAAAAAAAAAAAAAAIP/aAAgBAQAGPwIf/8QAFBABAAAAAAAAAAAAAAAAAAAAIP/aAAgBAQABPyEf/9oADAMBAAIAAwAAABD7/8QAFBEBAAAAAAAAAAAAAAAAAAAAAP/aAAgBAwEBPxB//8QAFBEBAAAAAAAAAAAAAAAAAAAAAP/aAAgBAgEBPxB//8QAFBABAAAAAAAAAAAAAAAAAAAAIP/aAAgBAQABPxAf/9k="

func progressiveJPEGFixture(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(progressiveJPEGFixtureBase64)
	if err != nil { t.Fatal(err) }
	return b
}

func TestLosslessJPEGRouteAndMediaURLs(t *testing.T) {
	progressive := progressiveJPEGFixture(t)
	path := writeNamedMediaFile(t, "progressive.jpg", progressive)
	file := types.FileInfo{ID: 931, Path: path, Hash: "jpeg-lossless-test", Size: int64(len(progressive))}
	server := newMediaTestServer(t, file)
	server.media.losslessJPEGTool = writeJPEGTranscodeStub(t, "cat")
	id := fallbackPublicFileID(file.ID)
	var dto FileDTO
	dtoRecord := httptest.NewRecorder()
	server.Handler().ServeHTTP(dtoRecord, authedRequest(http.MethodGet, "/api/v1/files/"+id))
	if dtoRecord.Code != http.StatusOK { t.Fatalf("file DTO status %d: %s", dtoRecord.Code, dtoRecord.Body.String()) }
	if err := json.Unmarshal(dtoRecord.Body.Bytes(), &dto); err != nil { t.Fatal(err) }
	if dto.MediaURLs.Lossless != "/api/v1/files/"+id+"/lossless" { t.Fatalf("lossless URL missing: %+v", dto.MediaURLs) }
	for i, want := range []string{"miss", "hit"} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, dto.MediaURLs.Lossless))
		if rec.Code != http.StatusOK { t.Fatalf("lossless request %d: %d: %s", i, rec.Code, rec.Body.String()) }
		if got := rec.Header().Get("X-Gooru-Cache"); got != want { t.Fatalf("cache status %q, want %q", got, want) }
		if !bytes.Equal(rec.Body.Bytes(), progressive) { t.Fatal("stub converter output differs from source") }
	}
	for _, route := range []string{"content", "download"} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/files/"+id+"/"+route))
		if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), progressive) {
			t.Fatalf("%s changed the original: status=%d", route, rec.Code)
		}
	}
	server.cfg.Media.LosslessJPEGTranscode = false
	server.media.cfg.Media.LosslessJPEGTranscode = false
	if server.fileDTO(nil, file, false).MediaURLs.Lossless != "" { t.Fatal("disabled derivative still advertised") }
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/files/"+id+"/lossless"))
	if rec.Code != http.StatusNotFound { t.Fatalf("disabled derivative status %d", rec.Code) }
}

func TestLosslessJPEGUnavailableWhenNotProgressiveOrNoTool(t *testing.T) {
	progressive := progressiveJPEGFixture(t)
	path := writeNamedMediaFile(t, "progressive.jpg", progressive)
	file := types.FileInfo{ID: 932, Path: path, Hash: "jpeg-unavailable", Size: int64(len(progressive))}
	server := newMediaTestServer(t, file)
	if server.media.losslessJPEGAvailable(file) && server.media.losslessJPEGTool == "" {
		t.Fatal("advertised derivative without converter")
	}
	server.media.losslessJPEGTool = writeJPEGTranscodeStub(t, "cat")
	server.media.losslessJPEGTool = ""
	if server.media.losslessJPEGAvailable(file) { t.Fatal("missing converter should disable derivative") }
}
