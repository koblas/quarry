// Package claudedesktop adds quarry's MCP server to Claude Desktop by editing
// the mcpServers.quarry entry of Desktop's claude_desktop_config.json.
//
// The entry starts the quarry on PATH when that is the running binary, so a
// Homebrew link outlives upgrades, and otherwise the running binary; a binary
// in a temporary location is refused. The home directory, the path of the
// running binary, the PATH lookup and the temporary root arrive as options, so
// every decision here is testable against a temporary folder.
package claudedesktop
