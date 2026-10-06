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
	require.GreaterOrEqual(t, len(lines), 3)
	assert.Equal(t, "# Monthly summary job", lines[0])
	assert.Equal(t, monthlySummaryUse, lines[2])
	assert.Contains(t, readme, "\n"+monthlySummaryHeading+"\n")
	_, afterCommandNames, found := strings.Cut(prd, "\n- Command names:")
	require.True(t, found, "the PRD Decisions list must carry the Command names bullet")
	_, nextLines, _ := strings.Cut(afterCommandNames, "\n")
	assert.Equal(t, monthlySummaryDecision, strings.SplitN(nextLines, "\n", 2)[0])
}

func Test_monthly_summary_reference_carries_the_launchd_recipe_to_the_end_of_the_file(t *testing.T) {
	text := repoFile(t, monthlySummaryRef)
	start := strings.Index(text, "\n## Run quarry summary every month\n")
	require.GreaterOrEqual(t, start, 0, "the reference must carry the recipe heading")

	recipe := text[start+1:]

	assert.Equal(t, ticks(monthlySummaryRecipe)+"\n", recipe)
}

func Test_readme_monthly_summary_section_is_verbatim_between_claude_code_and_credits(t *testing.T) {
	readme := repoFile(t, "README.md")
	start := strings.Index(readme, "\n"+monthlySummaryHeading+"\n")
	require.GreaterOrEqual(t, start, 0, "README.md must carry the monthly summary section")
	rest := readme[start+1:]
	end := strings.Index(rest, "\n## ")
	require.GreaterOrEqual(t, end, 0, "a section must follow the monthly summary section")

	section := strings.TrimRight(rest[:end], "\n")

	assert.Equal(t, ticks(readmeMonthlySummarySection), section)
	assert.Equal(t, "\n"+readmeCreditsHeading, rest[end:end+len("\n"+readmeCreditsHeading)])
	assert.Less(t, strings.Index(readme, readmeClaudeCodeHeading), start)
	assert.True(t, repoFileExists(monthlySummaryRef), "the README link must resolve")
}

// Ruled copy: the README section, byte for byte.
//
//nolint:lll // ruled copy is pinned byte-equal, so its lines cannot wrap
const readmeMonthlySummarySection = `## Run a monthly summary

¤quarry summary¤ prints last month's unusual charges, new recurring charges, net worth change and findings. To get it every month, follow [plugin/skills/quarry/references/monthly-summary.md](plugin/skills/quarry/references/monthly-summary.md).`

// Ruled copy: the launchd recipe, from its heading to the end of the file, byte for byte.
const monthlySummaryRecipe = `## Run quarry summary every month

1. Find quarry's full path with ¤command -v quarry¤ (for example /Users/you/go/bin/quarry).
   launchd does not use your shell's PATH.
2. Save this as ~/Library/LaunchAgents/com.github.koblas.quarry.summary.plist, with both
   /Users/you/go/bin/quarry replaced by that path:

¤¤¤xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.github.koblas.quarry.summary</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/sh</string>
    <string>-c</string>
    <string>umask 077; mkdir -p "$HOME/Library/Logs/quarry"; { /Users/you/go/bin/quarry sync; /Users/you/go/bin/quarry summary; } >>"$HOME/Library/Logs/quarry/summary.log" 2>&amp;1</string>
  </array>
  <key>StartCalendarInterval</key>
  <dict>
    <key>Day</key><integer>1</integer>
    <key>Hour</key><integer>9</integer>
    <key>Minute</key><integer>0</integer>
  </dict>
</dict>
</plist>
¤¤¤

3. Load it:      launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.github.koblas.quarry.summary.plist
4. Try it now:   launchctl kickstart gui/$(id -u)/com.github.koblas.quarry.summary
   then read ~/Library/Logs/quarry/summary.log.
5. To stop it:   launchctl bootout gui/$(id -u)/com.github.koblas.quarry.summary

quarry sync needs Quicken running with your file open. If Quicken was closed when the job ran,
the log shows sync's error line, then a summary that warns its snapshot was taken before the
month ended; open your Quicken file and run ¤quarry sync; quarry summary¤ in a terminal.
If the Mac is asleep at 9:00 on the 1st, launchd runs the job when it wakes.
The log holds your payees, amounts and net worth; umask 077 keeps it readable only by you.`
