// White-box: summaryHeading, summaryFindingsPhrase and renderSummary are unexported layout rules; the ignored
// clause of an unread ignore list cannot be reached through the command, which counts no ignored finding then.
package cli

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// septemberSummary is an empty summary of September 2026 in currency.
func septemberSummary(currency money.Currency) report.Summary {
	start := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	return report.Summary{
		Month:    report.Month{Start: start, End: time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)},
		Currency: currency,
		NetWorth: report.NetWorth{Currency: currency},
	}
}

func Test_summaryHeading_names_the_currency_unless_amounts_stay_native(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     string
	}{
		{name: "CAD", currency: money.CAD, want: "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD"},
		{name: "USD", currency: money.USD, want: "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in USD"},
		{name: "native adds no currency", currency: money.Native, want: "Summary of September 2026 (2026-09-01 to 2026-09-30)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, summaryHeading(septemberSummary(c.currency)))
		})
	}
}

func Test_summaryFindingsPhrase_says_ignored_only_when_the_ignore_list_was_read(t *testing.T) {
	cases := []struct {
		name        string
		ignoreKnown bool
		want        string
	}{
		{name: "ignore list read", ignoreKnown: true, want: "3 open, 2 ignored; run quarry findings to list them"},
		{name: "ignore list unread", ignoreKnown: false, want: "3 open; run quarry findings to list them"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tally := document.FindingsTally{Counts: finding.Counts{Open: 3, Ignored: 2}, IgnoreKnown: c.ignoreKnown}

			assert.Equal(t, c.want, summaryFindingsPhrase(tally))
		})
	}
}

func Test_renderSummary_says_when_the_snapshot_manifest_has_no_time(t *testing.T) {
	s := septemberSummary(money.CAD)
	s.Status.Run.Snapshot = store.SnapshotRef{Path: "/snapshots/20260928T140200Z.sqlite"}

	got := renderSummary(s, document.FindingsTally{IgnoreKnown: true}, time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC))

	assert.Contains(t, got, "\nSnapshot  20260928T140200Z, time taken not recorded in its manifest\n")
}

func Test_renderSummary_says_when_the_store_holds_no_transactions(t *testing.T) {
	s := septemberSummary(money.CAD)

	got := renderSummary(s, document.FindingsTally{IgnoreKnown: true}, time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC))

	assert.Contains(t, got, "\nDates     no transactions\nFindings  none open\n\n")
}

var (
	augustEnd    = time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC)
	septemberEnd = time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)
)

// monthEndRow is a month-end row of accountType in currency worth cents, which converts to CAD at par.
func monthEndRow(accountType, currency string, cents int64) store.NetWorthRow {
	balance := big.NewInt(cents)
	return store.NetWorthRow{Type: accountType, Currency: currency, Accounts: 1, Balance: balance, BalanceCAD: balance}
}

// monthEndUnratedRow is monthEndRow without the exchange rate that would convert it.
func monthEndUnratedRow(accountType, currency string, cents int64) store.NetWorthRow {
	return store.NetWorthRow{Type: accountType, Currency: currency, Accounts: 1, Balance: big.NewInt(cents)}
}

// monthEndHolding is day's net worth: its rows and their totals.
func monthEndHolding(day time.Time, rows []store.NetWorthRow, totals ...report.NetWorthTotal) report.NetWorthDate {
	return report.NetWorthDate{Date: day, Rows: rows, Totals: totals}
}

func Test_signedMoney_signs_a_nonzero_amount_and_leaves_zero_bare(t *testing.T) {
	cases := []struct {
		name  string
		cents *big.Int
		want  string
	}{
		{name: "positive gets a plus", cents: big.NewInt(618_643), want: "+6,186.43"},
		{name: "negative keeps its minus", cents: big.NewInt(-21_960), want: "-219.60"},
		{name: "zero is unsigned", cents: big.NewInt(0), want: "0.00"},
		{name: "thousands are grouped", cents: big.NewInt(100_000), want: "+1,000.00"},
		{name: "a missing rate says so", cents: nil, want: "no rate"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, signedMoney(c.cents))
		})
	}
}

func Test_changeRows_lists_a_signed_cell_for_each_type_and_the_total_when_converted(t *testing.T) {
	n := report.NetWorth{Currency: money.CAD, Dates: []report.NetWorthDate{
		monthEndHolding(augustEnd, []store.NetWorthRow{
			monthEndRow("chequing", "CAD", 100_000), monthEndRow("credit_card", "CAD", -10_000), monthEndRow("savings", "CAD", 50_000),
		}, report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(140_000)}),
		monthEndHolding(septemberEnd, []store.NetWorthRow{
			monthEndRow("chequing", "CAD", 125_050), monthEndRow("credit_card", "CAD", -15_025), monthEndRow("savings", "CAD", 50_000),
		}, report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(160_025)}),
	}}

	rows := changeRows(n, n.Change())

	assert.Equal(t, [][]string{{"Change", "+250.50", "-50.25", "0.00", "+200.25"}}, rows)
}

func Test_changeRows_says_no_rate_for_a_type_and_a_total_a_missing_rate_leaves_out(t *testing.T) {
	n := report.NetWorth{Currency: money.CAD, Dates: []report.NetWorthDate{
		monthEndHolding(augustEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 100_000), monthEndUnratedRow("brokerage", "USD", 40_000)},
			report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(100_000)}, report.NetWorthTotal{Currency: "USD", Value: big.NewInt(40_000)}),
		monthEndHolding(septemberEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 125_050), monthEndUnratedRow("brokerage", "USD", 50_000)},
			report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(125_050)}, report.NetWorthTotal{Currency: "USD", Value: big.NewInt(50_000)}),
	}}

	rows := changeRows(n, n.Change())

	assert.Equal(t, [][]string{{"Change", "no rate", "+250.50", "no rate"}}, rows)
}

func Test_changeRows_lists_a_currency_held_on_the_end_day_only_when_native(t *testing.T) {
	n := report.NetWorth{Currency: money.Native, Dates: []report.NetWorthDate{
		monthEndHolding(augustEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 100_000), monthEndRow("credit_card", "CAD", -10_000)},
			report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(90_000)}),
		monthEndHolding(septemberEnd, []store.NetWorthRow{
			monthEndRow("chequing", "CAD", 120_000), monthEndRow("credit_card", "CAD", -10_000),
			monthEndRow("chequing", "USD", 150_000), monthEndRow("credit_card", "USD", -20_000),
		}, report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(110_000)}, report.NetWorthTotal{Currency: "USD", Value: big.NewInt(130_000)}),
	}}

	rows := changeRows(n, n.Change())

	assert.Equal(t, [][]string{
		{"Change", "CAD", "+200.00", "0.00", "+200.00"},
		{"Change", "USD", "+1,500.00", "-200.00", "+1,300.00"},
	}, rows)
}

func Test_changeRows_leaves_a_type_blank_in_a_currency_that_never_held_it_when_native(t *testing.T) {
	n := report.NetWorth{Currency: money.Native, Dates: []report.NetWorthDate{
		monthEndHolding(augustEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 100_000), monthEndRow("credit_card", "USD", -10_000)},
			report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(100_000)}, report.NetWorthTotal{Currency: "USD", Value: big.NewInt(-10_000)}),
		monthEndHolding(septemberEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 120_000), monthEndRow("credit_card", "USD", -15_000)},
			report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(120_000)}, report.NetWorthTotal{Currency: "USD", Value: big.NewInt(-15_000)}),
	}}

	rows := changeRows(n, n.Change())

	assert.Equal(t, [][]string{
		{"Change", "CAD", "+200.00", "", "+200.00"},
		{"Change", "USD", "", "-50.00", "-50.00"},
	}, rows)
}

func Test_renderNetWorthWithChange_adds_no_line_when_the_first_month_end_has_no_balance(t *testing.T) {
	n := report.NetWorth{Currency: money.CAD, Dates: []report.NetWorthDate{
		monthEndHolding(augustEnd, nil),
		monthEndHolding(septemberEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 125_050)},
			report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(125_050)}),
	}}

	got := renderNetWorthWithChange(n)

	assert.Contains(t, got, "2026-09-30")
	assert.NotContains(t, got, "Change")
}
