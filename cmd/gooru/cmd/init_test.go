package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gooru.local/internal/database"
	"gooru.local/types"
)

func TestEnsureDatabaseParentCreatesMissingDirectories(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "nested", "library", "gooru.db")
	parent := filepath.Dir(dbPath)

	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("expected parent to be absent before setup, err=%v", err)
	}

	if err := ensureDatabaseParent(dbPath); err != nil {
		t.Fatalf("create database parent: %v", err)
	}

	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("stat created parent: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("created parent is not a directory: mode=%v", info.Mode())
	}
}

func TestInitHashingStrategyRequiresExplicitStrategyForIfMissing(t *testing.T) {
	previous := initFlags
	defer func() { initFlags = previous }()
	initFlags.hashingStrategy = ""
	initFlags.ifMissing = true

	_, err := initHashingStrategy(&cobra.Command{})
	if err == nil || !strings.Contains(err.Error(), "--hashing-strategy is required") {
		t.Fatalf("initHashingStrategy() error = %v, want explicit strategy requirement", err)
	}
}

func TestInitHashingStrategyRejectsUnknownStrategy(t *testing.T) {
	previous := initFlags
	defer func() { initFlags = previous }()
	initFlags.hashingStrategy = "quick"
	initFlags.ifMissing = false

	_, err := initHashingStrategy(&cobra.Command{})
	if err == nil || !strings.Contains(err.Error(), "must be partial or full") {
		t.Fatalf("initHashingStrategy() error = %v, want invalid strategy error", err)
	}
}

func TestInitCommandAutomatedFirstBootGoldenPath(t *testing.T) {
	previousFlags := initFlags
	previousDatabasePath := databasePath
	previousConfigPath := configPath
	previousVerbose := verbose
	previousOut := initCmd.OutOrStdout()
	defer func() {
		initFlags = previousFlags
		databasePath = previousDatabasePath
		configPath = previousConfigPath
		verbose = previousVerbose
		initCmd.SetOut(previousOut)
	}()

	dbPath := filepath.Join(t.TempDir(), "state", "gooru.db")
	databasePath = dbPath
	configPath = ""
	verbose = false
	initFlags.hashingStrategy = "full"
	initFlags.ifMissing = true

	var output bytes.Buffer
	initCmd.SetOut(&output)
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("first automated init: %v", err)
	}
	if !strings.Contains(output.String(), "Database initialized successfully") {
		t.Fatalf("first automated init output = %q, want success message", output.String())
	}

	store, err := database.NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("open initialized database: %v", err)
	}
	strategy, err := store.GetHashingStrategy()
	if err != nil {
		_ = store.Close()
		t.Fatalf("read hashing strategy: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close initialized database: %v", err)
	}
	if strategy != types.StrategyFull {
		t.Fatalf("hashing strategy = %q, want %q", strategy, types.StrategyFull)
	}

	output.Reset()
	initFlags.hashingStrategy = "partial"
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("repeat automated init: %v", err)
	}
	if !strings.Contains(output.String(), "leaving it unchanged") {
		t.Fatalf("repeat automated init output = %q, want unchanged message", output.String())
	}

	store, err = database.NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("reopen initialized database: %v", err)
	}
	defer store.Close()
	strategy, err = store.GetHashingStrategy()
	if err != nil {
		t.Fatalf("read hashing strategy after repeat init: %v", err)
	}
	if strategy != types.StrategyFull {
		t.Fatalf("repeat init changed hashing strategy to %q, want %q", strategy, types.StrategyFull)
	}
}

func TestInitializeDatabaseIfNeededCreatesRequestedStrategy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "gooru.db")
	created, err := initializeDatabaseIfNeeded(dbPath, types.StrategyFull, true, false)
	if err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	if !created {
		t.Fatal("expected missing database to be created")
	}

	store, err := database.NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("open initialized database: %v", err)
	}
	defer store.Close()
	strategy, err := store.GetHashingStrategy()
	if err != nil {
		t.Fatalf("read hashing strategy: %v", err)
	}
	if strategy != types.StrategyFull {
		t.Fatalf("hashing strategy = %q, want %q", strategy, types.StrategyFull)
	}
}

func TestInitializeDatabaseIfNeededLeavesExistingStrategyAuthoritative(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	created, err := initializeDatabaseIfNeeded(dbPath, types.StrategyPartial, false, false)
	if err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	if !created {
		t.Fatal("expected initial database creation")
	}

	created, err = initializeDatabaseIfNeeded(dbPath, types.StrategyFull, true, false)
	if err != nil {
		t.Fatalf("idempotent initialization: %v", err)
	}
	if created {
		t.Fatal("existing database must not be recreated")
	}

	store, err := database.NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("open existing database: %v", err)
	}
	defer store.Close()
	strategy, err := store.GetHashingStrategy()
	if err != nil {
		t.Fatalf("read existing hashing strategy: %v", err)
	}
	if strategy != types.StrategyPartial {
		t.Fatalf("existing hashing strategy changed to %q, want %q", strategy, types.StrategyPartial)
	}
}
