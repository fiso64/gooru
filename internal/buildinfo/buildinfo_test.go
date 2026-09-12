package buildinfo

import "testing"

func TestCurrentUsesInjectedIdentity(t *testing.T) {
	oldVersion, oldRevision, oldDirty := Version, Revision, Dirty
	t.Cleanup(func() { Version, Revision, Dirty = oldVersion, oldRevision, oldDirty })
	Version, Revision, Dirty = "1.2.3", "0123456789abcdef", "false"
	got := Current()
	if got.Version != "1.2.3" || got.Revision != "0123456789abcdef" || got.Dirty || got.Development {
		t.Fatalf("unexpected build info: %+v", got)
	}
	if ShortRevision(got.Revision) != "0123456789ab" {
		t.Fatalf("unexpected short revision: %q", ShortRevision(got.Revision))
	}
}
