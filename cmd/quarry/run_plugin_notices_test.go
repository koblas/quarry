package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			"`spending.md`, `cash-flow.md`, `recurring-and-anomalies.md`, `search.md` and `findings.md` " +
				"(walking David through the findings worklist), with `.sql` recipes where no command answers " +
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
