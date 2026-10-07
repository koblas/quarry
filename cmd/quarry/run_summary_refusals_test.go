package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	summaryNotAMonthLine = "quarry: --month %q is not a month; use YYYY-MM, such as 2026-09\n"
	summaryNotEndedLine  = "quarry: --month %s has not ended; summary covers whole months, so pass 2026-09 or earlier\n"
)

func Test_run_summary_refuses_a_month_it_cannot_summarize(t *testing.T) {
	cases := []struct {
		name       string
		month      string
		wantStderr string
	}{
		{name: "a month without its leading zero", month: "2026-9", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "2026-9")},
		{name: "a day", month: "2026-09-15", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "2026-09-15")},
		{name: "year zero", month: "0000-01", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "0000-01")},
		{name: "a month past twelve", month: "2026-13", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "2026-13")},
		{name: "a year alone", month: "2026", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "2026")},
		{name: "an empty value", month: "", wantStderr: fmt.Sprintf(summaryNotAMonthLine, "")},
		{name: "the current month", month: "2026-10", wantStderr: fmt.Sprintf(summaryNotEndedLine, "2026-10")},
		{name: "a later month", month: "2027-01", wantStderr: fmt.Sprintf(summaryNotEndedLine, "2027-01")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--month", c.month}, summaryClock)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_summary_accepts_a_month_that_has_ended(t *testing.T) {
	home := newHome(t)

	exitCode, _, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--month", "2026-09"}, summaryClock)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: no store at "+abbreviated(t, storePathUnder(home), home)+" yet; run quarry sync to build it\n", stderr.String())
}

func Test_run_summary_checks_arguments_then_month_then_config_then_store(t *testing.T) {
	const malformedConfig = "[snapshots\nkeep = 24\n"
	cases := []struct {
		name       string
		config     string
		args       []string
		wantExit   int
		wantStderr string
	}{
		{
			name: "a bad currency before a bad month", args: []string{"summary", "--currency", "EUR", "--month", "2026-9"},
			config: malformedConfig, wantExit: 2, wantStderr: badCurrencyFlag,
		},
		{
			name: "a bad month before a malformed config", args: []string{"summary", "--month", "2026-10"},
			config: malformedConfig, wantExit: 2, wantStderr: fmt.Sprintf(summaryNotEndedLine, "2026-10"),
		},
		{
			name: "a missing month value before a malformed config", args: []string{"summary", "--month"},
			config: malformedConfig, wantExit: 2, wantStderr: "quarry: flag needs an argument: --month; Run 'quarry summary --help' for usage.\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			writeConfig(t, home, c.config)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), c.args, summaryClock)

			assert.Equal(t, c.wantExit, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_summary_reads_the_config_before_looking_for_a_store(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, "[snapshots\nkeep = 24\n")

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary"}, summaryClock)

	require.Equal(t, 1, exitCode, stderr.String())
	assert.Empty(t, stdout.String())
	assert.Regexp(t, "^"+regexp.QuoteMeta("quarry: cannot read "+configShown+": line 1: ")+"[^\n]+"+regexp.QuoteMeta(configFix)+"\n$", stderr.String())
}

func Test_run_summary_with_a_currency_refuses_when_the_home_directory_is_unknown(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--currency", "USD"}, summaryClock)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry summary again\n", stderr.String())
}

func Test_run_summary_with_a_currency_prints_only_the_missing_store_when_the_config_is_malformed(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, "[snapshots\nkeep = 24\n")

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary", "--currency", "USD"}, summaryClock)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: no store at "+abbreviated(t, storePathUnder(home), home)+" yet; run quarry sync to build it\n", stderr.String())
}

func Test_run_summary_refuses_a_store_built_by_an_older_quarry(t *testing.T) {
	assertRefusesAnOlderStore(t, "summary")
}
