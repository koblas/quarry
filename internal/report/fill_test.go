package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fillWindow touches January, February and March 2026.
var fillWindow = store.Window{Since: day(2026, time.January, 1), Until: day(2026, time.March, 31)}

func spendFillRow(month, currency string) store.SpendingRow {
	return store.SpendingRow{Key: &month, Currency: currency, Spent: 100}
}

func cashFlowFillRow(month, currency string) store.CashFlowRow {
	return store.CashFlowRow{Period: month, Currency: currency, Income: 100}
}

// spendFillKeys is each spend --by month row of a read as "<month> <currency>", in order.
func spendFillKeys(t *testing.T, spending store.Spending, currency money.Currency) []string {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{spending: spending}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{Window: fillWindow, By: store.SpendByMonth, Currency: currency})

	require.NoError(t, err)
	keys := make([]string, len(result.Rows))
	for i, r := range result.Rows {
		keys[i] = *r.Key + " " + r.Currency
	}
	return keys
}

// cashFlowFillKeys is each cashflow row of a read as "<month> <currency>", in order.
func cashFlowFillKeys(t *testing.T, flow store.CashFlow, currency money.Currency) []string {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{cashFlow: flow}))

	result, err := srv.CashFlow(t.Context(), report.CashFlowRequest{Window: fillWindow, By: store.CashFlowByMonth, Currency: currency})

	require.NoError(t, err)
	keys := make([]string, len(result.Rows))
	for i, r := range result.Rows {
		keys[i] = r.Period + " " + r.Currency
	}
	return keys
}

// fillCase is a report in currency over a store whose Totals are totals and whose rows are stored,
// each "<month> <currency>"; want lists the rows both reports must give.
type fillCase struct {
	name     string
	currency money.Currency
	totals   []string
	stored   []string
	want     []string
}

var fillCases = []fillCase{
	{
		name: "CAD mode fills CAD in every period and USD only where it was stored", currency: money.CAD,
		totals: []string{"CAD", "USD"}, stored: []string{"2026-01 USD"},
		want: []string{"2026-01 CAD", "2026-01 USD", "2026-02 CAD", "2026-03 CAD"},
	},
	{
		name: "USD mode fills USD in every period and CAD only where it was stored", currency: money.USD,
		totals: []string{"CAD", "USD"}, stored: []string{"2026-02 CAD"},
		want: []string{"2026-01 USD", "2026-02 USD", "2026-02 CAD", "2026-03 USD"},
	},
	{
		name: "the target row comes first even when another currency leads the totals", currency: money.CAD,
		totals: []string{"USD", "CAD"}, stored: []string{"2026-01 USD", "2026-01 CAD"},
		want: []string{"2026-01 CAD", "2026-01 USD", "2026-02 CAD", "2026-03 CAD"},
	},
	{
		name: "a target with no stored row still gets zero rows beside the other currency's rows", currency: money.CAD,
		totals: []string{"USD"}, stored: []string{"2026-02 USD"},
		want: []string{"2026-01 CAD", "2026-02 CAD", "2026-02 USD", "2026-03 CAD"},
	},
	{
		name: "an empty window gets no rows in USD mode", currency: money.USD,
		want: []string{},
	},
	{
		name: "an empty window gets no rows in CAD mode", currency: money.CAD,
		want: []string{},
	},
	{
		name: "native fills every currency in every period", currency: money.Native,
		totals: []string{"CAD", "USD"}, stored: []string{"2026-01 USD"},
		want: []string{"2026-01 CAD", "2026-01 USD", "2026-02 CAD", "2026-02 USD", "2026-03 CAD", "2026-03 USD"},
	},
}

func splitKey(key string) (string, string) {
	return key[:7], key[8:]
}

func Test_spend_by_month_fills_zero_rows_for_the_report_currency_only(t *testing.T) {
	for _, c := range fillCases {
		t.Run(c.name, func(t *testing.T) {
			var spending store.Spending
			for _, cur := range c.totals {
				spending.Totals = append(spending.Totals, store.SpendingTotal{Currency: cur, Spent: 100})
			}
			for _, key := range c.stored {
				month, cur := splitKey(key)
				spending.Rows = append(spending.Rows, spendFillRow(month, cur))
			}

			assert.Equal(t, c.want, spendFillKeys(t, spending, c.currency))
		})
	}
}

func Test_cashflow_fills_zero_rows_for_the_report_currency_only(t *testing.T) {
	for _, c := range fillCases {
		t.Run(c.name, func(t *testing.T) {
			var flow store.CashFlow
			for _, cur := range c.totals {
				flow.Totals = append(flow.Totals, store.CashFlowTotal{Currency: cur, Income: 100})
			}
			for _, key := range c.stored {
				month, cur := splitKey(key)
				flow.Rows = append(flow.Rows, cashFlowFillRow(month, cur))
			}

			assert.Equal(t, c.want, cashFlowFillKeys(t, flow, c.currency))
		})
	}
}

func Test_spend_carries_the_stores_unconverted_count_and_reads_the_store_once(t *testing.T) {
	var reads int
	unconverted := store.Unconverted{Transactions: 3, FirstRate: day(2026, time.January, 2)}
	srv := report.NewServer(report.WithStore(fakeStore{spending: store.Spending{Unconverted: unconverted}, spendingReads: &reads}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, unconverted, result.Unconverted)
	assert.Equal(t, 1, reads)
}

func Test_cashflow_carries_the_stores_unconverted_count_and_reads_the_store_once(t *testing.T) {
	var reads int
	unconverted := store.Unconverted{Transactions: 3, FirstRate: day(2026, time.January, 2)}
	srv := report.NewServer(report.WithStore(fakeStore{cashFlow: store.CashFlow{Unconverted: unconverted}, cashFlowReads: &reads}))

	result, err := srv.CashFlow(t.Context(), report.CashFlowRequest{Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, unconverted, result.Unconverted)
	assert.Equal(t, 1, reads)
}
