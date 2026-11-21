// Package version resolves the version a quarry binary reports from the build
// information the Go toolchain embedded in it.
//
// Resolve is the one place the rule lives: the command line, the MCP server
// and the store all take their version string from it.
package version
