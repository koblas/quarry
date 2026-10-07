package cli_test

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeSummary runs quarry summary against fake at spendNow, whose last month is August 2026. A fake that
// records no snapshot time gets one at spendNow, after every month these tests ask for has ended.
func executeSummary(t *testing.T, fake fakeReportStore, load cli.ConfigLoader, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	fake.summary.Status.Run.Snapshot.TakenAt = cmp.Or(fake.summary.Status.Run.Snapshot.TakenAt, spendNow)
	env := reportEnv(fake, stdout, stderr, atSpendNow, withLoader(load))
	return cli.Execute(t.Context(), append([]string{"summary"}, args...), env)
}

func Test_summary_help_says_what_it_summarizes(t *testing.T) {
	//nolint:dupword // the ruled copy ends the example with "summary" and starts the next paragraph with it
	const long = `Summarize one month, last month unless --month names another: how fresh
the store is and how many findings are open, the month's unusually large
charges, the recurring charges new in the month, and net worth at the end
of the month beside the end of the month before. It is meant to run once
a month after quarry sync, for example from launchd:

  quarry sync; quarry summary

summary only reads quarry's store; it never runs sync and never looks at
Quicken, so it works with Quicken closed. When the store was built from a
snapshot taken before the month ended, summary warns, since transactions
from the rest of the month are missing; open your Quicken file, run quarry
sync, then run summary again.

Unusually large charges are those quarry anomalies lists for the month,
for example quarry anomalies --since 2026-09 --until 2026-09.

A recurring charge is new in the first month quarry recurring can list
it: usually the month of its third monthly or quarterly charge, fourth
weekly charge or second yearly charge, later when its amount changed too
often before then. A recurring charge that starts again after a charge
off schedule is new only if it had ended first, with no charge for 14
days (weekly), 45 days (monthly), 120 days (quarterly) or 400 days
(yearly). Recurring charges are judged as of the month's last day, so
later charges never change a past month's summary.

Net worth is what quarry networth lists at the two month ends, for
example quarry networth --since 2026-08 --until 2026-09; Change is the
difference. In CAD or USD it includes changes in the exchange rate.

Findings counts the findings open in the store; new and fixed are what
the last sync found, whenever it ran.

Months begin and end at midnight in this Mac's time zone. Amounts are in
CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
With --currency native, CAD and USD are listed separately, never added
together.
`
	var stdout, stderr bytes.Buffer

	err := executeSummary(t, fakeReportStore{}, cadConfig, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_summary_help_shows_examples(t *testing.T) {
	const examples = `Examples:
  quarry summary
  quarry summary --month 2026-08 --currency USD
  quarry summary --json
`
	var stdout, stderr bytes.Buffer

	err := executeSummary(t, fakeReportStore{}, cadConfig, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), examples)
}

func Test_summary_help_shows_the_month_flag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSummary(t, fakeReportStore{}, cadConfig, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Regexp(t, `(?m)^ +--month YYYY-MM +summarize month YYYY-MM instead of last month; it must have ended$`, stdout.String())
}

func Test_summary_defaults_to_last_month_in_the_configs_currency(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var got store.SummaryParams
	usd := func(string) (config.Config, error) { return config.Config{Currency: money.USD}, nil }

	err := executeSummary(t, fakeReportStore{gotSummary: &got}, usd, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), got.Through)
	assert.Contains(t, stdout.String(), "Summary of August 2026 (2026-08-01 to 2026-08-31), amounts in USD\n")
}

func Test_summary_reads_the_month_it_was_asked_for_in_the_currency_it_was_asked_for(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var got store.SummaryParams

	err := executeSummary(t, fakeReportStore{gotSummary: &got}, cadConfig, &stdout, &stderr, "--month", "2026-07", "--currency", "USD")

	require.NoError(t, err)
	assert.Equal(t, store.SummaryParams{
		Through: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		Dates:   []time.Time{time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)},
	}, got)
	assert.Contains(t, stdout.String(), "Summary of July 2026 (2026-07-01 to 2026-07-31), amounts in USD\n")
}

func Test_summary_warns_and_prints_when_the_config_is_unreadable_and_currency_is_given(t *testing.T) {
	var stdout, stderr bytes.Buffer
	unreadable := func(string) (config.Config, error) { return config.Config{}, errConfigRead }

	err := executeSummary(t, fakeReportStore{}, unreadable, &stdout, &stderr, "--currency", "USD")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: cannot tell which findings you ignored or how you classified your accounts: "+
		"cannot read config.toml: permission denied; findings you ignored are counted as open, "+
		"and every investment account is counted as unclassified\n", stderr.String())
	assert.Contains(t, stdout.String(), "Summary of August 2026 (2026-08-01 to 2026-08-31), amounts in USD\n")
}

func Test_summary_refuses_an_unreadable_config_without_the_currency_flag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	unreadable := func(string) (config.Config, error) { return config.Config{}, errConfigRead }

	err := executeSummary(t, fakeReportStore{}, unreadable, &stdout, &stderr)

	require.ErrorIs(t, err, errConfigRead)
	assert.NotErrorAs(t, err, new(cli.UsageError))
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_summary_reads_the_config_once_even_with_the_currency_flag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var asked []string
	load := func(name string) (config.Config, error) {
		asked = append(asked, name)

		return cadConfig(name)
	}

	err := executeSummary(t, fakeReportStore{}, load, &stdout, &stderr, "--currency", "USD")

	require.NoError(t, err)
	assert.Equal(t, []string{"summary"}, asked)
}

func Test_summary_prints_the_configs_warnings_on_stderr_with_or_without_the_currency_flag(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "without the flag", args: nil},
		{name: "with the flag", args: []string{"--currency", "CAD"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSummary(t, fakeReportStore{}, warningConfig, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Equal(t, "quarry: warning: "+unknownKeyShown+"\n", stderr.String())
			assert.Contains(t, stdout.String(), "Summary of August 2026")
		})
	}
}

func Test_summary_returns_the_fault_of_opening_the_store_as_a_runtime_error(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"summary"}, refusedEnv(&stdout, &stderr))

	require.ErrorIs(t, err, errStoreRead)
	assert.NotErrorAs(t, err, new(cli.UsageError))
	assert.Empty(t, stdout.String())
}

func Test_summary_returns_the_report_fault_as_a_runtime_error(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSummary(t, fakeReportStore{err: errStoreRead}, cadConfig, &stdout, &stderr)

	require.ErrorIs(t, err, errStoreRead)
	assert.NotErrorAs(t, err, new(cli.UsageError))
	assert.Empty(t, stdout.String())
}

func Test_summary_refuses_a_failed_write_to_stdout(t *testing.T) {
	var stderr bytes.Buffer

	err := executeSummary(t, fakeReportStore{}, cadConfig, failingWriter{err: errNoSpace}, &stderr)

	require.ErrorIs(t, err, errNoSpace)
	assert.ErrorContains(t, err, "cannot write the result to stdout")
}

// summaryTakenAt is a store whose snapshot was taken at taken.
func summaryTakenAt(taken time.Time) fakeReportStore {
	return fakeReportStore{summary: store.Summary{Status: store.Status{Run: store.ImportRun{Snapshot: store.SnapshotRef{TakenAt: taken}}}}}
}

func Test_summary_ends_its_snapshot_warning_with_the_command_that_repeats_the_summary_asked_for(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		taken time.Time
		want  string
	}{
		{
			name:  "the default month",
			taken: time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC),
			want: "quarry: warning: the store was built from a snapshot taken 2026-08-30 12:00 UTC, before August 2026 ended, " +
				"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, " +
				"then run quarry summary again\n",
		},
		{
			name:  "a month named on the command line",
			args:  []string{"--month", "2026-07"},
			taken: time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC),
			want: "quarry: warning: the store was built from a snapshot taken 2026-07-30 12:00 UTC, before July 2026 ended, " +
				"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, " +
				"then run quarry summary --month 2026-07 again\n",
		},
		{
			name:  "the default month named anyway",
			args:  []string{"--month", "2026-08"},
			taken: time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC),
			want: "quarry: warning: the store was built from a snapshot taken 2026-08-30 12:00 UTC, before August 2026 ended, " +
				"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, " +
				"then run quarry summary --month 2026-08 again\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSummary(t, summaryTakenAt(c.taken), cadConfig, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Equal(t, c.want, stderr.String())
		})
	}
}

func Test_summary_says_nothing_about_a_snapshot_taken_when_the_month_ended(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSummary(t, summaryTakenAt(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)), cadConfig, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Summary of August 2026")
}

func Test_summary_warns_about_the_snapshot_after_it_warns_about_the_config(t *testing.T) {
	var stdout, stderr bytes.Buffer
	unreadable := func(string) (config.Config, error) { return config.Config{}, errConfigRead }

	err := executeSummary(t, summaryTakenAt(time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)), unreadable, &stdout, &stderr, "--currency", "USD")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: cannot tell which findings you ignored or how you classified your accounts: "+
		"cannot read config.toml: permission denied; findings you ignored are counted as open, "+
		"and every investment account is counted as unclassified\n"+
		"quarry: warning: the store was built from a snapshot taken 2026-08-30 12:00 UTC, before August 2026 ended, "+
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, "+
		"then run quarry summary again\n", stderr.String())
}

func Test_summary_does_not_warn_about_the_snapshot_when_the_summary_cannot_be_written(t *testing.T) {
	var stderr bytes.Buffer

	err := executeSummary(t, summaryTakenAt(time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)), cadConfig, failingWriter{err: errNoSpace}, &stderr)

	require.ErrorIs(t, err, errNoSpace)
	assert.Empty(t, stderr.String())
}

func Test_summary_reads_the_clock_once(t *testing.T) {
	var stdout bytes.Buffer
	taken := spendNow.Add(-3 * 24 * time.Hour)
	fake := fakeReportStore{summary: store.Summary{Status: store.Status{Run: store.ImportRun{
		Snapshot: store.SnapshotRef{Path: "/snapshots/20260926T120000Z.sqlite", TakenAt: taken},
	}}}}

	err := executeAtAdvancingClock(t, "summary", fake, &stdout)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Snapshot  20260926T120000Z, taken ")
	assert.Contains(t, stdout.String(), " (3 days ago)\n")
}

// findingsFixture builds the findings of one store: open of them (the first openNew found by the latest
// build), ignored more named in the returned ignore list, and fixed (the first newlyFixed fixed by it).
func findingsFixture(open, openNew, ignored, fixed, newlyFixed int) ([]store.Finding, []string) {
	var findings []store.Finding
	ignore := make([]string, 0, ignored)
	add := func(prefix string, n int, edit func(i int, f *store.Finding)) {
		for i := range n {
			f := store.Finding{ID: "duplicate:" + prefix + strconv.Itoa(i), Type: finding.Duplicate}
			edit(i, &f)
			findings = append(findings, f)
		}
	}
	fixedAt := new(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	add("open", open, func(i int, f *store.Finding) { f.New = i < openNew })
	add("ignored", ignored, func(int, *store.Finding) {})
	add("fixed", fixed, func(i int, f *store.Finding) { f.FixedAt, f.NewlyFixed = fixedAt, i < newlyFixed })
	for i := range ignored {
		ignore = append(ignore, "duplicate:ignored"+strconv.Itoa(i))
	}
	return findings, ignore
}

func Test_summary_findings_row_names_the_last_sync(t *testing.T) {
	cases := []struct {
		name                                      string
		open, openNew, ignored, fixed, newlyFixed int
		want                                      string
	}{
		{name: "none open and nothing found", want: "none open"},
		{name: "open, nothing new or fixed", open: 2, want: "2 open; run quarry findings to list them"},
		{name: "open count grouped by thousands", open: 1234, want: "1,234 open; run quarry findings to list them"},
		{name: "new and fixed", open: 4, openNew: 3, fixed: 2, newlyFixed: 2, want: "4 open; the last sync found 3 new and 2 fixed; run quarry findings to list them"},
		{name: "new only", open: 2, openNew: 1, want: "2 open; the last sync found 1 new; run quarry findings to list them"},
		{name: "fixed only", open: 2, fixed: 3, newlyFixed: 1, want: "2 open; the last sync found 1 fixed; run quarry findings to list them"},
		{name: "fixed in an earlier sync is not mentioned", open: 2, fixed: 3, want: "2 open; run quarry findings to list them"},
		{name: "ignored", open: 3, ignored: 5, want: "3 open, 5 ignored; run quarry findings to list them"},
		{name: "ignored, new and fixed", open: 4, openNew: 3, ignored: 5, fixed: 2, newlyFixed: 2, want: "4 open, 5 ignored; the last sync found 3 new and 2 fixed; run quarry findings to list them"},
		{name: "ignored count grouped by thousands", open: 1, ignored: 1234, want: "1 open, 1,234 ignored; run quarry findings to list them"},
		{
			name: "new and fixed counts grouped by thousands", open: 1234, openNew: 1234, fixed: 1234, newlyFixed: 1234,
			want: "1,234 open; the last sync found 1,234 new and 1,234 fixed; run quarry findings to list them",
		},
		{name: "new count grouped by thousands", open: 1234, openNew: 1234, want: "1,234 open; the last sync found 1,234 new; run quarry findings to list them"},
		{name: "fixed count grouped by thousands", open: 1, fixed: 1234, newlyFixed: 1234, want: "1 open; the last sync found 1,234 fixed; run quarry findings to list them"},
		{name: "none open, some ignored", ignored: 2, want: "none open, 2 ignored"},
		{name: "none open, some fixed by the last sync", fixed: 3, newlyFixed: 2, want: "none open; the last sync found 2 fixed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings, ignore := findingsFixture(c.open, c.openNew, c.ignored, c.fixed, c.newlyFixed)
			fake := fakeReportStore{summary: store.Summary{Status: store.Status{Findings: findings}}}
			load := func(string) (config.Config, error) { return config.Config{Currency: money.CAD, Ignore: ignore}, nil }
			var stdout, stderr bytes.Buffer

			err := executeSummary(t, fake, load, &stdout, &stderr)

			require.NoError(t, err)
			assert.Contains(t, stdout.String(), "\nFindings  "+c.want+"\n\n")
		})
	}
}

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
