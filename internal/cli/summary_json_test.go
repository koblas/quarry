package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryJSONRead is the part of summary's document these tests read.
type summaryJSONRead struct {
	Findings struct {
		Ignored *int `json:"ignored"`
	} `json:"findings"`
	Warnings []string `json:"warnings"`
}

func readSummaryJSON(t *testing.T, stdout *bytes.Buffer) summaryJSONRead {
	t.Helper()
	var doc summaryJSONRead

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	return doc
}

const (
	augustPredatesWarning = "the store was built from a snapshot taken 2026-08-30 12:00 UTC, before August 2026 ended, " +
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, " +
		"then run quarry summary again"
	julyPredatesWarning = "the store was built from a snapshot taken 2026-07-30 12:00 UTC, before July 2026 ended, " +
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, " +
		"then run quarry summary --month 2026-07 again"
	cannotTellWarning = "cannot tell which findings you ignored or how you classified your accounts: " +
		"cannot read config.toml: permission denied; findings you ignored are counted as open, " +
		"and every investment account is counted as unclassified"
)

var augustTaken = time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)

func Test_summary_json_lists_the_configs_warning_in_its_absolute_form_before_the_snapshot_warning(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSummary(t, summaryTakenAt(augustTaken), warningConfig, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.Equal(t, []string{unknownKeyAbsolute, augustPredatesWarning}, readSummaryJSON(t, &stdout).Warnings)
	assert.Equal(t, "quarry: warning: "+unknownKeyShown+"\nquarry: warning: "+augustPredatesWarning+"\n", stderr.String())
}

func Test_summary_json_lists_the_cannot_tell_warning_before_the_snapshot_warning_when_the_config_is_unreadable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	unreadable := func(string) (config.Config, error) { return config.Config{}, errConfigRead }

	err := executeSummary(t, summaryTakenAt(augustTaken), unreadable, &stdout, &stderr, "--json", "--currency", "USD")

	require.NoError(t, err)
	assert.Equal(t, []string{cannotTellWarning, augustPredatesWarning}, readSummaryJSON(t, &stdout).Warnings)
}

func Test_summary_json_leaves_ignored_null_when_the_config_is_unreadable_and_a_currency_is_given(t *testing.T) {
	var stdout, stderr bytes.Buffer
	unreadable := func(string) (config.Config, error) { return config.Config{}, errConfigRead }

	err := executeSummary(t, summaryTakenAt(augustTaken), unreadable, &stdout, &stderr, "--json", "--currency", "USD")

	require.NoError(t, err)
	assert.Nil(t, readSummaryJSON(t, &stdout).Findings.Ignored)
}

func Test_summary_json_counts_ignored_when_the_config_is_readable(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSummary(t, summaryTakenAt(augustTaken), cadConfig, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.Equal(t, new(0), readSummaryJSON(t, &stdout).Findings.Ignored)
}

func Test_summary_prints_the_same_warnings_on_stderr_with_and_without_json(t *testing.T) {
	unreadable := func(string) (config.Config, error) { return config.Config{}, errConfigRead }
	var plainOut, plainErr, jsonOut, jsonErr bytes.Buffer

	plain := executeSummary(t, summaryTakenAt(augustTaken), unreadable, &plainOut, &plainErr, "--currency", "USD")
	asJSON := executeSummary(t, summaryTakenAt(augustTaken), unreadable, &jsonOut, &jsonErr, "--currency", "USD", "--json")

	require.NoError(t, plain)
	require.NoError(t, asJSON)
	assert.Equal(t, "quarry: warning: "+cannotTellWarning+"\nquarry: warning: "+augustPredatesWarning+"\n", plainErr.String())
	assert.Equal(t, plainErr.String(), jsonErr.String())
}

func Test_summary_json_ends_the_snapshot_warning_with_the_summary_asked_for_when_a_month_is_named(t *testing.T) {
	var stdout, stderr bytes.Buffer
	taken := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)

	err := executeSummary(t, summaryTakenAt(taken), cadConfig, &stdout, &stderr, "--json", "--month", "2026-07")

	require.NoError(t, err)
	assert.Equal(t, []string{julyPredatesWarning}, readSummaryJSON(t, &stdout).Warnings)
}

func Test_summary_json_lists_no_warning_for_a_snapshot_taken_when_the_month_ended(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSummary(t, summaryTakenAt(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)), cadConfig, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.Equal(t, []string{}, readSummaryJSON(t, &stdout).Warnings)
	assert.Empty(t, stderr.String())
}

func Test_summary_json_refuses_a_failed_write_without_the_snapshot_warning(t *testing.T) {
	var stderr bytes.Buffer

	err := executeSummary(t, summaryTakenAt(augustTaken), cadConfig, failingWriter{err: errNoSpace}, &stderr, "--json")

	require.ErrorIs(t, err, errNoSpace)
	require.ErrorContains(t, err, "cannot write the result to stdout")
	assert.Empty(t, stderr.String())
}

func Test_summary_json_refuses_what_summary_refuses_and_prints_nothing(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "a month that is not a month", args: []string{"--month", "2026-9"}, want: `--month "2026-9" is not a month; use YYYY-MM, such as 2026-08`},
		{name: "a month that has not ended", args: []string{"--month", "2026-09"}, want: "--month 2026-09 has not ended; summary covers whole months, so pass 2026-08 or earlier"},
		{name: "a currency that is not CAD, USD or native", args: []string{"--currency", "EUR"}, want: badCurrencyFlag},
		{name: "a positional argument", args: []string{"extra"}, want: "summary takes no arguments"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSummary(t, summaryTakenAt(augustTaken), cadConfig, &stdout, &stderr, append(c.args, "--json")...)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			require.EqualError(t, err, c.want)
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}
