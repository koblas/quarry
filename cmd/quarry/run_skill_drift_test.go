package main

import (
	"context"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// driftSource is one file the skill ships, by repo-relative name.
type driftSource struct {
	name string
	text string
}

// driftCheck is what one kind of name check saw: the names it resolved against
// the product, and the uses that matched nothing.
type driftCheck struct {
	resolved   []string
	mismatches []string
}

// helpNode is one command in the help tree: its long flag names and its children.
type helpNode struct {
	flags    []string
	children map[string]helpNode
}

// skillDriftSources is SKILL.md, the README's Claude Code section, and every file
// under references/, read from disk.
func skillDriftSources(t *testing.T) []driftSource {
	t.Helper()
	return append([]driftSource{
		{name: skillPath, text: repoFile(t, skillPath)},
		{name: "README.md Claude Code section", text: readmeClaudeCodeText(t)},
	}, referenceSources(t)...)
}

// readmeClaudeCodeText is README.md from the Claude Code heading up to the Credits heading.
func readmeClaudeCodeText(t *testing.T) string {
	t.Helper()
	_, afterStart, found := strings.Cut(repoFile(t, "README.md"), readmeClaudeCodeHeading+"\n")
	require.True(t, found, "README.md must carry the Claude Code section")
	section, _, found := strings.Cut(afterStart, readmeCreditsHeading)
	require.True(t, found, "README.md must carry the Credits section after it")
	return readmeClaudeCodeHeading + "\n" + section
}

// referenceSources is every file under references/, listed from disk and named by repo-relative path.
func referenceSources(t *testing.T) []driftSource {
	t.Helper()
	var sources []driftSource
	err := filepath.WalkDir("../../"+referencesDir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel := path.Join(referencesDir, strings.TrimPrefix(filepath.ToSlash(file), "../../"+referencesDir+"/"))
		sources = append(sources, driftSource{name: rel, text: repoFile(t, rel)})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, sources)
	return sources
}

// helpTree is the command tree as `quarry <path> --help` prints it.
func helpTree(_ *testing.T) helpNode { return helpNode{} }

// commandMismatches resolves every `quarry <command> --flag` use in sources against tree.
func commandMismatches(_ helpNode, _ []driftSource) driftCheck { return driftCheck{} }

// toolMismatches resolves every MCP tool name SKILL.md section 9 uses against tools.
func toolMismatches(_ []driftSource, _ []string) driftCheck { return driftCheck{} }

// relationMismatches resolves every v_* view and SQL relation in sources against relations.
func relationMismatches(_ []driftSource, _ []string) driftCheck { return driftCheck{} }

func Test_every_quarry_name_the_skill_uses_exists(t *testing.T) {
	t.Run("quarry commands and flags", func(t *testing.T) {
		check := commandMismatches(helpTree(t), skillDriftSources(t))

		assert.Contains(t, check.resolved, "snapshots prune")
		assert.Empty(t, check.mismatches)
	})

	t.Run("MCP tool names", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
		defer cancel()
		peer := startMCP(ctx, t, func(*cli.Env) {})
		listed, err := peer.session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, peer.session.Close())
		peer.waitForExit(ctx, t)
		tools := make([]string, len(listed.Tools))
		for i, tool := range listed.Tools {
			tools[i] = tool.Name
		}

		check := toolMismatches(skillDriftSources(t), tools)

		assert.Contains(t, check.resolved, "sync_status")
		assert.Empty(t, check.mismatches)
	})

	t.Run("tables and v_* views", func(t *testing.T) {
		check := relationMismatches(skillDriftSources(t), storeRelations(t))

		assert.Contains(t, check.resolved, "v_spending")
		assert.Empty(t, check.mismatches)
	})
}
