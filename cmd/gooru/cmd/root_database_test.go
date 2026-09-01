package cmd

import "testing"

func TestConfiguredDatabasePathUsesExplicitOverride(t *testing.T) {
	previous := databasePath
	databasePath = "/tmp/gooru-instance.db"
	t.Cleanup(func() { databasePath = previous })

	got, err := configuredDatabasePath()
	if err != nil {
		t.Fatal(err)
	}
	if got != databasePath {
		t.Fatalf("configuredDatabasePath() = %q, want %q", got, databasePath)
	}
}
