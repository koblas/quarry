// Package claudeplugin installs and removes quarry's Claude Code plugin by
// running the claude command, never by editing Claude Code's own files.
//
// The claude command is reached through a Runner, so every decision here is
// testable without a subprocess.
package claudeplugin
