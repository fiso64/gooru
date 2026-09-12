package serve

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

var (
	initializedTestDBTemplatesMu sync.Mutex
	initializedTestDBTemplates   = map[types.HashingStrategy][]byte{}
)

func writeInitializedTestDB(t *testing.T, dbPath string, strategy types.HashingStrategy) {
	t.Helper()
	data := initializedTestDBTemplate(t, strategy)
	if err := os.WriteFile(dbPath, data, 0600); err != nil {
		t.Fatalf("write initialized test database: %v", err)
	}
}

func initializedTestDBTemplate(t *testing.T, strategy types.HashingStrategy) []byte {
	t.Helper()
	initializedTestDBTemplatesMu.Lock()
	defer initializedTestDBTemplatesMu.Unlock()
	if data, ok := initializedTestDBTemplates[strategy]; ok {
		return data
	}

	templateDir, err := os.MkdirTemp("", "gooru-serve-test-db-template-")
	if err != nil {
		t.Fatalf("create test database template dir: %v", err)
	}
	defer os.RemoveAll(templateDir)
	templatePath := filepath.Join(templateDir, "template.db")
	if err := core.Init(templatePath, strategy, false); err != nil {
		t.Fatalf("initialize test database template: %v", err)
	}
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read test database template: %v", err)
	}
	initializedTestDBTemplates[strategy] = data
	return data
}
