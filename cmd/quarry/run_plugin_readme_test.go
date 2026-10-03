// The README section is read by repo-relative path, so this test lives in
// package main beside the other cmd/quarry tests.
package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	readmeClaudeCodeHeading = "## Use quarry with Claude Code"
	readmeCreditsHeading    = "## Credits"
)

func Test_readme_section_for_claude_code_is_verbatim_and_precedes_credits(t *testing.T) {
	readme := repoFile(t, "README.md")

	start := strings.Index(readme, readmeClaudeCodeHeading+"\n")
	require.GreaterOrEqual(t, start, 0, "README.md must carry the Claude Code section")
	rest := readme[start:]
	end := strings.Index(rest[len(readmeClaudeCodeHeading):], "\n## ")
	require.GreaterOrEqual(t, end, 0, "a section must follow the Claude Code section")
	section := strings.TrimRight(rest[:len(readmeClaudeCodeHeading)+end], "\n")

	assert.Equal(t, ticks(readmeClaudeCodeSection), section)
	assert.Less(t, start, strings.Index(readme, "\n"+readmeCreditsHeading+"\n"))
}

// Ruled copy: the README section, byte for byte.
//
//nolint:lll // ruled copy is pinned byte-equal, so its lines cannot wrap
const readmeClaudeCodeSection = `## Use quarry with Claude Code

quarry ships a Claude Code plugin: a skill that teaches Claude to answer questions from your Quicken data with quarry, and the config for quarry's MCP server. The plugin runs the ¤quarry¤ binary from your PATH, so install quarry first and check it works in a terminal:

¤¤¤
command -v quarry      # prints the path; if empty, add $(go env GOPATH)/bin to your PATH
quarry status          # if there is no store yet, open your Quicken file and run: quarry sync
¤¤¤

Then add the marketplace and install the plugin:

¤¤¤
claude plugin marketplace add koblas/quarry
claude plugin install quarry@quarry
¤¤¤

Ask Claude a question such as "How did our grocery spending change since 2022?" or "Which subscriptions started this year?", or type ¤/quarry:quarry¤ to load the skill yourself. Claude checks how fresh the data is with ¤quarry status¤, answers from quarry's output, and runs ¤quarry sync¤ only when you ask.

quarry itself sends nothing anywhere, but the output of the commands Claude runs becomes part of your conversation with Claude. Ask for totals rather than full transaction lists when that is all you need.

To update the plugin: ¤claude plugin marketplace update quarry¤. Update the quarry binary at the same time; if Claude reports that quarry is older than the skill, update quarry.`
