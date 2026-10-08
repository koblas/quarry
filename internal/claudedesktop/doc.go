// Package claudedesktop adds quarry's MCP server to Claude Desktop by editing
// the mcpServers.quarry entry of Desktop's claude_desktop_config.json.
//
// The home directory and the path of the running quarry binary arrive as
// options, so every decision here is testable against a temporary folder.
package claudedesktop
