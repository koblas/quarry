// Package main: main cannot be imported, so these tests live beside the unexported run and
// the test helpers the cmd/quarry tests share.
package main

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const referencesDir = "plugin/skills/quarry/references"

var findingTypeLine = regexp.MustCompile(`^ {2}([a-z][a-z-]*) {2,}\S`)

const findingsHelpTypesHeading = "quarry looks for:"

// findingTypesInHelp is the finding types that `quarry findings --help` lists under its types heading.
func findingTypesInHelp(t *testing.T) []string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(t.Context(), []string{"findings", "--help"}, &stdout, &stderr), stderr.String())
	_, listing, found := strings.Cut(stdout.String(), findingsHelpTypesHeading+"\n")
	require.True(t, found, "quarry findings --help has no %q list", findingsHelpTypesHeading)
	var types []string
	for line := range strings.SplitSeq(listing, "\n") {
		if line == "" {
			break
		}
		if match := findingTypeLine.FindStringSubmatch(line); match != nil {
			types = append(types, match[1])
		}
	}
	require.NotEmpty(t, types)
	return types
}

// findingTypeBullets is each type as findings.md's types list opens its bullet, so a type named elsewhere does not count.
func findingTypeBullets(types []string) []string {
	bullets := make([]string, len(types))
	for i, name := range types {
		bullets[i] = "- `" + name + "`:"
	}
	return bullets
}

// quickenTableNames is the Quicken Z-table names that text mentions.
func quickenTableNames(text string) []string {
	return zTableName.FindAllString(text, -1)
}

func Test_reference_files_state_their_job(t *testing.T) {
	cases := []struct {
		file    string
		phrases []string
	}{
		{"spending.md", []string{
			"quarry spend", "--by", "--currency", "refund", "references/sql/spending-trend.sql",
			"do not compare a partial period with a whole one as if they were equal",
			"| `currency` | `'CAD'`, `'USD'` or `'native'`. Use the `currency` that `quarry spend --json` reports, " +
				"so the trend matches the user's other totals; `'native'` lists each currency unconverted, never added together. |",
		}},
		{"cash-flow.md", []string{
			"quarry cashflow", "savings rate", "n/a", "partial", "references/sql/income-by-category.sql",
			"| `currency` | `'CAD'`, `'USD'` or `'native'`. Use the `currency` that `quarry cashflow --json` reports, " +
				"so the recipe matches the user's other totals; `'native'` lists each currency unconverted, never added together. |",
		}},
		{"recurring-and-anomalies.md", []string{
			"quarry recurring --json", "quarry anomalies --json", "`new`", "`state`", "`first_charge`", "`price_changes`",
			"`per_year`", "`usual`", "`times`", "`not_judged`", "no SQL form",
			"more than 14 days (weekly), 45 days (monthly), 120 days (quarterly) or 400 days (annual)",
		}},
		{"search.md", []string{"quarry search", "transfer", "excluded", "native", "--limit"}},
		{"findings.md", append(findingTypeBullets(findingTypesInHelp(t)), []string{
			"in Quicken, then `quarry sync`", "findings.ignore", "quarry findings --csv", "only when the user asks",
			"compare register entries only, not buys, sells, dividends or other investment transactions",
			"which accounts quarry needs classified",
			"`unclassified-account`: a brokerage or retirement account, open or closed, that the config file lists as neither registered nor non-registered. " +
				"Fixed in quarry's config, not Quicken; see \"Classifying accounts\".",
		}...)},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			text := collapseWhitespace(repoFile(t, referencesDir+"/"+c.file))

			for _, phrase := range c.phrases {
				assert.Contains(t, text, phrase)
			}
		})
	}
}

func Test_references_name_scan_flags_crafted_quicken_text(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"a Quicken table", "join ZTRANSACTION on it", "ZTRANSACTION"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{c.want}, quickenTableNames(c.text))
		})
	}
}

// toolNamedJSONFields are JSON fields the references name that share a tool's name.
var toolNamedJSONFields = []string{"anomalies"}

// mcpToolSpans is the tool names, JSON fields of the same name aside, that sources spell as a code span or fenced line.
func mcpToolSpans(sources []driftSource, tools []string) []string {
	var named []string
	for _, source := range sources {
		for _, unit := range codeUnits(source.text) {
			if name := strings.TrimSpace(unit.text); slices.Contains(tools, name) && !slices.Contains(toolNamedJSONFields, name) {
				named = append(named, fmt.Sprintf("%s:%d: %s", source.name, unit.line, name))
			}
		}
	}
	return named
}

func Test_references_name_scan_flags_crafted_tool_names(t *testing.T) {
	tools := []string{"sync_status", "query", "anomalies"}
	cases := []struct {
		name, text string
		want       []string
	}{
		{"a tool in a code span", "Call `sync_status` first.", []string{"crafted:1: sync_status"}},
		{"a tool on a fenced line", "```\nquery\n```", []string{"crafted:2: query"}},
		{"a JSON field named like a tool", "`anomalies` lists the charges.", nil},
		{"a tool name in prose", "Run a query on sync_status.", nil},
		{"a code span that only contains a tool name", "`quarry sync_status`", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, mcpToolSpans([]driftSource{{name: "crafted", text: c.text}}, tools))
		})
	}
}

func Test_references_name_no_mcp_tool(t *testing.T) {
	tools := mcpToolNames(t)

	require.Contains(t, tools, "sync_status")
	assert.Empty(t, mcpToolSpans(referenceSources(t), tools))
}

func Test_references_name_no_quicken_table(t *testing.T) {
	sources := skillDriftSources(t)
	require.Greater(t, len(sources), 2)

	for _, source := range sources {
		t.Run(source.name, func(t *testing.T) {
			assert.Empty(t, quickenTableNames(source.text))
		})
	}
}
