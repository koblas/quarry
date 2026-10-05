package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbNoCostAdd is shares added to acct-1 on date with no cost; amount 0 as the importer stores it.
func acbNoCostAdd(t *testing.T, sourceID int64, security, date string, shares int64) store.InvestmentTransaction {
	t.Helper()
	return acbTx(t, sourceID, "acct-1", security, date, store.ActionAddShares, "CAD", shares, 0)
}

func acbCosted(tx store.InvestmentTransaction, cents int64) store.InvestmentTransaction {
	tx.CostBasis = &cents
	return tx
}

func acbReinvest(t *testing.T, sourceID int64, date string, shares int64) store.InvestmentTransaction {
	t.Helper()
	return acbTx(t, sourceID, "acct-1", "sec-1", date, store.ActionReinvestDividend, "CAD", shares, -500)
}

func acbBuy(t *testing.T) store.InvestmentTransaction {
	t.Helper()
	return acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000)
}

func acbSell(t *testing.T, sourceID int64, date string, shares int64) store.InvestmentTransaction {
	t.Helper()
	return acbTx(t, sourceID, "acct-1", "sec-1", date, store.ActionSell, "CAD", -shares, 2_000)
}

func acbSaleMarks(result report.ACB) []bool {
	var marks []bool
	for _, year := range result.Years {
		for _, sale := range year.Sales {
			marks = append(marks, sale.UnknownCost)
		}
	}

	return marks
}

func acbEventMarks(position report.ACBSecurity) []bool {
	marks := make([]bool, 0, len(position.Events))
	for _, event := range position.Events {
		marks = append(marks, event.UnknownCost)
	}

	return marks
}

func Test_acb_marks_a_sale_made_while_shares_with_no_cost_are_held(t *testing.T) {
	add := acbNoCostAdd(t, 2, "sec-1", "2024-02-01", 5*acbMillion)
	registeredAdd := acbTx(t, 2, "acct-9", "sec-1", "2024-02-01", store.ActionAddShares, "CAD", 5*acbMillion, 0)
	registeredReinvest := acbTx(t, 2, "acct-9", "sec-1", "2024-02-01", store.ActionReinvestDividend, "CAD", acbMillion, -500)
	sale := acbSell(t, 3, "2024-03-01", 4*acbMillion)
	cases := []struct {
		name           string
		txs            []store.InvestmentTransaction
		wantMarks      []bool
		wantIncomplete bool
	}{
		{
			name:      "added shares with no cost, then a sale",
			txs:       []store.InvestmentTransaction{acbBuy(t), add, sale},
			wantMarks: []bool{true}, wantIncomplete: true,
		},
		{
			name:      "added shares with a cost, then a sale",
			txs:       []store.InvestmentTransaction{acbBuy(t), acbCosted(add, 5_000), sale},
			wantMarks: []bool{false}, wantIncomplete: false,
		},
		{
			name:      "a reinvested dividend with no cost, then a sale",
			txs:       []store.InvestmentTransaction{acbBuy(t), acbReinvest(t, 2, "2024-02-01", acbMillion), sale},
			wantMarks: []bool{true}, wantIncomplete: true,
		},
		{
			name:      "a reinvested dividend with a cost, then a sale",
			txs:       []store.InvestmentTransaction{acbBuy(t), acbCosted(acbReinvest(t, 2, "2024-02-01", acbMillion), 500), sale},
			wantMarks: []bool{false}, wantIncomplete: false,
		},
		{
			name:      "the no-cost add and the sale on one day, the sale first by source id",
			txs:       []store.InvestmentTransaction{acbBuy(t), acbNoCostAdd(t, 3, "sec-1", "2024-02-01", 5*acbMillion), acbSell(t, 2, "2024-02-01", 4*acbMillion)},
			wantMarks: []bool{true}, wantIncomplete: true,
		},
		{
			name:      "a sale before the no-cost add",
			txs:       []store.InvestmentTransaction{acbBuy(t), acbSell(t, 2, "2024-01-20", 4*acbMillion), acbNoCostAdd(t, 3, "sec-1", "2024-02-01", 5*acbMillion)},
			wantMarks: []bool{false}, wantIncomplete: true,
		},
		{
			name:      "two partial sales",
			txs:       []store.InvestmentTransaction{acbBuy(t), add, acbSell(t, 3, "2024-03-01", 2*acbMillion), acbSell(t, 4, "2024-04-01", 2*acbMillion)},
			wantMarks: []bool{true, true}, wantIncomplete: true,
		},
		{
			name:      "a sale of more than the pool holds",
			txs:       []store.InvestmentTransaction{acbBuy(t), add, acbSell(t, 3, "2024-03-01", 20*acbMillion)},
			wantMarks: []bool{true}, wantIncomplete: true,
		},
		{
			name:      "added shares of zero units",
			txs:       []store.InvestmentTransaction{acbBuy(t), acbNoCostAdd(t, 2, "sec-1", "2024-02-01", 0), sale},
			wantMarks: []bool{false}, wantIncomplete: false,
		},
		{
			name:      "a reinvested dividend of zero units",
			txs:       []store.InvestmentTransaction{acbBuy(t), acbReinvest(t, 2, "2024-02-01", 0), sale},
			wantMarks: []bool{false}, wantIncomplete: false,
		},
		{
			name:      "a reinvested dividend of negative units",
			txs:       []store.InvestmentTransaction{acbBuy(t), acbReinvest(t, 2, "2024-02-01", -acbMillion), sale},
			wantMarks: []bool{false}, wantIncomplete: false,
		},
		{
			name:      "added shares in a registered account",
			txs:       []store.InvestmentTransaction{acbBuy(t), registeredAdd, sale},
			wantMarks: []bool{false}, wantIncomplete: false,
		},
		{
			name:      "a dividend reinvested in a registered account",
			txs:       []store.InvestmentTransaction{acbBuy(t), registeredReinvest, sale},
			wantMarks: []bool{false}, wantIncomplete: false,
		},
		{
			name:      "added shares dated after today",
			txs:       []store.InvestmentTransaction{acbBuy(t), sale, acbNoCostAdd(t, 4, "sec-1", "2026-10-06", 5*acbMillion)},
			wantMarks: []bool{false}, wantIncomplete: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, c.txs...)

			require.Len(t, got.Securities, 1)
			assert.Equal(t, c.wantMarks, acbSaleMarks(got))
			assert.Equal(t, c.wantIncomplete, got.Securities[0].Incomplete)
		})
	}
}

func Test_acb_stops_marking_sales_once_the_pool_sells_out(t *testing.T) {
	got := acbWalkOf(t,
		acbBuy(t),
		acbNoCostAdd(t, 2, "sec-1", "2024-02-01", 5*acbMillion),
		acbSell(t, 3, "2024-03-01", 15*acbMillion),
		acbTx(t, 4, "acct-1", "sec-1", "2024-04-01", store.ActionBuy, "CAD", 10*acbMillion, -12_000),
		acbSell(t, 5, "2024-05-01", 4*acbMillion),
	)

	require.Len(t, got.Securities, 1)
	assert.Equal(t, []bool{true, false}, acbSaleMarks(got))
	assert.False(t, got.Securities[0].Incomplete)
}

func Test_acb_reopens_the_span_when_shares_with_no_cost_are_added_again(t *testing.T) {
	got := acbWalkOf(t,
		acbBuy(t),
		acbNoCostAdd(t, 2, "sec-1", "2024-02-01", 5*acbMillion),
		acbSell(t, 3, "2024-03-01", 15*acbMillion),
		acbTx(t, 4, "acct-1", "sec-1", "2024-04-01", store.ActionBuy, "CAD", 10*acbMillion, -12_000),
		acbNoCostAdd(t, 5, "sec-1", "2024-05-01", 2*acbMillion),
		acbSell(t, 6, "2024-06-01", 4*acbMillion),
	)

	require.Len(t, got.Securities, 1)
	assert.Equal(t, []bool{true, true}, acbSaleMarks(got))
	assert.True(t, got.Securities[0].Incomplete)
}

func Test_acb_marks_the_events_that_move_shares_with_no_cost(t *testing.T) {
	got := acbWalkOf(t,
		acbBuy(t),
		acbCosted(acbNoCostAdd(t, 2, "sec-1", "2024-02-01", acbMillion), 1_000),
		acbNoCostAdd(t, 3, "sec-1", "2024-03-01", 5*acbMillion),
		acbReinvest(t, 4, "2024-04-01", acbMillion),
		acbSell(t, 5, "2024-05-01", 2*acbMillion),
	)

	require.Len(t, got.Securities, 1)
	assert.Equal(t, []bool{false, false, true, true, true}, acbEventMarks(got.Securities[0]))
}

func Test_acb_marks_a_removal_inside_the_span_and_ends_the_span_when_it_empties_the_pool(t *testing.T) {
	cases := []struct {
		name           string
		removed        int64
		wantIncomplete bool
	}{
		{name: "a removal that leaves shares keeps the span", removed: 3 * acbMillion, wantIncomplete: true},
		{name: "a removal that empties the pool ends the span", removed: 15 * acbMillion, wantIncomplete: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t,
				acbBuy(t),
				acbNoCostAdd(t, 2, "sec-1", "2024-02-01", 5*acbMillion),
				acbTx(t, 3, "acct-1", "sec-1", "2024-03-01", store.ActionRemoveShares, "CAD", -c.removed, 0),
			)

			require.Len(t, got.Securities, 1)
			assert.Equal(t, []bool{false, true, true}, acbEventMarks(got.Securities[0]))
			assert.Equal(t, c.wantIncomplete, got.Securities[0].Incomplete)
		})
	}
}

func Test_acb_leaves_a_removal_before_any_shares_were_added_with_no_cost_unmarked(t *testing.T) {
	got := acbWalkOf(t,
		acbBuy(t),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionRemoveShares, "CAD", -3*acbMillion, 0),
		acbNoCostAdd(t, 3, "sec-1", "2024-03-01", 5*acbMillion),
	)

	require.Len(t, got.Securities, 1)
	assert.Equal(t, []bool{false, false, true}, acbEventMarks(got.Securities[0]))
}

func Test_acb_keeps_the_span_to_the_security_that_was_added_with_no_cost(t *testing.T) {
	got := acbWalkOf(t,
		acbBuy(t),
		acbNoCostAdd(t, 2, "sec-1", "2024-02-01", 5*acbMillion),
		acbTx(t, 3, "acct-1", "sec-2", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
		acbTx(t, 4, "acct-1", "sec-2", "2024-03-01", store.ActionSell, "CAD", -4*acbMillion, 2_000),
	)

	require.Len(t, got.Securities, 2)
	assert.Equal(t, []bool{false}, acbSaleMarks(got))
	incomplete := map[string]bool{}
	for _, position := range got.Securities {
		incomplete[position.Security.ID] = position.Incomplete
	}
	assert.Equal(t, map[string]bool{"sec-1": true, "sec-2": false}, incomplete)
}

func Test_ACBYear_counts_the_sales_marked_unknown_cost(t *testing.T) {
	cases := []struct {
		name  string
		sales []report.ACBSale
		want  int
	}{
		{name: "none", sales: []report.ACBSale{{}, {PossibleSuperficialLoss: true}}, want: 0},
		{name: "one", sales: []report.ACBSale{{}, {UnknownCost: true}}, want: 1},
		{name: "two", sales: []report.ACBSale{{UnknownCost: true}, {}, {UnknownCost: true, PossibleSuperficialLoss: true}}, want: 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.ACBYear{Sales: c.sales}.UnknownCostSales())
		})
	}
}
