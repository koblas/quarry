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

func Test_notices_and_prd_carry_the_ruled_plugin_edits(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{
			"notices credit the plugin for the skill layout and rules",
			"THIRD_PARTY_NOTICES",
			"plugin/ (skill layout and the untrusted-data and reporting rules in SKILL.md)",
		},
		{
			"prd lists the shipped references and defers the Phase 4 ones",
			"docs/initial-prd.md",
			"`spending.md`, `cash-flow.md`, `recurring-and-anomalies.md`, `search.md`, `findings.md` " +
				"(walking David through the findings worklist) and `monthly-summary.md` (the launchd job " +
				"that runs sync and summary each month), with `.sql` recipes where no command answers " +
				"the question; `net-worth.md` and `investments.md` arrive with Phase 4.",
		},
		{
			"prd limits the generated schema reference to tables, views and conventions",
			"docs/initial-prd.md",
			"(generated from the store, so it can't drift; it lists tables, views and conventions only, " +
				"never accounts or categories)",
		},
		{
			"prd places the plugin manifest and the marketplace listing",
			"docs/initial-prd.md",
			"bundles the skill and the MCP server config (in `plugin.json`), in `plugin/`, " +
				"listed by `.claude-plugin/marketplace.json` at the repo root",
		},
		{
			"prd records that the category tax line is not imported",
			"docs/initial-prd.md",
			"Quicken's category tax line is not imported; tax totals are by user-named category until it is.",
		},
		{
			"prd notes the securities type and currency conventions",
			"docs/initial-prd.md",
			"(type deferred: Quicken's type codes are unlabelled; currency as recorded, NULL when Quicken has none)",
		},
		{
			"prd names v_holdings as built from holding_shares",
			"docs/initial-prd.md",
			"`v_holdings` (shares and value by day, from `holding_shares`; Phase 4b)",
		},
		{
			"prd lists the holdings command beside accounts",
			"docs/initial-prd.md",
			"| `quarry accounts` | Accounts with current balances, closed ones on request | " +
				"| `quarry holdings` | Securities held in each investment account on one day " +
				"(`--as-of`, default today) with share count, latest price and its date, and value; " +
				"`--account` to narrow, cash in investment accounts not included |",
		},
		{
			"prd lists the holdings MCP tool beside search_transactions",
			"docs/initial-prd.md",
			"transfers and report-excluded transactions included and flagged | " +
				"| `holdings` | Securities held on one day (as_of, default today) with share count, " +
				"latest price and its date, and value, with accounts and currency parameters; " +
				"cash in investment accounts not included |",
		},
		{
			"prd names the investment transactions table and its cash side in transactions",
			"docs/initial-prd.md",
			"| `investment_transactions` (commission DECIMAL(18,4)) | Action (buy, sell, dividend, reinvest, share transfer, split), security, shares, price, fees, amount " +
				"| Cash side also appears in `transactions` |",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, collapseWhitespace(repoFile(t, tt.file)), tt.want)
		})
	}
}

// collapseWhitespace joins wrapped lines so a pinned sentence matches wherever
// the file breaks it.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

const (
	readmeClaudeCodeHeading = "## Use quarry with Claude Code"
	readmeCreditsHeading    = "## Credits"
)

func Test_readme_section_for_claude_code_is_verbatim_and_precedes_credits(t *testing.T) {
	readme := repoFile(t, "README.md")

	section := readmeClaudeCodeText(t)

	assert.Equal(t, ticks(readmeClaudeCodeSection), section)
	assert.Less(t, strings.Index(readme, readmeClaudeCodeHeading), strings.Index(readme, "\n"+readmeCreditsHeading+"\n"))
}

// readmeClaudeCodeText is README.md from the Claude Code heading to the end of that section, which the next `## ` heading closes.
func readmeClaudeCodeText(t *testing.T) string {
	t.Helper()
	readme := repoFile(t, "README.md")
	start := strings.Index(readme, readmeClaudeCodeHeading+"\n")
	require.GreaterOrEqual(t, start, 0, "README.md must carry the Claude Code section")
	rest := readme[start:]
	end := strings.Index(rest[len(readmeClaudeCodeHeading):], "\n## ")
	require.GreaterOrEqual(t, end, 0, "a section must follow the Claude Code section")
	return strings.TrimRight(rest[:len(readmeClaudeCodeHeading)+end], "\n")
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

quarry's only network request is the exchange-rate fetch during ¤quarry sync¤, which carries nothing but dates, back to the date of your earliest transaction. The output of the commands Claude runs becomes part of your conversation with Claude, so ask for totals rather than full transaction lists when that is all you need.

To update the plugin: ¤claude plugin marketplace update quarry¤. Update the quarry binary at the same time; if Claude reports that quarry is older than the skill, update quarry.`
