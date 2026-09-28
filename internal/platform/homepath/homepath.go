package homepath

import "strings"

// Expand replaces a leading "~/" in path with home; any other path
// (absolute, relative, or a bare "~") is returned unchanged.
func Expand(home, path string) string {
	if strings.HasPrefix(path, "~/") {
		return home + strings.TrimPrefix(path, "~")
	}
	return path
}

// Abbreviate replaces a leading home prefix in path with "~"; path is
// returned unchanged when it is not home itself or a path under it.
func Abbreviate(home, path string) string {
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
