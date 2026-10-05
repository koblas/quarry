// White-box: renderNetWorth is the unexported table rule, and its rows, captions and Total rows are driven
// as values, without a store behind a command.
package cli

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

var netWorthDay = time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC)

// netWorthIn is a snapshot of rows in currency with the totals given.
func netWorthIn(currency money.Currency, totals []report.NetWorthTotal, rows ...store.NetWorthRow) report.NetWorth {
	return report.NetWorth{
		Dates:    []report.NetWorthDate{{Date: netWorthDay, Rows: rows, Totals: totals}},
		AsOf:     netWorthDay,
		Currency: currency,
	}
}

func chequingCAD(balance, converted *big.Int) store.NetWorthRow {
	return store.NetWorthRow{Type: "chequing", Currency: "CAD", Balance: balance, BalanceCAD: converted}
}

func Test_renderNetWorth_converted_table_lists_rows_with_an_In_column_and_one_total(t *testing.T) {
	n := netWorthIn(money.CAD, []report.NetWorthTotal{{Currency: "CAD", Value: big.NewInt(150_000)}},
		chequingCAD(big.NewInt(100_000), big.NewInt(100_000)),
		store.NetWorthRow{Type: "savings", Currency: "USD", Balance: big.NewInt(36_765), BalanceCAD: big.NewInt(50_000)})

	got := renderNetWorth(n)

	assert.Equal(t, "Net worth on 2026-03-12, amounts in CAD\n\n"+
		"Type      Currency   Balance    In CAD\n"+
		"chequing  CAD       1,000.00  1,000.00\n"+
		"savings   USD         367.65    500.00\n"+
		"Total                         1,500.00\n", got)
}

func Test_renderNetWorth_native_table_has_no_In_column_and_a_total_per_currency(t *testing.T) {
	n := netWorthIn(money.Native, []report.NetWorthTotal{
		{Currency: "CAD", Value: big.NewInt(100_000)}, {Currency: "USD", Value: big.NewInt(36_765)},
	},
		chequingCAD(big.NewInt(100_000), nil),
		store.NetWorthRow{Type: "savings", Currency: "USD", Balance: big.NewInt(36_765)})

	got := renderNetWorth(n)

	assert.Equal(t, "Net worth on 2026-03-12\n\n"+
		"Type      Currency   Balance\n"+
		"chequing  CAD       1,000.00\n"+
		"savings   USD         367.65\n"+
		"Total     CAD       1,000.00\n"+
		"Total     USD         367.65\n", got)
}

func Test_renderNetWorth_leaves_the_In_cell_of_an_unconverted_row_blank(t *testing.T) {
	n := netWorthIn(money.USD, []report.NetWorthTotal{{Currency: "USD", Value: big.NewInt(500)}},
		store.NetWorthRow{Type: "chequing", Currency: "CAD", Balance: big.NewInt(1_000)},
		store.NetWorthRow{Type: "savings", Currency: "USD", Balance: big.NewInt(500), BalanceUSD: big.NewInt(500)})

	got := renderNetWorth(n)

	assert.Equal(t, "Net worth on 2026-03-12, amounts in USD\n\n"+
		"Type      Currency  Balance  In USD\n"+
		"chequing  CAD         10.00\n"+
		"savings   USD          5.00    5.00\n"+
		"Total                          5.00\n", got)
}

func Test_renderNetWorth_prints_a_negative_balance_with_its_sign(t *testing.T) {
	n := netWorthIn(money.CAD, nil, store.NetWorthRow{
		Type: "credit_card", Currency: "CAD", Balance: big.NewInt(-25_050), BalanceCAD: big.NewInt(-25_050),
	})

	got := renderNetWorth(n)

	assert.Contains(t, got, "credit_card  CAD       -250.50  -250.50\n")
}

func Test_renderNetWorth_has_a_caption_and_header_only_when_there_are_no_rows_and_no_totals(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     string
	}{
		{name: "converted", currency: money.CAD, want: "Net worth on 2026-03-12, amounts in CAD\n\nType  Currency  Balance  In CAD\n"},
		{name: "native", currency: money.Native, want: "Net worth on 2026-03-12\n\nType  Currency  Balance\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, renderNetWorth(netWorthIn(c.currency, nil)))
		})
	}
}

var netWorthWindow = store.Window{Since: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), Until: netWorthDay}

// netWorthHistoryOf is a history in currency over the days given.
func netWorthHistoryOf(currency money.Currency, dates ...report.NetWorthDate) report.NetWorth {
	return report.NetWorth{Dates: dates, AsOf: netWorthDay, Window: &netWorthWindow, Currency: currency}
}

func monthEndOf(year int, month time.Month, day int, totals []report.NetWorthTotal, rows ...store.NetWorthRow) report.NetWorthDate {
	return report.NetWorthDate{Date: time.Date(year, month, day, 0, 0, 0, 0, time.UTC), Rows: rows, Totals: totals}
}

func cadTotal(cents int64) []report.NetWorthTotal {
	return []report.NetWorthTotal{{Currency: "CAD", Value: big.NewInt(cents)}}
}

func cadRow(accountType string, cents int64) store.NetWorthRow {
	return store.NetWorthRow{Type: accountType, Currency: "CAD", Balance: big.NewInt(cents), BalanceCAD: big.NewInt(cents)}
}

func Test_renderNetWorth_history_lists_a_column_per_type_with_a_blank_cell_where_a_type_has_no_row(t *testing.T) {
	n := netWorthHistoryOf(money.CAD,
		monthEndOf(2026, time.January, 31, cadTotal(75_000), cadRow("chequing", 100_000), cadRow("credit_card", -25_000)),
		monthEndOf(2026, time.February, 28, cadTotal(200_000), cadRow("chequing", 200_000)))

	got := renderNetWorth(n)

	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in CAD\n\n"+
		"Month end   chequing  credit_card     Total\n"+
		"2026-01-31  1,000.00      -250.00    750.00\n"+
		"2026-02-28  2,000.00               2,000.00\n", got)
}

func Test_renderNetWorth_history_sums_each_type_across_currencies_and_shows_a_zero_sum_as_0_00(t *testing.T) {
	n := netWorthHistoryOf(money.CAD, monthEndOf(2026, time.January, 31, cadTotal(0),
		cadRow("chequing", 50_000),
		store.NetWorthRow{Type: "chequing", Currency: "USD", Balance: big.NewInt(-36_765), BalanceCAD: big.NewInt(-50_000)}))

	got := renderNetWorth(n)

	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-01-31, amounts in CAD\n\n"+
		"Month end   chequing  Total\n"+
		"2026-01-31      0.00   0.00\n", got)
}

func Test_renderNetWorth_history_gives_no_column_to_a_type_whose_balance_is_zero_on_every_date(t *testing.T) {
	n := netWorthHistoryOf(money.CAD, monthEndOf(2026, time.January, 31, cadTotal(100), cadRow("chequing", 100), cadRow("savings", 0)))

	got := renderNetWorth(n)

	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-01-31, amounts in CAD\n\n"+
		"Month end   chequing  Total\n"+
		"2026-01-31      1.00   1.00\n", got)
}

func Test_renderNetWorth_history_lists_a_month_end_with_no_rows_with_a_blank_total(t *testing.T) {
	n := netWorthHistoryOf(money.CAD,
		monthEndOf(2026, time.January, 31, nil),
		monthEndOf(2026, time.February, 28, cadTotal(100), cadRow("chequing", 100)))

	got := renderNetWorth(n)

	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in CAD\n\n"+
		"Month end   chequing  Total\n"+
		"2026-01-31\n"+
		"2026-02-28      1.00   1.00\n", got)
}

func Test_renderNetWorth_history_leaves_a_cell_blank_when_no_rate_converts_its_rows(t *testing.T) {
	n := netWorthHistoryOf(money.USD, monthEndOf(2026, time.January, 31, nil,
		store.NetWorthRow{Type: "chequing", Currency: "CAD", Balance: big.NewInt(1_000)}))

	got := renderNetWorth(n)

	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-01-31, amounts in USD\n\n"+
		"Month end   chequing  Total\n"+
		"2026-01-31\n", got)
}

func Test_renderNetWorth_history_native_lists_a_line_per_currency_with_its_total_and_a_date_alone_when_no_row(t *testing.T) {
	n := netWorthHistoryOf(money.Native,
		monthEndOf(2026, time.January, 31, []report.NetWorthTotal{
			{Currency: "CAD", Value: big.NewInt(75_000)}, {Currency: "USD", Value: big.NewInt(50_000)},
		},
			cadRow("chequing", 100_000), cadRow("credit_card", -25_000),
			store.NetWorthRow{Type: "chequing", Currency: "USD", Balance: big.NewInt(50_000)}),
		monthEndOf(2026, time.February, 28, nil))

	got := renderNetWorth(n)

	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-02-28\n\n"+
		"Month end   Currency  chequing  credit_card   Total\n"+
		"2026-01-31  CAD       1,000.00      -250.00  750.00\n"+
		"2026-01-31  USD         500.00               500.00\n"+
		"2026-02-28\n", got)
}

func Test_renderNetWorth_history_has_a_caption_and_header_only_when_no_month_end_is_listed(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     string
	}{
		{name: "converted", currency: money.CAD, want: "Net worth at each month end 2026-01-01 to 2026-03-12, amounts in CAD\n\nMonth end  Total\n"},
		{name: "native", currency: money.Native, want: "Net worth at each month end 2026-01-01 to 2026-03-12\n\nMonth end  Currency  Total\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, renderNetWorth(netWorthHistoryOf(c.currency)))
		})
	}
}

func Test_render_networth_omits_a_zero_row_that_json_keeps(t *testing.T) {
	n := netWorthIn(money.CAD, []report.NetWorthTotal{{Currency: "CAD", Value: big.NewInt(100)}},
		chequingCAD(big.NewInt(100), big.NewInt(100)),
		store.NetWorthRow{Type: "savings", Currency: "CAD", Balance: big.NewInt(0), BalanceCAD: big.NewInt(0)})

	text := renderNetWorth(n)
	balances := document.NewNetWorth(n, nil).Dates[0].Balances

	assert.NotContains(t, text, "savings")
	assert.Len(t, balances, 2)
}
