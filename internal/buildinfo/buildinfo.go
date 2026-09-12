package buildinfo

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Version, Revision, and Dirty are overridden with -ldflags by release and
// package builds. Source builds intentionally identify themselves as dev.
var (
	Version  = "0.0.0-dev"
	Revision = ""
	Dirty    = ""
)

type Info struct {
	Version     string `json:"version"`
	Revision    string `json:"revision"`
	Dirty       bool   `json:"dirty"`
	Development bool   `json:"development"`
}

func Current() Info {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = "0.0.0-dev"
	}
	revision := strings.TrimSpace(Revision)
	dirtyText := strings.TrimSpace(Dirty)
	dirty := strings.EqualFold(dirtyText, "true")
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range bi.Settings {
			switch setting.Key {
			case "vcs.revision":
				if revision == "" && setting.Value != "" {
					revision = setting.Value
				}
			case "vcs.modified":
				if dirtyText == "" {
					dirty = setting.Value == "true"
				}
			}
		}
	}
	if revision == "" {
		revision = "unknown"
	}
	return Info{Version: version, Revision: revision, Dirty: dirty, Development: strings.Contains(version, "dev") || dirty || revision == "unknown"}
}

func ShortRevision(revision string) string {
	revision = strings.TrimSpace(revision)
	if len(revision) <= 12 {
		return revision
	}
	return revision[:12]
}

func Summary() string {
	info := Current()
	state := ""
	if info.Dirty {
		state = ", dirty"
	} else if info.Development {
		state = ", development"
	}
	return fmt.Sprintf("gooru v%s (revision %s%s)", info.Version, ShortRevision(info.Revision), state)
}
