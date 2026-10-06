// Package main: main cannot be imported, so these tests live beside the unexported run and
// the test helpers the cmd/quarry tests share.
package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ruled copy: each constant is pinned byte for byte.
//
//nolint:lll // ruled copy is pinned byte-equal, so its lines cannot wrap
const (
	monthlySummaryQuestion = "What happened last month; a monthly summary"
	monthlySummaryCell     = "`quarry summary --json` (`--month YYYY-MM` for an earlier month)"
	monthlySummaryRef      = "plugin/skills/quarry/references/monthly-summary.md"
	monthlySummaryBullet   = "- [Monthly summary job](references/monthly-summary.md): a launchd job that runs quarry sync and quarry summary on the 1st of each month."
	monthlySummaryHeading  = "## Run a monthly summary"
	monthlySummaryUse      = "Use this when the user wants last month's summary every month without asking: a launchd job that runs `quarry sync`, then `quarry summary`, on the 1st and writes both to a log only they can read. Show the user these steps; write or load the LaunchAgent only when they ask you to."
	monthlySummaryDecision = "- Monthly summary: quarry summary reads the store only; \"last month\" is the calendar month before today in local time; a recurring charge is new in the first month quarry recurring can list it, unless it starts again after an off-schedule charge before its earlier series had ended; recurring is judged as of the month's last day; the job is quarry sync; quarry summary from launchd; no stored state."
)

func Test_monthly_summary_job_is_documented_where_a_reader_looks(t *testing.T) {
	skill := repoFile(t, skillPath)
	readme := repoFile(t, "README.md")
	prd := repoFile(t, "docs/initial-prd.md")

	cell, rowErr := skillRunCell(skill, monthlySummaryQuestion)

	require.NoError(t, rowErr)
	assert.Equal(t, monthlySummaryCell, cell)
	assert.Contains(t, skill, "\n"+monthlySummaryBullet+"\n")
	lines := strings.Split(repoFile(t, monthlySummaryRef), "\n")
	assert.Equal(t, "# Monthly summary job", lines[0])
	assert.Equal(t, monthlySummaryUse, lines[2])
	claudeCode := strings.Index(readme, "\n"+readmeClaudeCodeHeading+"\n")
	section := strings.Index(readme, "\n"+monthlySummaryHeading+"\n")
	credits := strings.Index(readme, "\n"+readmeCreditsHeading+"\n")
	assert.Less(t, claudeCode, section)
	assert.Less(t, section, credits)
	assert.Contains(t, prd, "\n"+monthlySummaryDecision+"\n")
}
