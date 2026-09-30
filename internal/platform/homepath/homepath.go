package homepath

import "strings"

// Expand replaces a leading "~/" in path with home; any other path
// (absolute, relative, or a bare "~") is returned unchanged.
func Expand(home, path string) string {
	if strings.HasPrefix(path, "~/") {
		return trimSlashes(home) + strings.TrimPrefix(path, "~")
	}
	return path
}

// Abbreviate replaces a leading home prefix in path with "~"; path is
// returned unchanged when it is not home itself or a path under it.
func Abbreviate(home, path string) string {
	home = trimSlashes(home)
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// trimSlashes drops the trailing slashes a $HOME may carry, keeping a home of
// only slashes (the root) as it is.
func trimSlashes(home string) string {
	if trimmed := strings.TrimRight(home, "/"); trimmed != "" {
		return trimmed
	}
	return home
}
