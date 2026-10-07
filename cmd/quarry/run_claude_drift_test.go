// Package main: main cannot be imported, so these tests live beside the unexported run and
// the test helpers the shared cmd/quarry tests share.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const claudePluginLinePrefix = "  claude plugin "

func Test_claude_commands_agree_with_the_manifests_and_readme(t *testing.T) {
	install := claudePluginLines(t, "install")
	uninstall := claudePluginLines(t, "uninstall")
	section := readmeSection(t, readmeInstallHeading)
	claudeCodeSection := readmeSection(t, readmeClaudeCodeHeading)
	var marketplace struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal([]byte(repoFile(t, ".claude-plugin/marketplace.json")), &marketplace))
	var plugin struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal([]byte(repoFile(t, "plugin/.claude-plugin/plugin.json")), &plugin))
	modulePath := strings.Fields(strings.SplitN(repoFile(t, "go.mod"), "\n", 2)[0])[1]

	require.Len(t, install, 2)
	require.Len(t, uninstall, 2)
	for _, line := range append(install, uninstall...) {
		assert.Contains(t, section, "\n"+line+"\n", "README install section must carry the help's command line")
	}
	for _, line := range install {
		scopeless := strings.Replace(line, " --scope user", "", 1)
		assert.Contains(t, claudeCodeSection, "`"+scopeless+"`", "README Claude Code section must name the help's command without its scope")
	}
	assert.Equal(t, strings.TrimPrefix(modulePath, "github.com/"), lastField(install[0]), "marketplace add names the repository")
	assert.Equal(t, plugin.Name+"@"+marketplace.Name, lastField(install[1]), "install names the plugin in the marketplace")
	assert.Equal(t, plugin.Name+"@"+marketplace.Name, lastField(uninstall[0]), "uninstall names the same plugin")
	assert.Equal(t, marketplace.Name, lastField(uninstall[1]), "marketplace remove names the marketplace")
}

// claudePluginLines are the `claude plugin ...` command lines in `quarry claude <name> --help`.
func claudePluginLines(t *testing.T, name string) []string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"claude", name, "--help"}, &stdout, &stderr))
	var lines []string
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if strings.HasPrefix(line, claudePluginLinePrefix) {
			lines = append(lines, strings.TrimPrefix(line, "  "))
		}
	}
	return lines
}

func lastField(line string) string {
	fields := strings.Fields(line)
	return fields[len(fields)-1]
}
