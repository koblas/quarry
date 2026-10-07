package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// netWorthNoRatesLine is the net-worth warning for a store with no exchange rates, in a CAD summary.
const netWorthNoRatesLine = "the store has no exchange rates, so USD balances are not converted to CAD and are left out of the CAD total; " +
	"pass --currency native to list them, or run quarry sync to fetch rates"

func Test_run_summary_warns_once_per_kind_when_the_store_has_no_exchange_rates(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, summaryNativeRows())

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"summary"}, summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), anomaliesTable("Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD", "4 charges checked",
		[]string{"2026-09-14", "US Chequing (USD)", "Hulu", "Food:Groceries", "USD 150.00", "USD 20.00", "7.5x", "payee, 5 earlier"},
		[]string{"2026-09-14", "Chequing (CAD)", "Bell Canada", "Food:Groceries", "412.00", "96.05", "4.3x", "payee, 5 earlier"}))
	assert.Contains(t, stdout.String(), recurringTable("Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD",
		[]string{"Crave", "CAD", "month", "22.59", "271.08", "2026-07-03", "2026-09-03", "active", ""},
		[]string{"Spotify", "USD", "month", "9.99", "119.88", "2026-07-08", "2026-09-08", "active", ""},
		[]string{"Total", "CAD", "", "", "271.08", "", "", "", ""},
		[]string{"Total", "USD", "", "", "119.88", "", "", "", ""}))
	assert.Contains(t, stdout.String(), netWorthHistoryLine("Change", "no rate", "-22.59", "no rate"))
	assert.Equal(t, septemberTimeUnknownWarning+warningLine(noRatesLine)+warningLine(netWorthNoRatesLine), stderr.String())
}

const (
	chargeBeforeFirstRateLine = "1 charge dated before 2026-10-02, the first exchange rate in the store, is listed in USD, not converted to CAD"
	seriesBeforeFirstRateLine = "1 series with a charge dated before 2026-10-02, the first exchange rate in the store, " +
		"is listed in USD, not converted to CAD"
	monthEndsBeforeFirstRateLine = "USD balances on 2 month ends before 2026-10-02, the first exchange rate in the store, " +
		"are not converted to CAD and are left out of the CAD total; pass --currency native to list them"
	colourWarningTilde = "~/Library/Application Support/quarry/config.toml: unknown key colour; quarry ignores it"
	unreadableConfigW2 = "cannot tell which findings you ignored or how you classified your accounts: " +
		"~/Library/Application Support/quarry/config.toml: reporting.currency must be CAD, USD or native, got \"EUR\"; " +
		"findings you ignored are counted as open, and every investment account is counted as unclassified"
)

// septemberTimeUnknownText is septemberTimeUnknownWarning as --json's warnings carry it: without the stderr prefix and newline.
func septemberTimeUnknownText() string {
	return strings.TrimSuffix(strings.TrimPrefix(septemberTimeUnknownWarning, "quarry: warning: "), "\n")
}

// runSummaryText runs quarry summary with args at summaryClock and returns its stdout and stderr.
func runSummaryText(t *testing.T, args ...string) (string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"summary"}, args...), summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

// seedNativeSummaryStore stores summaryNativeRows, which holds no exchange rates, under a fresh HOME.
func seedNativeSummaryStore(t *testing.T) string {
	t.Helper()
	home := newHome(t)
	replaceStore(t, home, summaryNativeRows())
	return home
}

// seedRatedNativeSummaryStore stores summaryNativeRows with a USD rate first dated October 2, after both month ends.
func seedRatedNativeSummaryStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	replaceStoreWithRates(t, home, summaryNativeRows(), usdRate(day(2026, time.October, 2), 1_350_000))
}

// usdBalanceOnlyRows holds one USD deposit in January: a USD balance on both month ends, no charge, no series.
func usdBalanceOnlyRows() store.Rows {
	usd := usdChequingAccount("acct-usd", 1)
	return chargeRows([]store.Account{usd}, chargeTxn{
		id: "usd-deposit", account: usd.ID, payee: "Employer", currency: "USD", day: day(2026, time.January, 2),
		splits: []chargeSplit{{category: "cat-fuel", cents: 100_000}},
	})
}

func Test_run_summary_json_lists_the_missing_rate_warnings_after_the_snapshot_warning_and_leaves_the_cells_null(t *testing.T) {
	seedNativeSummaryStore(t)

	doc, _, stderr := runSummaryJSON(t, summaryClock)

	assert.Equal(t, []string{septemberTimeUnknownText(), noRatesLine, netWorthNoRatesLine}, doc.Warnings)
	assert.Equal(t, []summaryChangeTypeJSON{
		{Type: "chequing", Currency: "CAD"},
		{Type: "credit_card", Currency: "CAD", Value: new("-22.59")},
	}, doc.NetWorth.Changes.Types)
	assert.Equal(t, []summaryMoneyJSON{{Currency: "CAD"}}, doc.NetWorth.Changes.Totals)
	assert.Equal(t, warningLines(doc.Warnings), stderr)
}

func Test_run_summary_prints_the_same_warnings_on_stderr_with_and_without_json(t *testing.T) {
	seedNativeSummaryStore(t)
	_, plain := runSummaryText(t)

	_, _, flagged := runSummaryJSON(t, summaryClock)

	assert.Equal(t, septemberTimeUnknownWarning+warningLine(noRatesLine)+warningLine(netWorthNoRatesLine), plain)
	assert.Equal(t, plain, flagged)
}

func Test_run_summary_warns_about_each_kind_dated_before_the_first_rate(t *testing.T) {
	seedRatedNativeSummaryStore(t)

	stdout, stderr := runSummaryText(t)

	assert.Contains(t, stdout, netWorthHistoryLine("Change", "no rate", "-22.59", "no rate"))
	assert.Equal(t, septemberTimeUnknownWarning+warningLine(chargeBeforeFirstRateLine)+warningLine(seriesBeforeFirstRateLine)+
		warningLine(monthEndsBeforeFirstRateLine), stderr)
}

func Test_run_summary_json_lists_each_kind_dated_before_the_first_rate_and_leaves_the_cells_null(t *testing.T) {
	seedRatedNativeSummaryStore(t)

	doc, _, stderr := runSummaryJSON(t, summaryClock)

	assert.Equal(t, []string{septemberTimeUnknownText(), chargeBeforeFirstRateLine, seriesBeforeFirstRateLine, monthEndsBeforeFirstRateLine}, doc.Warnings)
	assert.Equal(t, []summaryMoneyJSON{{Currency: "CAD"}}, doc.NetWorth.Changes.Totals)
	assert.Equal(t, warningLines(doc.Warnings), stderr)
}

func Test_run_summary_warns_only_about_the_month_ends_when_no_charge_or_series_needs_a_rate(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, usdBalanceOnlyRows(), usdRate(day(2026, time.October, 2), 1_350_000))

	stdout, stderr := runSummaryText(t)

	assert.Contains(t, stdout, "No unusually large charges.")
	assert.Contains(t, stdout, "No new recurring charges.")
	assert.Contains(t, stdout, "Change       no rate  no rate\n")
	assert.Equal(t, septemberTimeUnknownWarning+warningLine(monthEndsBeforeFirstRateLine), stderr)
}

func Test_run_summary_json_lists_only_the_month_end_warning_when_no_charge_or_series_needs_a_rate(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, usdBalanceOnlyRows(), usdRate(day(2026, time.October, 2), 1_350_000))

	doc, _, _ := runSummaryJSON(t, summaryClock)

	assert.Equal(t, []string{septemberTimeUnknownText(), monthEndsBeforeFirstRateLine}, doc.Warnings)
	assert.Equal(t, []summaryChangeTypeJSON{{Type: "chequing", Currency: "CAD"}}, doc.NetWorth.Changes.Types)
	assert.Equal(t, []summaryMoneyJSON{{Currency: "CAD"}}, doc.NetWorth.Changes.Totals)
}

func Test_run_summary_prints_a_config_warning_before_the_snapshot_and_rate_warnings(t *testing.T) {
	home := seedNativeSummaryStore(t)
	writeConfig(t, home, "colour = \"red\"\n")

	_, stderr := runSummaryText(t)

	assert.Equal(t, warningLine(colourWarningTilde)+septemberTimeUnknownWarning+warningLine(noRatesLine)+warningLine(netWorthNoRatesLine), stderr)
}

func Test_run_summary_json_lists_a_config_warning_before_the_snapshot_and_rate_warnings(t *testing.T) {
	home := seedNativeSummaryStore(t)
	writeConfig(t, home, "colour = \"red\"\n")

	doc, _, _ := runSummaryJSON(t, summaryClock)

	assert.Equal(t, []string{
		configPath(home) + ": unknown key colour; quarry ignores it", septemberTimeUnknownText(), noRatesLine, netWorthNoRatesLine,
	}, doc.Warnings)
}

func Test_run_summary_prints_the_cannot_tell_warning_before_the_snapshot_and_rate_warnings(t *testing.T) {
	home := seedNativeSummaryStore(t)
	writeConfig(t, home, "[reporting]\ncurrency = \"EUR\"\n")

	_, stderr := runSummaryText(t, "--currency", "CAD")

	assert.Equal(t, warningLine(unreadableConfigW2)+septemberTimeUnknownWarning+warningLine(noRatesLine)+warningLine(netWorthNoRatesLine), stderr)
}

func Test_run_summary_json_lists_the_cannot_tell_warning_before_the_snapshot_and_rate_warnings(t *testing.T) {
	home := seedNativeSummaryStore(t)
	writeConfig(t, home, "[reporting]\ncurrency = \"EUR\"\n")

	doc, _, _ := runSummaryJSON(t, summaryClock, "--currency", "CAD")

	require.Len(t, doc.Warnings, 4)
	assert.Equal(t, "cannot tell which findings you ignored or how you classified your accounts: "+configPath(home)+
		": reporting.currency must be CAD, USD or native, got \"EUR\"; findings you ignored are counted as open, "+
		"and every investment account is counted as unclassified", doc.Warnings[0])
	assert.Equal(t, []string{septemberTimeUnknownText(), noRatesLine, netWorthNoRatesLine}, doc.Warnings[1:])
}

func Test_run_summary_native_prints_no_rate_warning(t *testing.T) {
	seedNativeSummaryStore(t)

	_, stderr := runSummaryText(t, "--currency", "native")

	assert.Equal(t, septemberTimeUnknownWarning, stderr)
}

func Test_run_summary_json_native_lists_no_rate_warning(t *testing.T) {
	seedNativeSummaryStore(t)

	doc, _, _ := runSummaryJSON(t, summaryClock, "--currency", "native")

	assert.Equal(t, []string{septemberTimeUnknownText()}, doc.Warnings)
}

// siblingStderr is the stderr of quarry args at summaryClock.
func siblingStderr(t *testing.T, args ...string) string {
	t.Helper()

	exitCode, _, stderr := runSpendCaptureAt(context.Background(), args, summaryClock)

	require.Equal(t, 0, exitCode, stderr.String())
	return stderr.String()
}

func Test_run_summary_in_usd_prints_the_lines_anomalies_and_networth_print_for_the_same_store(t *testing.T) {
	seedNativeSummaryStore(t)
	anomalies := siblingStderr(t, "anomalies", "--since", "2026-09", "--until", "2026-09", "--currency", "USD")
	netWorth := siblingStderr(t, "networth", "--since", "2026-08", "--until", "2026-09", "--currency", "USD")
	require.Contains(t, anomalies, "listed in each account's own currency")
	require.Contains(t, netWorth, "CAD balances are not converted to USD")

	_, stderr := runSummaryText(t, "--currency", "USD")

	assert.Equal(t, septemberTimeUnknownWarning+anomalies+netWorth, stderr)
}

func Test_run_summary_json_in_usd_lists_the_lines_stderr_prints(t *testing.T) {
	seedNativeSummaryStore(t)

	doc, _, stderr := runSummaryJSON(t, summaryClock, "--currency", "USD")

	assert.Equal(t, "USD", doc.Currency)
	require.Len(t, doc.Warnings, 3)
	assert.Equal(t, warningLines(doc.Warnings), stderr)
}
