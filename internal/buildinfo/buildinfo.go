package buildinfo

import (
	"runtime/debug"
	"strings"
)

// Version, Revision, and Dirty are intended to be populated with -ldflags for
// release/package builds. Their defaults describe an ordinary development
// build without pretending it is an official release.
var (
	Version  = "0.1.0-dev"
	Revision = "unknown"
	Dirty    = "true"
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
		version = "0.1.0-dev"
	}
	revision := strings.TrimSpace(Revision)
	dirty := strings.EqualFold(strings.TrimSpace(Dirty), "true")

	if bi, ok := debug.ReadBuildInfo(); ok {
		if revision == "" || revision == "unknown" {
			for _, setting := range bi.Settings {
				if setting.Key == "vcs.revision" && setting.Value != "" {
					revision = setting.Value
				}
			}
		}
		if strings.TrimSpace(Dirty) == "" {
			for _, setting := range bi.Settings {
				if setting.Key == "vcs.modified" {
					dirty = setting.Value == "true"
				}
			}
		}
	}
	if revision == "" {
		revision = "unknown"
	}

	return Info{
		Version:     version,
		Revision:    revision,
		Dirty:       dirty,
		Development: strings.Contains(version, "dev") || dirty || revision == "unknown",
	}
}

func ShortRevision(revision string) string {
	revision = strings.TrimSpace(revision)
	if len(revision) <= 12 {
		return revision
	}
	return revision[:12]
}
