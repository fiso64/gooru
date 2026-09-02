package database

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestLogQueryRedactsBoundValues(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	logQuery(logger, "SELECT id FROM locations WHERE path = ? AND content_hash = ?", []interface{}{"/private/photos/girlfriend.jpg", "secret-hash"})

	got := output.String()
	if !strings.Contains(got, "SELECT id FROM locations WHERE path = ? AND content_hash = ?") {
		t.Fatalf("query shape missing from log: %q", got)
	}
	if !strings.Contains(got, "2 bound values redacted") {
		t.Fatalf("redaction count missing from log: %q", got)
	}
	for _, secret := range []string{"girlfriend.jpg", "secret-hash", "/private/photos"} {
		if strings.Contains(got, secret) {
			t.Fatalf("SQL log leaked %q: %q", secret, got)
		}
	}
}
