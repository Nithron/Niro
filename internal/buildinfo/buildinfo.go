// Package buildinfo exposes the version stamped in at build time:
//
//	go build -ldflags "-X github.com/nithron/niro/internal/buildinfo.Version=v0.1.0"
package buildinfo

import "runtime/debug"

// Version is set via -ldflags; it falls back to the VCS revision.
var Version = ""

// String returns the version, or "dev-<short commit>" for untagged builds.
func String() string {
	if Version != "" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		rev, dirty := "", false
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if len(rev) > 7 {
			rev = rev[:7]
		}
		if rev != "" {
			if dirty {
				rev += "-dirty"
			}
			return "dev-" + rev
		}
	}
	return "dev"
}
