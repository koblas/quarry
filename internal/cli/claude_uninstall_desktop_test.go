package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	desktopRemovedLine    = "Removed the quarry MCP server from Claude Desktop.\n"
	desktopQuitUnloadLine = "Quit and reopen Claude Desktop to unload it.\n"

	quarryEntry = `"quarry": {"command": "` + desktopQuarryBinary + `", "args": ["mcp"]}`
)

func Test_claude_uninstall_removes_quarry_from_claude_desktop_after_claude_code(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		want     string
	}{
		{
			name: "beside other servers",
			existing: `{
  "globalShortcut": "Cmd+Shift+Space",
  "bigNumber": 12345678901234567890,
  "mcpServers": {
    "github": {"command": "npx", "args": ["-y", "server-github"], "env": {"ID": 12345678901234567890}},
    ` + quarryEntry + `
  }
}
`,
			want: `{
  "globalShortcut": "Cmd+Shift+Space",
  "bigNumber": 12345678901234567890,
  "mcpServers": {
    "github": {"command": "npx", "args": ["-y", "server-github"], "env": {"ID": 12345678901234567890}}
  }
}
`,
		},
		{
			name:     "the only server",
			existing: `{"mcpServers": {` + quarryEntry + `}}`,
			want:     `{"mcpServers": {}}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			config := filepath.Join(folder, desktopConfigName)
			writeDesktopConfig(t, config, c.existing)
			tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn)

			stdout, stderr, err := runClaudeAt(t, tool, nil, home, "claude", "uninstall")

			require.NoError(t, err)
			assert.Equal(t, pluginUninstalledLine+marketplaceRemovedLine+uninstallRestartLine+
				desktopRemovedLine+desktopQuitUnloadLine, stdout)
			assert.Empty(t, stderr)
			written, err := os.ReadFile(config)
			require.NoError(t, err)
			assert.Equal(t, decodeConfig(t, []byte(c.want)), decodeConfig(t, written))
			backup, err := os.ReadFile(config + ".before-quarry")
			require.NoError(t, err)
			assert.Equal(t, c.existing, string(backup))
		})
	}
}
