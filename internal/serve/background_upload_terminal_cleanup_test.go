package serve

import "testing"

func TestBackgroundUploadTaskCarriesTerminalCleanupDescriptor(t *testing.T) {
	request, err := backgroundUploadTaskRequest("operation-1", []savedUpload{{
		name:     "photo.jpg",
		path:     "/uploads/.gooru-upload-staging/operation-1/photo.jpg",
		targetID: "default",
	}}, nil)
	if err != nil {
		t.Fatalf("backgroundUploadTaskRequest: %v", err)
	}

	cleanup := request.TerminalFailureCleanup
	if cleanup == nil {
		t.Fatal("upload task has no terminal failure cleanup")
	}
	if cleanup.DedupeKey != "upload-cleanup:operation-1" || cleanup.Kind != backgroundUploadCleanupTaskKind {
		t.Fatalf("cleanup identity = dedupe %q kind %q", cleanup.DedupeKey, cleanup.Kind)
	}
	if cleanup.SubjectKind != "operation" || cleanup.SubjectID != "operation-1" || cleanup.InputKey != "" {
		t.Fatalf("cleanup subject = %q/%q input %q", cleanup.SubjectKind, cleanup.SubjectID, cleanup.InputKey)
	}
	if cleanup.ResourceClass != backgroundUploadResourceClass || cleanup.Priority != backgroundUploadCleanupPriority || cleanup.MaxAttempts != 5 {
		t.Fatalf("cleanup scheduling = resource %q priority %d attempts %d", cleanup.ResourceClass, cleanup.Priority, cleanup.MaxAttempts)
	}
}
