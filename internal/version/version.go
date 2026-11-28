package version

import (
	"runtime"
	"runtime/debug"
)

const (
	// devel is the version Go reports for a binary built without a module version.
	devel = "(devel)"
	// shortRevisionLen is how many characters of the VCS revision the version string shows.
	shortRevisionLen = 12

	revisionKey = "vcs.revision"
	modifiedKey = "vcs.modified"
)

// Info is what a build recorded about itself. Commit and Modified are nil
// when the build did not record them (or recorded them unreadably), never a
// guessed value.
type Info struct {
	Version  string
	Commit   *string
	Modified *bool
	Go       string
}

// Resolve derives Info from info, which may be nil. Version is the module
// version, else "(devel)" with the short revision and "+dirty" when recorded.
// Commit and Modified are nil unless the build recorded them readably.
func Resolve(info *debug.BuildInfo) Info {
	out := Info{Version: devel, Go: runtime.Version()}
	if info == nil {
		return out
	}

	if rev, ok := setting(info.Settings, revisionKey); ok && rev != "" {
		out.Commit = &rev
	}
	if modified, ok := setting(info.Settings, modifiedKey); ok {
		switch modified {
		case "true":
			out.Modified = new(true)
		case "false":
			out.Modified = new(false)
		}
	}

	if info.Main.Version != "" && info.Main.Version != devel {
		out.Version = info.Main.Version
		return out
	}
	if out.Commit != nil {
		out.Version = devel + " " + shortRevision(*out.Commit)
		if out.Modified != nil && *out.Modified {
			out.Version += "+dirty"
		}
	}
	return out
}

// shortRevision is revision cut to shortRevisionLen characters, whole when shorter.
func shortRevision(revision string) string {
	if len(revision) <= shortRevisionLen {
		return revision
	}
	return revision[:shortRevisionLen]
}

// setting is the value settings records for key; false when absent or recorded
// with differing values.
func setting(settings []debug.BuildSetting, key string) (string, bool) {
	var value string
	found := false
	for _, s := range settings {
		if s.Key != key {
			continue
		}
		if found && s.Value != value {
			return "", false
		}
		value, found = s.Value, true
	}
	return value, found
}
