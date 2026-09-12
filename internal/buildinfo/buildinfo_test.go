package buildinfo

import "testing"

func TestCurrentUsesInjectedIdentity(t *testing.T) {
	oldVersion, oldRevision, oldDirty, oldDevelopment := Version, Revision, Dirty, Development
	t.Cleanup(func() { Version, Revision, Dirty, Development = oldVersion, oldRevision, oldDirty, oldDevelopment })
	Version, Revision, Dirty, Development = "1.2.3", "0123456789abcdef", "false", "false"
	got := Current()
	if got.Version != "1.2.3" || got.Revision != "0123456789abcdef" || got.Dirty || got.Development {
		t.Fatalf("unexpected build info: %+v", got)
	}
	if ShortRevision(got.Revision) != "0123456789ab" {
		t.Fatalf("unexpected short revision: %q", ShortRevision(got.Revision))
	}
}

func TestCurrentCanMarkCanonicalVersionAsDevelopment(t *testing.T) {
	oldVersion, oldRevision, oldDirty, oldDevelopment := Version, Revision, Dirty, Development
	t.Cleanup(func() { Version, Revision, Dirty, Development = oldVersion, oldRevision, oldDirty, oldDevelopment })
	Version, Revision, Dirty, Development = "1.2.3", "0123456789abcdef", "false", "true"
	got := Current()
	if !got.Development || got.Dirty {
		t.Fatalf("expected clean development identity, got %+v", got)
	}
}
