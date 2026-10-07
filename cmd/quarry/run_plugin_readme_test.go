// Package main: main cannot be imported, so these tests live beside the unexported run and
// the test helpers the cmd/quarry tests share.
package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	readmeClaudeCodeHeading = "## Use quarry with Claude Code"
	readmeInstallHeading    = "## Install or remove the plugin"
	readmeMonthlyHeading    = "## Run a monthly summary"
	readmeCreditsHeading    = "## Credits"
)

func Test_readme_section_for_claude_code_is_verbatim_and_precedes_credits(t *testing.T) {
	readme := repoFile(t, "README.md")

	section := readmeSection(t, readmeClaudeCodeHeading)

	assert.Equal(t, ticks(readmeClaudeCodeSection), section)
	assert.Less(t, strings.Index(readme, readmeClaudeCodeHeading), strings.Index(readme, "\n"+readmeCreditsHeading+"\n"))
}

func Test_readme_section_for_installing_and_removing_the_plugin_is_verbatim(t *testing.T) {
	readme := repoFile(t, "README.md")

	section := readmeSection(t, readmeInstallHeading)

	assert.Equal(t, ticks(readmeInstallSection), section)
	assert.Less(t, strings.Index(readme, readmeClaudeCodeHeading), strings.Index(readme, "\n"+readmeInstallHeading+"\n"))
	assert.Less(t, strings.Index(readme, "\n"+readmeInstallHeading+"\n"), strings.Index(readme, "\n"+readmeMonthlyHeading+"\n"))
	assert.Less(t, strings.Index(readme, "\n"+readmeMonthlyHeading+"\n"), strings.Index(readme, "\n"+readmeCreditsHeading+"\n"))
}

// readmeSection is README.md from heading to the end of that section, which the next `## ` heading closes.
func readmeSection(t *testing.T, heading string) string {
	t.Helper()
	readme := repoFile(t, "README.md")
	start := strings.Index(readme, heading+"\n")
	require.GreaterOrEqual(t, start, 0, "README.md must carry the %q section", heading)
	rest := readme[start:]
	end := strings.Index(rest[len(heading):], "\n## ")
	require.GreaterOrEqual(t, end, 0, "a section must follow the %q section", heading)
	return strings.TrimRight(rest[:len(heading)+end], "\n")
}

// Ruled copy: the README sections, byte for byte.
//
//nolint:lll // ruled copy is pinned byte-equal, so its lines cannot wrap
const (
	readmeClaudeCodeSection = `## Use quarry with Claude Code

quarry ships a Claude Code plugin: a skill that teaches Claude to answer questions from your Quicken data with quarry, and the config for quarry's MCP server. The plugin runs the ¤quarry¤ binary from your PATH, so install quarry first and check it works in a terminal:

¤¤¤
command -v quarry      # prints the path; if empty, add $(go env GOPATH)/bin to your PATH
quarry status          # if there is no store yet, open your Quicken file and run: quarry sync
¤¤¤

Then install the plugin:

¤¤¤
quarry claude install
¤¤¤

This runs ¤claude plugin marketplace add koblas/quarry¤ and ¤claude plugin install quarry@quarry¤ for you; you can run those two yourself instead.

Ask Claude a question such as "How did our grocery spending change since 2022?" or "Which subscriptions started this year?", or type ¤/quarry:quarry¤ to load the skill yourself. Claude checks how fresh the data is with ¤quarry status¤, answers from quarry's output, and runs ¤quarry sync¤ only when you ask.

quarry's only network request is the exchange-rate fetch during ¤quarry sync¤, which carries nothing but dates, back to the date of your earliest transaction; ¤quarry claude install¤ has Claude Code download the plugin from GitHub, and quarry itself sends nothing. The output of the commands Claude runs becomes part of your conversation with Claude, so ask for totals rather than full transaction lists when that is all you need.

To update the plugin: ¤claude plugin marketplace update quarry¤. Update the quarry binary at the same time; if Claude reports that quarry is older than the skill, update quarry.`

	readmeInstallSection = `## Install or remove the plugin

¤quarry claude install¤ runs two commands for you, skipping any that is already done, so running it again is safe:

¤¤¤
claude plugin marketplace add --scope user koblas/quarry
claude plugin install --scope user quarry@quarry
¤¤¤

Restart Claude Code to load the plugin. ¤quarry claude uninstall¤ runs the reverse, and leaves your quarry store alone:

¤¤¤
claude plugin uninstall --scope user quarry@quarry
claude plugin marketplace remove --scope user quarry
¤¤¤

If you installed the plugin in a project, that copy stays, and ¤quarry claude uninstall¤ names it so you can remove it yourself.

To add only the MCP server, without the skill, run:

¤¤¤
claude mcp add --scope user quarry -- quarry mcp
¤¤¤`
)
