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

// septemberSummary is an empty summary of September 2026 in currency: both windows span the month and the
// net worth holds both month ends, as Server.Summary always returns them.
func septemberSummary(currency money.Currency) report.Summary {
	month := report.Month{
		Start: time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
	}
	window := store.Window{Since: month.Start, Until: month.End}
	return report.Summary{
		Month:     month,
		Currency:  currency,
		Anomalies: report.Anomalies{Window: window, Currency: currency},
		Recurring: report.Recurring{Window: window, Currency: currency},
		NetWorth: report.NetWorth{Currency: currency, Dates: []report.NetWorthDate{
			monthEndHolding(time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC), nil),
			monthEndHolding(month.End, nil),
		}},
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
	s.Status.Run.Snapshot = store.SnapshotRef{Path: "/snapshots/20260928T140200Z.sqlite"}

	got := renderSummary(s, document.FindingsTally{IgnoreKnown: true}, time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC))

	assert.Equal(t, "Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD\n\n"+
		"Snapshot  20260928T140200Z, time taken not recorded in its manifest\n"+
		"Dates     no transactions\n"+
		"Findings  none open\n\n"+
		"Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n"+
		"No unusually large charges.\n\n"+
		"0 charges checked\n\n"+
		"Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD\n\n"+
		"No new recurring charges.\n\n"+
		"Net worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD\n\n"+
		"No account has a balance on 2026-08-31 or 2026-09-30.\n", got)
}

func Test_summaryAnomaliesSection_says_none_when_no_charge_is_listed(t *testing.T) {
	cases := []struct {
		name string
		a    report.Anomalies
		want string
	}{
		{
			name: "no charge checked",
			a:    report.Anomalies{Window: spendingWindow(), Currency: money.CAD},
			want: "Unusually large charges 2026-01-01 to 2026-03-09 in all accounts, amounts in CAD\n\nNo unusually large charges.\n\n0 charges checked\n",
		},
		{
			name: "charges checked and some too young to judge",
			a:    report.Anomalies{Window: spendingWindow(), Currency: money.CAD, Checked: 412, NotJudged: 37},
			want: "Unusually large charges 2026-01-01 to 2026-03-09 in all accounts, amounts in CAD\n\nNo unusually large charges.\n\n" +
				"412 charges checked; 37 had too little history to judge\n",
		},
		{
			name: "native adds no amounts clause",
			a:    report.Anomalies{Window: spendingWindow(), Currency: money.Native, Checked: 3},
			want: "Unusually large charges 2026-01-01 to 2026-03-09 in all accounts\n\nNo unusually large charges.\n\n3 charges checked\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, summaryAnomaliesSection(c.a))
		})
	}
}

func Test_summaryAnomaliesSection_keeps_the_anomalies_table_when_a_charge_is_listed(t *testing.T) {
	a := listed(anomalyOf(new("Hydro"), nil, 0))
	a.Currency = money.CAD

	got := summaryAnomaliesSection(a)

	assert.Equal(t, renderAnomalies(a), got)
	assert.Contains(t, got, "\nDate        Account ")
	assert.NotContains(t, got, "No unusually large charges.")
}

func Test_summaryRecurringSection_says_none_when_no_series_is_new(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     string
	}{
		{name: "CAD", currency: money.CAD, want: "Recurring charges new 2026-01-01 to 2026-03-09 in all accounts, amounts in CAD\n\nNo new recurring charges.\n"},
		{name: "native adds no amounts clause", currency: money.Native, want: "Recurring charges new 2026-01-01 to 2026-03-09 in all accounts\n\nNo new recurring charges.\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, summaryRecurringSection(report.Recurring{Window: spendingWindow(), Currency: c.currency}))
		})
	}
}

func Test_summaryRecurringSection_keeps_the_recurring_table_when_a_series_is_new(t *testing.T) {
	r := report.Recurring{
		Window:   spendingWindow(),
		Currency: money.CAD,
		Series: []report.Series{{
			Payee: "Rogers", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceMonthly, Amount: 9500, PerYear: new(int64(114000)),
			First: recurringDay(time.February, 3), Last: recurringDay(time.March, 3), State: report.SeriesActive, New: true,
		}},
		Totals: []report.RecurringTotal{{Currency: "CAD", PerYear: 114000}},
	}

	got := summaryRecurringSection(r)

	assert.Equal(t, renderRecurringTitled(summaryRecurringTitle, r), got)
	assert.Contains(t, got, "\nPayee   Currency ")
	assert.NotContains(t, got, "No new recurring charges.")
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

func Test_renderNetWorthWithChange_says_no_account_has_a_balance_when_neither_month_end_has_one(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		caption  string
	}{
		{name: "CAD", currency: money.CAD, caption: "Net worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD"},
		{name: "USD", currency: money.USD, caption: "Net worth at each month end 2026-08-31 to 2026-09-30, amounts in USD"},
		{name: "native adds no amounts clause", currency: money.Native, caption: "Net worth at each month end 2026-08-31 to 2026-09-30"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := report.NetWorth{Currency: c.currency, Dates: []report.NetWorthDate{monthEndHolding(augustEnd, nil), monthEndHolding(septemberEnd, nil)}}

			assert.Equal(t, c.caption+"\n\nNo account has a balance on 2026-08-31 or 2026-09-30.\n", renderNetWorthWithChange(n))
		})
	}
}

func Test_renderNetWorthWithChange_says_no_change_is_shown_when_only_the_first_month_end_is_empty(t *testing.T) {
	cases := []struct {
		name string
		n    report.NetWorth
		want string
	}{
		{
			name: "CAD",
			n: report.NetWorth{Currency: money.CAD, Dates: []report.NetWorthDate{
				monthEndHolding(augustEnd, nil),
				monthEndHolding(septemberEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 125_050)},
					report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(125_050)}),
			}},
			want: "Net worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD\n\n" +
				"Month end   chequing     Total\n" +
				"2026-08-31\n" +
				"2026-09-30  1,250.50  1,250.50\n" +
				"\nNo change shown: no account has a balance on 2026-08-31.\n",
		},
		{
			name: "native with CAD and USD on the end day",
			n: report.NetWorth{Currency: money.Native, Dates: []report.NetWorthDate{
				monthEndHolding(augustEnd, nil),
				monthEndHolding(septemberEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 125_050), monthEndRow("chequing", "USD", 200_000)},
					report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(125_050)}, report.NetWorthTotal{Currency: "USD", Value: big.NewInt(200_000)}),
			}},
			want: "Net worth at each month end 2026-08-31 to 2026-09-30\n\n" +
				"Month end   Currency  chequing     Total\n" +
				"2026-08-31\n" +
				"2026-09-30  CAD       1,250.50  1,250.50\n" +
				"2026-09-30  USD       2,000.00  2,000.00\n" +
				"\nNo change shown: no account has a balance on 2026-08-31.\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, renderNetWorthWithChange(c.n))
		})
	}
}

func Test_renderNetWorthWithChange_shows_the_change_when_the_first_month_end_has_a_row_in_one_currency_only(t *testing.T) {
	n := report.NetWorth{Currency: money.Native, Dates: []report.NetWorthDate{
		monthEndHolding(augustEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 100_000)},
			report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(100_000)}),
		monthEndHolding(septemberEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 125_050), monthEndRow("chequing", "USD", 200_000)},
			report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(125_050)}, report.NetWorthTotal{Currency: "USD", Value: big.NewInt(200_000)}),
	}}

	got := renderNetWorthWithChange(n)

	assert.Contains(t, got, "\nChange      USD       +2,000.00  +2,000.00\n")
	assert.NotContains(t, got, "No change shown")
	assert.NotContains(t, got, "No account has a balance")
}

func Test_renderNetWorthWithChange_shows_the_change_when_only_the_last_month_end_is_empty(t *testing.T) {
	n := report.NetWorth{Currency: money.CAD, Dates: []report.NetWorthDate{
		monthEndHolding(augustEnd, []store.NetWorthRow{monthEndRow("chequing", "CAD", 100_000)},
			report.NetWorthTotal{Currency: "CAD", Value: big.NewInt(100_000)}),
		monthEndHolding(septemberEnd, nil),
	}}

	got := renderNetWorthWithChange(n)

	assert.Contains(t, got, "\nChange      -1,000.00  -1,000.00\n")
	assert.NotContains(t, got, "No change shown")
	assert.NotContains(t, got, "No account has a balance")
}
