package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_acb_adds_added_shares_at_their_cost_or_at_none(t *testing.T) {
	cost := int64(15_000)
	withCost := acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionAddShares, "CAD", 5*acbMillion, 0)
	withCost.CostBasis = &cost
	withoutCost := acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionAddShares, "CAD", 5*acbMillion, 0)
	buy := acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000)
	cases := []struct {
		name string
		add  store.InvestmentTransaction
		want acbPositionRow
	}{
		{name: "with a cost adds the units and the cost", add: withCost, want: acbPositionRow{Name: "XEQT", Shares: "15", ACB: 25_000}},
		{name: "with no cost adds the units at 0.00", add: withoutCost, want: acbPositionRow{Name: "XEQT", Shares: "15", ACB: 10_000}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, buy, c.add)

			assert.Equal(t, []acbPositionRow{c.want}, acbPositionRows(got))
			assert.Empty(t, acbSaleRows(got))
		})
	}
}

func Test_acb_removes_a_removals_share_of_the_acb_with_no_sale_and_no_gain(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionRemoveShares, "CAD", -4*acbMillion, 0),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "6", ACB: 6_000}}, acbPositionRows(got))
	assert.Empty(t, acbSaleRows(got))
	removal := got.Securities[0].Events[1]
	assert.Equal(t, store.ActionRemoveShares, removal.Action)
	assert.Equal(t, "4", removal.Shares.RatString())
	assert.Equal(t, "6", removal.Held.RatString())
	assert.Equal(t, int64(6_000), removal.ACB)
	assert.False(t, removal.Realized)
	assert.Nil(t, removal.Outlays)
}

func Test_acb_removes_the_whole_acb_when_a_removal_exceeds_the_pool(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionRemoveShares, "CAD", -12*acbMillion, 0),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "-2", ACB: 0}}, acbPositionRows(got))
}

func Test_acb_ignores_added_and_removed_shares_outside_the_pool(t *testing.T) {
	cost := int64(7_000)
	added := acbTx(t, 3, "acct-9", "sec-1", "2024-02-02", store.ActionAddShares, "CAD", 5*acbMillion, 0)
	added.CostBasis = &cost

	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
		acbTx(t, 2, "acct-9", "sec-1", "2024-02-01", store.ActionRemoveShares, "CAD", -4*acbMillion, 0),
		added,
		acbTx(t, 4, "acct-7", "sec-1", "2024-02-03", store.ActionRemoveShares, "CAD", -4*acbMillion, 0),
	)

	require.Len(t, got.Securities, 1)
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 10_000}}, acbPositionRows(got))
	assert.Len(t, got.Securities[0].Events, 1)
}

func Test_acb_applies_a_days_added_shares_before_its_removed_shares(t *testing.T) {
	cost := int64(15_000)
	added := acbTx(t, 3, "acct-1", "sec-1", "2024-02-01", store.ActionAddShares, "CAD", 5*acbMillion, 0)
	added.CostBasis = &cost

	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionRemoveShares, "CAD", -10*acbMillion, 0),
		added,
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "5", ACB: 8_333}}, acbPositionRows(got))
}

func Test_acb_skips_added_and_removed_shares_that_move_no_units(t *testing.T) {
	cost := int64(15_000)
	buy := acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000)
	addWithCost := func(units int64) store.InvestmentTransaction {
		add := acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionAddShares, "CAD", units, 0)
		add.CostBasis = &cost
		return add
	}
	cases := []struct {
		name string
		move store.InvestmentTransaction
	}{
		{name: "zero units added with a cost", move: addWithCost(0)},
		{name: "zero units added with no cost", move: acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionAddShares, "CAD", 0, 0)},
		{name: "negative units added with a cost", move: addWithCost(-5 * acbMillion)},
		{name: "zero units removed", move: acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionRemoveShares, "CAD", 0, 0)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, buy, c.move)

			assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 10_000}}, acbPositionRows(got))
			assert.Len(t, got.Securities[0].Events, 1)
		})
	}
}

func Test_acb_orders_a_days_added_shares_then_split_then_removed_shares_whatever_their_source_ids(t *testing.T) {
	cost := int64(15_000)
	added := acbTx(t, 3, "acct-1", "sec-1", "2024-02-01", store.ActionAddShares, "CAD", 5*acbMillion, 0)
	added.CostBasis = &cost

	got := acbWalkOf(t,
		acbTx(t, 10, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
		acbTx(t, 1, "acct-1", "sec-1", "2024-02-01", store.ActionRemoveShares, "CAD", -10*acbMillion, 0),
		acbSplitTx(t, 2, "acct-1", "2024-02-01", 2*acbMillion, acbMillion),
		added,
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "20", ACB: 16_667}}, acbPositionRows(got))
}

func Test_acb_orders_a_days_sale_and_removed_shares_by_source_id_after_its_added_shares(t *testing.T) {
	cost := int64(6_000)
	added := acbTx(t, 9, "acct-1", "sec-1", "2024-02-01", store.ActionAddShares, "CAD", 6*acbMillion, 0)
	added.CostBasis = &cost
	cases := []struct {
		name                 string
		sellSource           int64
		removeSource         int64
		wantACBRemovedBySale int64
	}{
		{name: "the sale has the lower source id", sellSource: 1, removeSource: 2, wantACBRemovedBySale: 6_000},
		{name: "the removal has the lower source id", sellSource: 2, removeSource: 1, wantACBRemovedBySale: 4_000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t,
				acbTx(t, 10, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 4*acbMillion, -4_000),
				added,
				acbTx(t, c.sellSource, "acct-1", "sec-1", "2024-02-01", store.ActionSell, "CAD", -6*acbMillion, 9_000),
				acbTx(t, c.removeSource, "acct-1", "sec-1", "2024-02-01", store.ActionRemoveShares, "CAD", -6*acbMillion, 0),
			)

			sales := acbSaleRows(got)
			require.Len(t, sales, 1)
			assert.Equal(t, c.wantACBRemovedBySale, sales[0].ACBRemoved)
		})
	}
}
