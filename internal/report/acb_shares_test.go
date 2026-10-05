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

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 0}}, acbPositionRows(got))
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
