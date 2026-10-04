// The reference files are read by repo-relative path, so these tests live in
// package main beside the other cmd/quarry tests.
package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const referencesDir = "plugin/skills/quarry/references"

var (
	findingTypeLine   = regexp.MustCompile(`^ {2}([a-z][a-z-]*) {2,}\S`)
	phase4ViewPattern = regexp.MustCompile(`\bv_(?:balances_daily|net_worth|holdings)\b`)
)

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

// collapsed is text with every run of whitespace turned into one space.
func collapsed(text string) string { return strings.Join(strings.Fields(text), " ") }

// phase4OrQuickenNames is the Phase 4 views and Quicken Z-table names that text mentions.
func phase4OrQuickenNames(text string) []string {
	return append(phase4ViewPattern.FindAllString(text, -1), zTableName.FindAllString(text, -1)...)
}

func Test_reference_files_state_their_job(t *testing.T) {
	cases := []struct {
		file    string
		phrases []string
	}{
		{"spending.md", []string{
			"quarry spend", "--by", "--currency", "refund", "references/sql/spending-trend.sql",
		}},
		{"cash-flow.md", []string{
			"quarry cashflow", "savings rate", "n/a", "partial", "references/sql/income-by-category.sql",
		}},
		{"recurring-and-anomalies.md", []string{
			"quarry recurring --json", "quarry anomalies --json", "`new`", "`state`", "`first_charge`", "`price_changes`",
			"`per_year`", "`usual`", "`times`", "`not_judged`", "no SQL form",
		}},
		{"search.md", []string{"quarry search", "transfer", "excluded", "native", "--limit"}},
		{"findings.md", append(findingTypesInHelp(t), []string{
			"in Quicken, then `quarry sync`", "findings.ignore", "quarry findings --csv", "only when the user asks",
		}...)},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			text := collapsed(repoFile(t, referencesDir+"/"+c.file))

			for _, phrase := range c.phrases {
				assert.Contains(t, text, phrase)
			}
		})
	}
}

func Test_phase_4_and_quicken_name_scan_flags_crafted_text(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"a Phase 4 view", "Read `v_net_worth` for the total.", "v_net_worth"},
		{"another Phase 4 view", "SELECT * FROM v_holdings", "v_holdings"},
		{"a Quicken table", "join ZTRANSACTION on it", "ZTRANSACTION"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{c.want}, phase4OrQuickenNames(c.text))
		})
	}
}

func Test_references_name_no_phase_4_view_or_quicken_table(t *testing.T) {
	sources := skillDriftSources(t)
	require.Greater(t, len(sources), 2)

	for _, source := range sources {
		t.Run(source.name, func(t *testing.T) {
			assert.Empty(t, phase4OrQuickenNames(source.text))
		})
	}
}
