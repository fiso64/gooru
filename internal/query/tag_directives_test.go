package query

import (
	"reflect"
	"testing"
)

func TestResolveTagDirectives(t *testing.T) {
	got, err := ResolveTagDirectives([]string{"project:inbox", "source:upload", "project:archive", "-project:archive", "project:inbox"})
	if err != nil {
		t.Fatalf("ResolveTagDirectives: %v", err)
	}
	want := []string{"project:inbox", "source:upload"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved tags=%v want %v", got, want)
	}
}

func TestResolveTagDirectivesAllowsLaterReAdd(t *testing.T) {
	got, err := ResolveTagDirectives([]string{"tag", "-tag", "tag"})
	if err != nil || !reflect.DeepEqual(got, []string{"tag"}) {
		t.Fatalf("resolved tags=%v err=%v", got, err)
	}
}

func TestValidateTagDirectiveRejectsInvalidExclusion(t *testing.T) {
	if err := ValidateTagDirective("-"); err == nil {
		t.Fatal("expected empty exclusion error")
	}
}
