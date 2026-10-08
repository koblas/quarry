package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	desktopConfigName   = "claude_desktop_config.json"
	desktopQuarryBinary = "/opt/homebrew/bin/quarry"

	desktopAddedLine = "Added the quarry MCP server to Claude Desktop; it starts \"" + desktopQuarryBinary + "\".\n"
	desktopQuitLine  = "Quit and reopen Claude Desktop to load it.\n"
)

// desktopHome returns a home directory holding an empty Claude Desktop folder, and that folder.
func desktopHome(t *testing.T) (home, folder string) {
	t.Helper()
	home = t.TempDir()
	folder = filepath.Join(home, "Library", "Application Support", "Claude")
	require.NoError(t, os.MkdirAll(folder, 0o755))
	return home, folder
}

func Test_claude_install_adds_quarry_to_claude_desktop_after_claude_code(t *testing.T) {
	home, folder := desktopHome(t)
	tool := &toolCalls{}
	var out, errOut bytes.Buffer

	err := cli.Execute(t.Context(), []string{"claude", "install"}, cli.Env{
		Stdout:     &out,
		Stderr:     &errOut,
		RunTool:    tool.run,
		Home:       home,
		Executable: func() (string, error) { return desktopQuarryBinary, nil },
	})

	require.NoError(t, err)
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine+desktopAddedLine+desktopQuitLine, out.String())
	assert.Empty(t, errOut.String())
	config := filepath.Join(folder, desktopConfigName)
	info, err := os.Stat(config)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	written, err := os.ReadFile(config)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(written, &decoded))
	assert.Equal(t, map[string]any{
		"mcpServers": map[string]any{
			"quarry": map[string]any{"command": desktopQuarryBinary, "args": []any{"mcp"}},
		},
	}, decoded)
	listing, err := os.ReadDir(folder)
	require.NoError(t, err)
	require.Len(t, listing, 1)
	assert.Equal(t, desktopConfigName, listing[0].Name())
}
