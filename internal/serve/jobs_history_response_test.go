package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestJobHistoryOmitsResultWhileDetailRetainsIt(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	auth := attachTestAuth(t, server)
	wantResult := strings.Repeat("job-result-payload", 1024)
	job, err := server.jobs.Submit(context.Background(), "upload_import", true, func(context.Context) (interface{}, error) {
		return wantResult, nil
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	waitForStatus(t, server.jobs, job.ID, JobCompleted)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?limit=20", nil)
	addAuthCookie(listReq, cfg, auth)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listRec.Code, listRec.Body.String())
	}
	var list JobListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != job.ID {
		t.Fatalf("unexpected list: %+v", list)
	}
	if list.Items[0].Result != nil {
		t.Fatalf("history result = %#v, want nil", list.Items[0].Result)
	}
	if strings.Contains(listRec.Body.String(), wantResult) {
		t.Fatal("history response serialized retained result payload")
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+job.ID, nil)
	addAuthCookie(detailReq, cfg, auth)
	detailRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d: %s", detailRec.Code, detailRec.Body.String())
	}
	var detail Job
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Result != wantResult {
		t.Fatalf("detail result = %#v, want retained payload", detail.Result)
	}
}
