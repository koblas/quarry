// Package homepath expands and abbreviates a leading "~/" in a path against
// an injected home directory, so callers never consult the environment
// directly.
package homepath
