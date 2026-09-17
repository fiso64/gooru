package cmd

import (
	"testing"

	core "gooru.local/gooru"
)

func TestParseRegistrationSort(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  core.FileRegistrationSort
	}{
		{input: "queue", want: core.FileRegistrationSortQueue},
		{input: "reverse", want: core.FileRegistrationSortReverseQueue},
		{input: "reverse-queue", want: core.FileRegistrationSortReverseQueue},
		{input: "reverse_queue", want: core.FileRegistrationSortReverseQueue},
		{input: "modtime", want: core.FileRegistrationSortModTime},
		{input: "mtime", want: core.FileRegistrationSortModTime},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseRegistrationSort(tc.input)
			if err != nil {
				t.Fatalf("parseRegistrationSort(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("parseRegistrationSort(%q)=%q want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseRegistrationSortRejectsUnknownValue(t *testing.T) {
	if _, err := parseRegistrationSort("random"); err == nil {
		t.Fatal("expected invalid --sort error")
	}
}
