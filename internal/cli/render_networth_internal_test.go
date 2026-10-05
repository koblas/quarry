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

func Test_renderNetWorth_leaves_the_In_cell_of_an_unconverted_row_blank_and_out_of_the_total(t *testing.T) {
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

func Test_render_networth_omits_a_zero_row_that_json_keeps(t *testing.T) {
	n := netWorthIn(money.CAD, []report.NetWorthTotal{{Currency: "CAD", Value: big.NewInt(100)}},
		chequingCAD(big.NewInt(100), big.NewInt(100)),
		store.NetWorthRow{Type: "savings", Currency: "CAD", Balance: big.NewInt(0), BalanceCAD: big.NewInt(0)})

	text := renderNetWorth(n)
	balances := document.NewNetWorth(n, nil).Dates[0].Balances

	assert.NotContains(t, text, "savings")
	assert.Len(t, balances, 2)
}
