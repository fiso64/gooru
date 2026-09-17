package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestParseRegistrationSort(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  core.FileRegistrationSort
	}{
		{input: "newest-last", want: core.FileRegistrationSortNewestLast},
		{input: "newest_last", want: core.FileRegistrationSortNewestLast},
		{input: "queue", want: core.FileRegistrationSortNewestLast},
		{input: "newest-first", want: core.FileRegistrationSortNewestFirst},
		{input: "newest_first", want: core.FileRegistrationSortNewestFirst},
		{input: "reverse_queue", want: core.FileRegistrationSortNewestFirst},
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

func TestAddCommandModTimeSortPersistsSourceModificationTime(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := core.Init(dbPath, types.StrategyPartial, false); err != nil {
		t.Fatalf("init database: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}

	previousSvc, previousSort, previousUseMetadata := svc, addRegistrationSort, addUseMetadata
	svc, addRegistrationSort, addUseMetadata = client, "modtime", false
	t.Cleanup(func() {
		_ = client.Close()
		svc, addRegistrationSort, addUseMetadata = previousSvc, previousSort, previousUseMetadata
	})

	path := filepath.Join(dir, "cli-added.jpg")
	if err := os.WriteFile(path, []byte("cli add sort"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	modtime := time.Date(2022, 4, 5, 6, 7, 8, 321_000_000, time.UTC)
	if err := os.Chtimes(path, modtime, modtime); err != nil {
		t.Fatalf("set source modtime: %v", err)
	}

	if err := addCmd.RunE(addCmd, []string{path}); err != nil {
		t.Fatalf("add command: %v", err)
	}
	files, err := client.GetAllFilesInfo()
	if err != nil {
		t.Fatalf("list tracked files: %v", err)
	}
	for _, file := range files {
		if file.Path == path {
			if file.AddedAt != modtime.UnixMilli() {
				t.Fatalf("added_at=%d want source modtime %d", file.AddedAt, modtime.UnixMilli())
			}
			return
		}
	}
	t.Fatalf("tracked file %q not found", path)
}
