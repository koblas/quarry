// Package claudedesktop adds quarry's MCP server to Claude Desktop, and removes
// it again, by editing the mcpServers.quarry entry of Desktop's
// claude_desktop_config.json.
//
// Install writes an entry that starts the quarry on PATH when that is the
// running binary, so a Homebrew link outlives upgrades, and otherwise the
// running binary; a binary in a temporary location, or one not named quarry,
// is refused. Uninstall removes mcpServers.quarry only when it starts
// `quarry mcp`, whatever the path, and leaves any other entry in place. Both
// save the original config beside it before replacing it. The home directory,
// the path of the running binary, the PATH lookup and the temporary root arrive
// as options, so every decision here is testable against a temporary folder.
package claudedesktop
