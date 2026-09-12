package serve

import (
	"reflect"
	"testing"
)

func TestBackgroundUploadActivatedCheckpointReconstructsReplacementState(t *testing.T) {
	files := []savedUpload{
		{name: "plain.jpg", path: "/uploads/plain.jpg", targetID: "default"},
		{name: "replace.jpg", path: "/uploads/.replace.jpg.tmp-1", destinationPath: "/uploads/replace.jpg", targetID: "default", replace: true},
		{name: "new.jpg", path: "/uploads/.new.jpg.tmp-2", destinationPath: "/uploads/new.jpg", targetID: "default", replace: true},
	}
	activated := []activatedSavedReplacement{
		{index: 1, replacement: activatedReplacement{finalPath: files[1].destinationPath, backupPath: files[1].path + ".backup", noOriginalMarkerPath: files[1].path + ".no-original", hadOriginal: true}},
		{index: 2, replacement: activatedReplacement{finalPath: files[2].destinationPath, backupPath: files[2].path + ".backup", noOriginalMarkerPath: files[2].path + ".no-original", hadOriginal: false}},
	}

	checkpoint := backgroundUploadActivatedCheckpoint(activated)
	if checkpoint.Phase != backgroundUploadPhaseActivated {
		t.Fatalf("phase = %q, want %q", checkpoint.Phase, backgroundUploadPhaseActivated)
	}
	checkpoint.Replacements[0], checkpoint.Replacements[1] = checkpoint.Replacements[1], checkpoint.Replacements[0]
	got, err := activatedSavedReplacementsFromCheckpoint(files, checkpoint)
	if err != nil {
		t.Fatalf("activatedSavedReplacementsFromCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(got, activated) {
		t.Fatalf("reconstructed replacements = %#v, want task activation order %#v", got, activated)
	}
}

func TestBackgroundUploadImportedCheckpointPersistsResponse(t *testing.T) {
	files := []savedUpload{{name: "replace.jpg", path: "/uploads/.replace.jpg.tmp-1", destinationPath: "/uploads/replace.jpg", targetID: "default", replace: true}}
	activated := []activatedSavedReplacement{{index: 0, replacement: activatedReplacement{finalPath: files[0].destinationPath, backupPath: files[0].path + ".backup", noOriginalMarkerPath: files[0].path + ".no-original", hadOriginal: true}}}
	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "replace.jpg", TargetID: "default", Status: "imported"}}, AffectedCount: 1}

	checkpoint := backgroundUploadImportedCheckpoint(activated, response)
	if checkpoint.Phase != backgroundUploadPhaseImported {
		t.Fatalf("phase = %q, want %q", checkpoint.Phase, backgroundUploadPhaseImported)
	}
	if checkpoint.Response == nil || !reflect.DeepEqual(*checkpoint.Response, response) {
		t.Fatalf("response = %#v, want %#v", checkpoint.Response, response)
	}
	got, err := activatedSavedReplacementsFromCheckpoint(files, checkpoint)
	if err != nil {
		t.Fatalf("activatedSavedReplacementsFromCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(got, activated) {
		t.Fatalf("reconstructed replacements = %#v, want %#v", got, activated)
	}
}

func TestBackgroundUploadCheckpointRejectsMismatchedReplacementState(t *testing.T) {
	files := []savedUpload{
		{name: "replace.jpg", path: "/uploads/.replace.jpg.tmp-1", destinationPath: "/uploads/replace.jpg", targetID: "default", replace: true},
		{name: "plain.jpg", path: "/uploads/plain.jpg", targetID: "default"},
	}
	cases := []backgroundUploadCheckpoint{
		{Phase: backgroundUploadPhaseStaged},
		{Phase: backgroundUploadPhaseActivated},
		{Phase: backgroundUploadPhaseActivated, Replacements: []backgroundUploadReplacementCheckpoint{{Index: -1}}},
		{Phase: backgroundUploadPhaseActivated, Replacements: []backgroundUploadReplacementCheckpoint{{Index: 1}}},
		{Phase: backgroundUploadPhaseActivated, Replacements: []backgroundUploadReplacementCheckpoint{{Index: 0}, {Index: 0}}},
	}
	for index, checkpoint := range cases {
		if _, err := activatedSavedReplacementsFromCheckpoint(files, checkpoint); err == nil {
			t.Fatalf("case %d unexpectedly accepted checkpoint %#v", index, checkpoint)
		}
	}
}
