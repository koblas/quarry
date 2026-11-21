package version

import "runtime/debug"

// Info is what a build recorded about itself. Commit and Modified are nil
// when the build did not record them, never a guessed value.
type Info struct {
	Version  string
	Commit   *string
	Modified *bool
	Go       string
}

// Resolve derives Info from info, which may be nil.
func Resolve(_ *debug.BuildInfo) Info {
	return Info{}
}
