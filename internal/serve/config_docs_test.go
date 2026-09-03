package serve

import (
	"os"
	"strings"
	"testing"

	"gooru.local/internal/securekey"
)

func TestConfigReferenceDocumentsEncryptionEnablement(t *testing.T) {
	doc, err := os.ReadFile("../../docs/CONFIG.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(doc)
	for _, want := range []string{
		"## `encryption`",
		"`encryption.enabled`",
		"`encryption.key_file`",
		securekey.EnvKey,
		securekey.EnvKeyFile,
		"encryption:\n  enabled: false",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("docs/CONFIG.md is missing encryption configuration reference %q", want)
		}
	}
}
