// Package main: main cannot be imported, so these tests live beside the unexported run and
// the test helpers the cmd/quarry tests share.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/config"
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
		{"monthly-summary.md", []string{
			"quarry sync", "quarry summary", "launchd", "StartCalendarInterval", "umask 077", "launchctl bootstrap",
			"launchctl bootout", "command -v quarry", "only when they ask you to",
			"SKILL.md sections 2 and 3 set the rules for every number you quote.",
		}},
		{"findings.md", append(findingTypeBullets(findingTypesInHelp(t)), []string{
			"in Quicken, then `quarry sync`", "findings.ignore", "quarry findings --csv", "only when the user asks",
			"prints one row per transaction, split, payee, category, account or investment transaction",
			"compare register entries only, not buys, sells, dividends or other investment transactions",
			"which accounts quarry needs classified",
			"`unclassified-account`: a brokerage or retirement account, open or closed, that the config file lists as neither registered nor non-registered. " +
				"Fixed in quarry's config, not Quicken; see \"Classifying accounts\".",
			"`shares-without-cost`: shares moved or added into a non-registered account with no cost basis in Quicken. " +
				"Open the Add Shares transaction and enter the cost (from the old broker's statement); quarry acb counts them at no cost until then.",
			"A finding that quarry no longer finds after the sync is marked fixed, except shares-without-cost, which leaves the list without being marked fixed.",
			"- quarry never writes that file. You write it only when the user asks, to record an account classification the user just gave you " +
				"(see \"Classifying accounts\"), or to add `acb.adjustment` lines for amounts the user reads you from a T3 slip (see \"ACB adjustments\"); " +
				"show the user the exact lines first, and write them only after the user says yes.",
			"## Classifying accounts", "## ACB adjustments",
			"(RRSP, RRIF, TFSA, RESP, FHSA, LIRA, a US 401(k) or IRA, or similar)",
			"Never guess from the account's name or from Quicken calling it a retirement account.",
			"Ignoring one does not stop quarry acb from needing it.",
			"Then run `quarry findings --type unclassified-account --status all --json` to confirm none are left",
			"Never write a second `[accounts]` line.",
			"never compute or guess one",
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
var toolNamedJSONFields = []string{"anomalies", "net_worth"}

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
	tools := []string{"sync_status", "query", "anomalies", "net_worth"}
	cases := []struct {
		name, text string
		want       []string
	}{
		{"a tool in a code span", "Call `sync_status` first.", []string{"crafted:1: sync_status"}},
		{"a tool on a fenced line", "```\nquery\n```", []string{"crafted:2: query"}},
		{"a JSON field named like a tool", "`anomalies` lists the charges.", nil},
		{"a second JSON field named like a tool", "`net_worth` holds the balances.", nil},
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

const (
	skillACBRow = "| Holdings and their value on a day | `quarry holdings --as-of <date> --json` |\n" +
		"| ACB, capital gains for a tax year | `quarry acb [--year <y>] [--security <s>] --json`; see `references/findings.md` \"Classifying accounts\" first |\n"
	skillACBTrigger = "The first time ACB or gains are asked for, run `quarry findings --type unclassified-account --status all --json`; " +
		"if it lists any account, classify them (`references/findings.md`, \"Classifying accounts\") before running `quarry acb`."
	classifyingAccountsHeading = "## Classifying accounts"
)

// rawMarkdownSection is the text under heading up to the next "## " heading; empty when heading is absent.
func rawMarkdownSection(text, heading string) string {
	_, rest, _ := strings.Cut(text, "\n"+heading+"\n")
	section, _, _ := strings.Cut(rest, "\n## ")
	return section
}

// markdownSection is rawMarkdownSection with its whitespace collapsed.
func markdownSection(text, heading string) string {
	return collapseWhitespace(rawMarkdownSection(text, heading))
}

func Test_skill_has_claude_classify_accounts_before_the_first_acb(t *testing.T) {
	skill := splitSkill(t, repoFile(t, skillPath))
	section4 := skill.bodies[skillHeadings[3]]
	classifying := markdownSection(repoFile(t, referencesDir+"/findings.md"), classifyingAccountsHeading)

	assert.Contains(t, section4, skillACBRow)
	assert.True(t, strings.HasSuffix(section4, "\n\n"+skillACBTrigger), "the trigger must be the last paragraph of section 4")
	assert.Contains(t, classifying, "(RRSP, RRIF, TFSA, RESP, FHSA, LIRA, a US 401(k) or IRA, or similar)")
	assert.Contains(t, classifying, "`quarry findings --type unclassified-account --status all --json`")
}

// Ruled copy: the two findings.md sections, word for word, with ¤ for a backtick.
//
//nolint:lll // ruled copy is pinned word for word, so its lines cannot wrap
const (
	adjustmentsHeading = "## ACB adjustments"

	findingsClassifyingSection = `- quarry acb needs every brokerage and retirement account, open or closed, listed as registered or non-registered in ¤~/Library/Application Support/quarry/config.toml¤. Quicken's file does not say which is which.
- List the ones left with ¤quarry findings --type unclassified-account --status all --json¤. Ignoring one does not stop quarry acb from needing it.
- For each, ask the user: "Is <account> (<type>, <currency>) a registered plan (RRSP, RRIF, TFSA, RESP, FHSA, LIRA, a US 401(k) or IRA, or similar) or non-registered?" Never guess from the account's name or from Quicken calling it a retirement account.
- Read config.toml first. Add each id to the existing ¤registered¤ or ¤non-registered¤ list under ¤[accounts]¤, or wherever the file already sets them (¤accounts.registered = …¤, ¤accounts = { … }¤). If the file has neither, add an ¤[accounts]¤ table at its end. Never write a second ¤[accounts]¤ line.
- Put a ¤# <account name>¤ comment beside each id, show the user the exact lines, and write them only after the user says yes.
- Then run ¤quarry findings --type unclassified-account --status all --json¤ to confirm none are left, and relay any line in ¤warnings¤.
- Without access to the file (through the MCP server), give the user the lines to add themselves.

¤¤¤toml
[accounts]
registered = [
  "acct-12",  # Questrade TFSA
  "acct-15",  # RBC RRSP
]
non-registered = ["acct-3"]  # Questrade Margin
¤¤¤`

	findingsAdjustmentsSection = `- quarry acb takes return of capital and reinvested distributions, which a fund reports on a T3 slip and Quicken does not hold, from ¤acb.adjustment¤ items in the config file. Ask the user for each amount, which kind it is, its security and its date; never compute or guess one.
- The security's id (¤sec-…¤) is ¤security_id¤ in ¤securities¤ of ¤quarry acb --json¤.
- Read config.toml first and add one ¤[[acb.adjustment]]¤ item per amount at the end of the file. Show the user the exact lines, and write them only after the user says yes.
- Then run ¤quarry acb --json¤ and relay any ¤acb.adjustment¤ line in ¤warnings¤.

¤¤¤toml
[[acb.adjustment]]
security = "sec-41"  # XEQT
date = 2024-12-31
return-of-capital = 12.34

[[acb.adjustment]]
security = "sec-41"  # XEQT
date = 2024-12-31
reinvested-distribution = 56.78
¤¤¤`
)

// markdownTOMLFences is the body of each ```toml fence in section, in order.
func markdownTOMLFences(section string) []string {
	var fences []string
	lines := strings.Split(section, "\n")
	start := -1
	for i, line := range lines {
		switch {
		case start < 0 && line == "```toml":
			start = i + 1
		case start >= 0 && line == "```":
			fences = append(fences, strings.Join(lines[start:i], "\n")+"\n")
			start = -1
		}
	}

	return fences
}

// loadConfigText loads text as quarry's config file from a fresh home directory.
func loadConfigText(t *testing.T, text string) config.Config {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	cfg, err := config.Load(home, path)
	require.NoError(t, err)

	return cfg
}

func Test_findings_reference_words_the_classifying_and_adjustment_sections_as_ruled(t *testing.T) {
	cases := []struct{ heading, want string }{
		{classifyingAccountsHeading, findingsClassifyingSection},
		{adjustmentsHeading, findingsAdjustmentsSection},
	}
	findings := repoFile(t, referencesDir+"/findings.md")

	for _, c := range cases {
		t.Run(c.heading, func(t *testing.T) {
			assert.Equal(t, collapseWhitespace(ticks(c.want)), markdownSection(findings, c.heading))
		})
	}
}

func Test_findings_reference_toml_fences_are_found_only_when_they_open_as_toml(t *testing.T) {
	text := "```toml\na = 1\nb = 2\n```\n\n```sql\nSELECT 1\n```\n\n```toml\nc = 3\n```\n"

	assert.Equal(t, []string{"a = 1\nb = 2\n", "c = 3\n"}, markdownTOMLFences(text))
}

func Test_findings_reference_classification_example_is_a_config_that_lists_both_account_lists(t *testing.T) {
	fences := markdownTOMLFences(rawMarkdownSection(repoFile(t, referencesDir+"/findings.md"), classifyingAccountsHeading))
	require.Len(t, fences, 1)

	cfg := loadConfigText(t, fences[0])

	assert.Equal(t, []string{"acct-12", "acct-15"}, cfg.Registered)
	assert.Equal(t, []string{"acct-3"}, cfg.NonRegistered)
	assert.Empty(t, cfg.Warnings)
}

func Test_findings_reference_adjustment_example_is_a_config_with_one_item_of_each_kind(t *testing.T) {
	fences := markdownTOMLFences(rawMarkdownSection(repoFile(t, referencesDir+"/findings.md"), adjustmentsHeading))
	require.Len(t, fences, 1)

	cfg := loadConfigText(t, fences[0])

	require.Len(t, cfg.Adjustments, 2)
	assert.Equal(t, []string{"sec-41", "sec-41"}, []string{cfg.Adjustments[0].Security, cfg.Adjustments[1].Security})
	assert.Equal(t, int64(1234), cfg.Adjustments[0].ReturnOfCapital)
	assert.Equal(t, int64(5678), cfg.Adjustments[1].ReinvestedDistribution)
	assert.Empty(t, cfg.Warnings)
}
