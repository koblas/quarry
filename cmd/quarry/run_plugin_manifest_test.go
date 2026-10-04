// Package main: main cannot be imported, so these tests live beside the unexported run and
// the test helpers the cmd/quarry tests share.
package main

import (
	"encoding/json"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wantMarketplaceJSON = `{
  "name": "quarry",
  "owner": { "name": "David Koblas" },
  "description": "quarry: answer questions from your Quicken Classic for Mac data",
  "plugins": [
    { "name": "quarry", "source": "./plugin", "description": "Skill and MCP server config for quarry, which reads your Quicken Classic for Mac data" }
  ]
}
`

const wantPluginJSON = `{
  "name": "quarry",
  "description": "Answer questions about your money from your Quicken Classic for Mac data with quarry. Requires the quarry binary on your PATH.",
  "version": "0.1.0",
  "author": { "name": "David Koblas" },
  "mcpServers": { "quarry": { "command": "quarry", "args": ["mcp"] } }
}
`

func Test_plugin_manifests_list_quarry_and_start_its_mcp_server(t *testing.T) {
	marketplace := repoFile(t, ".claude-plugin/marketplace.json")
	plugin := repoFile(t, "plugin/.claude-plugin/plugin.json")

	assert.Equal(t, wantMarketplaceJSON, marketplace) //nolint:testifylint // byte-equal is the contract, not JSON-equivalence
	assert.Equal(t, wantPluginJSON, plugin)           //nolint:testifylint // byte-equal is the contract, not JSON-equivalence
	assert.NoFileExists(t, repoRoot+"plugin/.mcp.json")
}

func Test_plugin_manifests_agree_on_where_the_plugin_lives_and_how_it_starts_quarry(t *testing.T) {
	var marketplace struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal([]byte(repoFile(t, ".claude-plugin/marketplace.json")), &marketplace))
	var plugin struct {
		Name       string `json:"name"`
		McpServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(repoFile(t, "plugin/.claude-plugin/plugin.json")), &plugin))

	require.Len(t, marketplace.Plugins, 1)
	listed := marketplace.Plugins[0]
	assert.True(t, strings.HasPrefix(listed.Source, "./"), "marketplace source %q", listed.Source)
	assert.FileExists(t, path.Join(repoRoot, listed.Source, ".claude-plugin/plugin.json"))
	assert.Equal(t, listed.Name, plugin.Name)
	assert.Equal(t, "quarry", plugin.McpServers["quarry"].Command)
	assert.Equal(t, []string{"mcp"}, plugin.McpServers["quarry"].Args)
}

// repoRoot is the repository root as seen from cmd/quarry, where go test runs.
const repoRoot = "../../"

// repoFile reads a file named relative to the repo root.
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(repoRoot + rel)
	require.NoError(t, err)
	return string(raw)
}
