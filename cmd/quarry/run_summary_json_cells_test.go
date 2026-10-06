package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runSummaryJSON runs quarry summary --json with args at now and returns its decoded document, stdout and stderr.
func runSummaryJSON(t *testing.T, now time.Time, args ...string) (summaryJSONDoc, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), append([]string{"summary", "--json"}, args...), spendEnvAt(&stdout, &stderr, now))

	require.Equal(t, 0, exitCode, stderr.String())
	return decodeSummaryJSON(t, stdout.String()), stdout.String(), stderr.String()
}

// compactField is the value at path in the JSON object stdout, compacted, so two commands' copies compare byte for byte.
func compactField(t *testing.T, stdout string, path ...string) string {
	t.Helper()
	raw := json.RawMessage(stdout)
	for _, key := range path {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		raw = fields[key]
	}
	var out bytes.Buffer

	require.NoError(t, json.Compact(&out, raw))
	return out.String()
}

// warningLine is the stderr line quarry prints for warning.
func warningLine(warning string) string { return "quarry: warning: " + warning + "\n" }

func Test_run_summary_json_writes_empty_arrays_and_nulls_for_the_first_month_of_data(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, firstMonthRows())

	doc, _, stderr := runSummaryJSON(t, summaryClock, "--month", "2026-09")

	assert.Equal(t, []anomalyJSON{}, doc.Anomalies.Charges)
	assert.Equal(t, []recurringSeriesJSON{}, doc.Recurring.Series)
	assert.Equal(t, []recurringTotalJSON{}, doc.Recurring.Totals)
	assert.Equal(t, summaryChangesJSON{Types: []summaryChangeTypeJSON{}, Totals: []summaryMoneyJSON{}}, doc.NetWorth.Changes)
	require.Len(t, doc.NetWorth.Dates, 2)
	assert.Equal(t, []summaryBalanceJSON{}, doc.NetWorth.Dates[0].Balances)
	assert.Equal(t, []summaryMoneyJSON{}, doc.NetWorth.Dates[0].Totals)
	assert.Len(t, doc.NetWorth.Dates[1].Balances, 1)
	assert.Equal(t, summarySnapshotJSON{ID: "20260929T000000Z"}, doc.Snapshot)
	assert.Equal(t, []string{
		"cannot tell whether the store holds all of September 2026: its snapshot's manifest does not record when it was taken; " +
			"open your Quicken file and run quarry sync to take a new snapshot",
	}, doc.Warnings)
	assert.Equal(t, septemberTimeUnknownWarning, stderr)
}

func Test_run_summary_json_writes_null_dates_for_a_store_without_transactions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}))

	doc, _, _ := runSummaryJSON(t, summaryClock)

	assert.Equal(t, summaryDatesJSON{}, doc.Dates)
	assert.Equal(t, summaryChangesJSON{Types: []summaryChangeTypeJSON{}, Totals: []summaryMoneyJSON{}}, doc.NetWorth.Changes)
}

func Test_run_summary_json_lists_each_currency_of_a_native_summary_separately(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, summaryNativeRows())

	doc, _, _ := runSummaryJSON(t, summaryClock, "--currency", "native")

	assert.Equal(t, "native", doc.Currency)
	assert.Equal(t, summaryChangesJSON{
		Types: []summaryChangeTypeJSON{
			{Type: "chequing", Currency: "CAD", Value: new("-412.00")},
			{Type: "credit_card", Currency: "CAD", Value: new("-22.59")},
			{Type: "chequing", Currency: "USD", Value: new("-159.99")},
		},
		Totals: []summaryMoneyJSON{
			{Currency: "CAD", Value: new("-434.59")},
			{Currency: "USD", Value: new("-159.99")},
		},
	}, doc.NetWorth.Changes)
}

func Test_run_summary_json_says_a_snapshot_taken_before_the_month_ended_does_not_cover_it(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pinLocalZone(t)
	rows := summaryRows(true)
	rows.ImportRuns[0].Snapshot.TakenAt = time.Date(2026, time.September, 28, 14, 2, 0, 0, time.FixedZone("EDT", -4*60*60))
	replaceStore(t, home, rows)

	doc, _, stderr := runSummaryJSON(t, summaryClockEDT())

	assert.Equal(t, summarySnapshotJSON{ID: "20261001T130512Z", TakenAt: new("2026-09-28T18:02:00Z"), CoversMonth: new(false)}, doc.Snapshot)
	require.NotEmpty(t, doc.Warnings)
	assert.Equal(t, warningLine(doc.Warnings[len(doc.Warnings)-1]), stderr)
}

func Test_run_summary_json_names_the_config_by_its_absolute_path_while_stderr_shows_it_with_a_tilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, summaryRows(true))
	writeConfig(t, home, "[reporting]\ncurrency = \"EUR\"\n")

	doc, _, stderr := runSummaryJSON(t, summaryClock, "--currency", "USD")

	assert.Equal(t, "USD", doc.Currency)
	assert.Nil(t, doc.Findings.Ignored)
	require.NotEmpty(t, doc.Warnings)
	assert.Equal(t, "cannot tell which findings you ignored or how you classified your accounts: "+configPath(home)+
		": reporting.currency must be CAD, USD or native, got \"EUR\"; findings you ignored are counted as open, "+
		"and every investment account is counted as unclassified", doc.Warnings[0])
	assert.Equal(t, "quarry: warning: cannot tell which findings you ignored or how you classified your accounts: "+
		"~/Library/Application Support/quarry/config.toml: reporting.currency must be CAD, USD or native, got \"EUR\"; "+
		"findings you ignored are counted as open, and every investment account is counted as unclassified\n", stderr)
}

func Test_run_summary_json_names_a_config_warning_by_its_absolute_path_while_stderr_shows_it_with_a_tilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, summaryRows(true))
	writeConfig(t, home, "colour = \"red\"\n")

	doc, _, stderr := runSummaryJSON(t, summaryClock)

	require.NotEmpty(t, doc.Warnings)
	assert.Equal(t, configPath(home)+": unknown key colour; quarry ignores it", doc.Warnings[0])
	assert.Equal(t, "quarry: warning: ~/Library/Application Support/quarry/config.toml: unknown key colour; quarry ignores it\n", stderr)
}

func Test_run_summary_json_carries_the_charges_quarry_anomalies_lists_for_the_month(t *testing.T) {
	seedSummaryStore(t)
	pinLocalZone(t)
	_, summaryOut, _ := runSummaryJSON(t, summaryClock)
	var anomaliesOut, anomaliesErr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies", "--json", "--since", "2026-09", "--until", "2026-09"},
		spendEnvAt(&anomaliesOut, &anomaliesErr, summaryClock))

	require.Equal(t, 0, exitCode, anomaliesErr.String())
	assert.NotEqual(t, "[]", compactField(t, summaryOut, "anomalies", "charges"))
	assert.Equal(t, compactField(t, anomaliesOut.String(), "anomalies"), compactField(t, summaryOut, "anomalies", "charges"))
}

func Test_run_summary_json_carries_the_dates_quarry_networth_lists_for_the_two_month_ends(t *testing.T) {
	seedSummaryStore(t)
	pinLocalZone(t)
	_, summaryOut, _ := runSummaryJSON(t, summaryClock)
	var netWorthOut, netWorthErr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--json", "--since", "2026-08", "--until", "2026-09"},
		spendEnvAt(&netWorthOut, &netWorthErr, summaryClock))

	require.Equal(t, 0, exitCode, netWorthErr.String())
	assert.Equal(t, compactField(t, netWorthOut.String(), "dates"), compactField(t, summaryOut, "net_worth", "dates"))
}
