package report_test

import (
	"math/big"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbShortPrelude is ten shares bought for 100.00, then fifteen sold: the pool is short five.
func acbShortPrelude(t *testing.T) []store.InvestmentTransaction {
	t.Helper()
	return []store.InvestmentTransaction{acbBuy(t), acbSell(t, 2, "2024-02-01", 15*acbMillion)}
}

// acbShortCover is units of sec-1 bought in acct-1 on 2024-03-01 for cost cents.
func acbShortCover(t *testing.T, units, cost int64) store.InvestmentTransaction {
	t.Helper()
	return acbTx(t, 3, "acct-1", "sec-1", "2024-03-01", store.ActionBuy, "CAD", units, -cost)
}

// acbLastOversold is the Oversold of the first security's last event, "" when nil.
func acbLastOversold(t *testing.T, result report.ACB) string {
	t.Helper()
	require.NotEmpty(t, result.Securities)
	events := result.Securities[0].Events
	oversold := events[len(events)-1].Oversold
	if oversold == nil {
		return ""
	}
	return oversold.RatString()
}

func Test_acb_counts_the_units_sold_beyond_the_pool_at_no_cost_and_leaves_the_pool_short(t *testing.T) {
	commission := int64(10_000)
	sell := acbSell(t, 2, "2024-02-01", 12*acbMillion)
	sell.Amount, sell.Commission = 50_000, &commission

	got := acbWalkOf(t, acbBuy(t), sell)

	assert.Equal(t, []acbSaleRow{
		{Date: "2024-02-01", Security: "sec-1", Shares: "12", Proceeds: 50_100, Outlays: 100, ACBRemoved: 10_000, Gain: 40_000},
	}, acbSaleRows(got))
	assert.Equal(t, []bool{true}, acbSaleMarks(got))
	sale := got.Securities[0].Events[1]
	assert.Equal(t, "-2", sale.Held.RatString())
	assert.Equal(t, int64(0), sale.ACB)
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "-2", ACB: 0}}, acbPositionRows(got))
}

func Test_acb_marks_a_disposition_oversold_only_beyond_what_the_pool_holds(t *testing.T) {
	removal := acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionRemoveShares, "CAD", -12*acbMillion, 0)
	cases := []struct {
		name         string
		txs          []store.InvestmentTransaction
		wantOversold string
		wantHeld     string
	}{
		{name: "a sale of what the pool holds", txs: []store.InvestmentTransaction{acbBuy(t), acbSell(t, 2, "2024-02-01", 10*acbMillion)}, wantOversold: "", wantHeld: "0"},
		{name: "a sale of a millionth more than it holds", txs: []store.InvestmentTransaction{acbBuy(t), acbSell(t, 2, "2024-02-01", 10*acbMillion+1)}, wantOversold: "1/1000000", wantHeld: "-1/1000000"},
		{name: "a removal beyond the pool", txs: []store.InvestmentTransaction{acbBuy(t), removal}, wantOversold: "2", wantHeld: "-2"},
		{
			name:         "a second sale while the pool is short counts the whole short",
			txs:          []store.InvestmentTransaction{acbBuy(t), acbSell(t, 2, "2024-02-01", 12*acbMillion), acbSell(t, 3, "2024-02-02", 3*acbMillion)},
			wantOversold: "5", wantHeld: "-5",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, c.txs...)

			assert.Equal(t, c.wantOversold, acbLastOversold(t, got))
			events := got.Securities[0].Events
			assert.Equal(t, c.wantHeld, events[len(events)-1].Held.RatString())
		})
	}
}

func Test_acb_covers_a_short_before_adding_units_to_the_pool(t *testing.T) {
	cases := []struct {
		name       string
		units      int64
		wantShares string
	}{
		{name: "units short of the short", units: 3 * acbMillion, wantShares: "-2"},
		{name: "units equal to the short", units: 5 * acbMillion, wantShares: "0"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, append(acbShortPrelude(t), acbShortCover(t, c.units, 5_000))...)

			assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: c.wantShares, ACB: 0}}, acbPositionRows(got))
			cover := got.Securities[0].Events[2]
			assert.Equal(t, c.wantShares, cover.Held.RatString())
			assert.Equal(t, int64(-5_000), cover.CAD)
		})
	}
}

func Test_acb_adds_the_cost_of_the_units_beyond_the_short_pro_rata_rounded_half_away(t *testing.T) {
	cases := []struct {
		name       string
		txs        []store.InvestmentTransaction
		rates      []store.Rate
		wantShares string
		wantACB    int64
	}{
		{
			name:       "a buy of twice the short at an odd cost rounds the half cent up",
			txs:        append(acbShortPrelude(t), acbShortCover(t, 10*acbMillion, 1_001)),
			wantShares: "5", wantACB: 501,
		},
		{
			name:       "a reinvested dividend with a cost",
			txs:        append(acbShortPrelude(t), acbCosted(acbReinvest(t, 3, "2024-03-01", 10*acbMillion), 501)),
			wantShares: "5", wantACB: 251,
		},
		{
			name:       "added shares with a cost",
			txs:        append(acbShortPrelude(t), acbCosted(acbNoCostAdd(t, 3, "sec-1", "2024-03-01", 10*acbMillion), 1_001)),
			wantShares: "5", wantACB: 501,
		},
		{
			name: "a USD buy converts the full cost before it is pro-rated",
			txs: append(acbShortPrelude(t),
				acbTx(t, 3, "acct-3", "sec-1", "2024-03-01", store.ActionBuy, "USD", 6*acbMillion, -1_001)),
			rates:      []store.Rate{acbRate(t, "2024-01-02", 1_500_000)},
			wantShares: "1", wantACB: 250,
		},
		{
			name:       "a buy into a pool that is not short adds the whole cost",
			txs:        []store.InvestmentTransaction{acbBuy(t), acbShortCover(t, 10*acbMillion, 1_001)},
			wantShares: "20", wantACB: 11_001,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "CAD")}, c.rates, c.txs...)

			assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: c.wantShares, ACB: c.wantACB}}, acbPositionRows(got))
		})
	}
}

func Test_acb_ends_the_unknown_cost_span_when_a_disposition_leaves_the_pool_short(t *testing.T) {
	add := acbNoCostAdd(t, 2, "sec-1", "2024-02-01", 5*acbMillion)
	cases := []struct {
		name      string
		txs       []store.InvestmentTransaction
		wantMarks []bool
	}{
		{
			name: "a sale that leaves the pool short, then a covering buy and a sale",
			txs: []store.InvestmentTransaction{
				acbBuy(t), add, acbSell(t, 3, "2024-03-01", 20*acbMillion),
				acbTx(t, 4, "acct-1", "sec-1", "2024-04-01", store.ActionBuy, "CAD", 10*acbMillion, -5_000),
				acbSell(t, 5, "2024-05-01", 2*acbMillion),
			},
			wantMarks: []bool{true, false},
		},
		{
			name:      "a sale that leaves shares held, then another sale",
			txs:       []store.InvestmentTransaction{acbBuy(t), add, acbSell(t, 3, "2024-03-01", 2*acbMillion), acbSell(t, 4, "2024-04-01", 2*acbMillion)},
			wantMarks: []bool{true, true},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, c.txs...)

			assert.Equal(t, c.wantMarks, acbSaleMarks(got))
		})
	}
}

func Test_acb_opens_no_unknown_cost_span_for_shares_with_no_cost_that_only_cover_a_short(t *testing.T) {
	cases := []struct {
		name           string
		units          int64
		wantMarks      []bool
		wantIncomplete bool
	}{
		{name: "units that only cover the short", units: 5 * acbMillion, wantMarks: []bool{true, false}, wantIncomplete: false},
		{name: "units partly beyond the short", units: 8 * acbMillion, wantMarks: []bool{true, true}, wantIncomplete: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			txs := append(acbShortPrelude(t),
				acbNoCostAdd(t, 3, "sec-1", "2024-03-01", c.units),
				acbTx(t, 4, "acct-1", "sec-1", "2024-03-15", store.ActionBuy, "CAD", 10*acbMillion, -5_000),
				acbSell(t, 5, "2024-04-01", 2*acbMillion),
			)

			got := acbWalkOf(t, txs...)

			assert.Equal(t, c.wantMarks, acbSaleMarks(got))
			assert.Equal(t, c.wantIncomplete, got.Securities[0].Incomplete)
			assert.True(t, got.Securities[0].Events[2].UnknownCost)
		})
	}
}

func Test_acb_marks_a_short_position_incomplete_until_it_is_covered(t *testing.T) {
	cases := []struct {
		name           string
		txs            []store.InvestmentTransaction
		wantIncomplete bool
	}{
		{name: "a short", txs: acbShortPrelude(t), wantIncomplete: true},
		{name: "a short covered back to 0", txs: append(acbShortPrelude(t), acbShortCover(t, 5*acbMillion, 5_000)), wantIncomplete: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, c.txs...)

			assert.Equal(t, c.wantIncomplete, got.Securities[0].Incomplete)
			assert.Nil(t, got.Securities[0].PerShare())
		})
	}
}

func Test_acb_skips_an_adjustment_while_the_pool_is_short_as_not_held(t *testing.T) {
	cases := []struct {
		name       string
		adjustment report.ACBAdjustment
	}{
		{name: "a return of capital", adjustment: acbAdjustment(t, "sec-1", "2024-06-30", 400, 0)},
		{name: "a reinvested distribution", adjustment: acbAdjustment(t, "sec-1", "2024-06-30", 0, 400)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkAdjusted(t, []report.ACBAdjustment{c.adjustment}, acbShortPrelude(t)...)

			assert.Equal(t, []report.ACBAdjustmentIssue{
				{Kind: report.ACBAdjustmentNotHeld, Item: 1, SecurityID: "sec-1", Security: "XEQT", Date: dateOf(t, "2024-06-30")},
			}, got.AdjustmentIssues)
			assert.Empty(t, acbAdjustmentRows(t, got))
			assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "-5", ACB: 0}}, acbPositionRows(got))
			assert.Equal(t, int64(0), got.Years[0].ReturnOfCapitalGain)
		})
	}
}

func Test_acb_splits_a_short_pool(t *testing.T) {
	txs := []store.InvestmentTransaction{
		acbBuy(t), acbSell(t, 2, "2024-02-01", 20*acbMillion), acbSplitTx(t, 3, "acct-1", "2024-03-01", 2*acbMillion, acbMillion),
	}

	got := acbWalkOf(t, txs...)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "-20", ACB: 0}}, acbPositionRows(got))
}

// acbSplitSale is a buy of bought millionths of sec-1 for 3,000.00, a split of newShares for oldShares, and
// a sale of sold millionths.
type acbSplitSale struct {
	bought, newShares, oldShares, sold int64
}

// acbHundredThirded is 100 shares split 1 for 3, then sold millionths.
func acbHundredThirded(sold int64) acbSplitSale {
	return acbSplitSale{bought: 100 * acbMillion, newShares: acbMillion, oldShares: 3 * acbMillion, sold: sold}
}

// acbThousandSeventh is 1,000 shares split 1 for 7, then sold millionths.
func acbThousandSeventh(sold int64) acbSplitSale {
	return acbSplitSale{bought: 1_000 * acbMillion, newShares: acbMillion, oldShares: 7 * acbMillion, sold: sold}
}

func acbSplitThenSell(t *testing.T, s acbSplitSale) []store.InvestmentTransaction {
	t.Helper()
	return []store.InvestmentTransaction{
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", s.bought, -300_000),
		acbSplitTx(t, 2, "acct-1", "2024-03-01", s.newShares, s.oldShares),
		acbSell(t, 3, "2024-04-01", s.sold),
	}
}

func Test_acb_counts_a_split_leftover_below_a_millionth_as_flat(t *testing.T) {
	cases := []struct {
		name         string
		txs          []store.InvestmentTransaction
		wantOversold string
		wantHeld     string
	}{
		{
			name:         "a 1 for 3 split of 100 sold at the shares millionth",
			txs:          acbSplitThenSell(t, acbHundredThirded(33_333_333)),
			wantOversold: "", wantHeld: "0",
		},
		{
			name:         "a 1 for 7 split of 1,000 sold at the shares rounded millionth",
			txs:          acbSplitThenSell(t, acbThousandSeventh(142_857_143)),
			wantOversold: "", wantHeld: "0",
		},
		{
			name:         "a 1 for 3 split of 100 sold a millionth beyond the shares",
			txs:          acbSplitThenSell(t, acbHundredThirded(33_333_334)),
			wantOversold: "1/1000000", wantHeld: "-1/1000000",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, c.txs...)

			assert.Equal(t, c.wantOversold, acbLastOversold(t, got))
			events := got.Securities[0].Events
			assert.Equal(t, c.wantHeld, events[len(events)-1].Held.RatString())
		})
	}
}

func Test_acb_skips_a_return_of_capital_on_a_pool_a_split_left_flat_as_not_held(t *testing.T) {
	txs := acbSplitThenSell(t, acbHundredThirded(33_333_333))

	got := acbWalkAdjusted(t, []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-01", 1_000, 0)}, txs...)

	assert.Equal(t, []report.ACBAdjustmentIssue{
		{Kind: report.ACBAdjustmentNotHeld, Item: 1, SecurityID: "sec-1", Security: "XEQT", Date: dateOf(t, "2024-06-01")},
	}, got.AdjustmentIssues)
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 0}}, acbPositionRows(got))
	assert.Equal(t, int64(0), got.Years[0].ReturnOfCapitalGain)
	assert.False(t, got.Securities[0].Incomplete)
}

func Test_acb_holds_a_millionth_when_two_splits_cancel_and_a_sale_leaves_it(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 100*acbMillion, -300_000),
		acbSplitTx(t, 2, "acct-1", "2024-03-01", acbMillion, 3*acbMillion),
		acbSplitTx(t, 3, "acct-1", "2024-03-02", 3*acbMillion, acbMillion),
		acbSell(t, 4, "2024-04-01", 99_999_999),
	)

	assert.Empty(t, acbLastOversold(t, got))
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "1/1000000", ACB: 0}}, acbPositionRows(got))
	assert.False(t, got.Securities[0].Incomplete)
}

func Test_acb_closes_the_no_cost_span_when_a_sale_leaves_less_than_half_a_millionth(t *testing.T) {
	got := acbWalkOf(t,
		acbNoCostAdd(t, 1, "sec-1", "2024-01-02", 100*acbMillion),
		acbSplitTx(t, 2, "acct-1", "2024-03-01", acbMillion, 3*acbMillion),
		acbSell(t, 3, "2024-04-01", 33_333_333),
	)

	assert.False(t, got.Securities[0].Incomplete)
}

func Test_ACBSecurity_holds_nothing_when_its_shares_round_to_no_millionth_whatever_the_walk_emits(t *testing.T) {
	position := report.ACBSecurity{Shares: big.NewRat(1, 3*acbMillion), ACB: 500}

	assert.False(t, position.Holds())
	assert.Nil(t, position.PerShare())
}

func Test_acb_adds_the_whole_cost_of_a_buy_after_a_sale_left_less_than_half_a_millionth_short(t *testing.T) {
	const cost = 1_000_000_000
	txs := acbSplitThenSell(t, acbThousandSeventh(142_857_143))

	got := acbWalkOf(t, append(txs, acbTx(t, 4, "acct-1", "sec-1", "2024-05-01", store.ActionBuy, "CAD", 10*acbMillion, -cost))...)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: cost}}, acbPositionRows(got))
}

func Test_acb_removes_all_cost_when_a_sale_leaves_less_than_half_a_millionth(t *testing.T) {
	const cost = 1_000_000_000
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 100*acbMillion, -cost),
		acbSplitTx(t, 2, "acct-1", "2024-03-01", acbMillion, 3*acbMillion),
		acbSell(t, 3, "2024-04-01", 33_333_333),
	)

	assert.Equal(t, int64(cost), got.Years[0].Sales[0].ACBRemoved)
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 0}}, acbPositionRows(got))
	assert.Nil(t, got.Securities[0].PerShare())
}

func Test_acb_leaves_a_pool_flat_when_a_split_follows_a_sale_that_left_less_than_half_a_millionth(t *testing.T) {
	txs := acbSplitThenSell(t, acbThousandSeventh(142_857_143))

	got := acbWalkOf(t, append(txs, acbSplitTx(t, 4, "acct-1", "2024-05-01", 7*acbMillion, acbMillion))...)

	events := got.Securities[0].Events
	assert.Equal(t, "0", events[len(events)-1].Held.RatString())
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 0}}, acbPositionRows(got))
	assert.Equal(t, []bool{false}, acbSaleMarks(got))
	assert.False(t, got.Securities[0].Incomplete)
}

func Test_acb_keeps_the_acb_of_a_pool_a_split_rounds_to_no_millionth(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 1, -100),
		acbSplitTx(t, 2, "acct-1", "2024-03-01", acbMillion, 3*acbMillion),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 100}}, acbPositionRows(got))
	assert.False(t, got.Securities[0].Holds())
	assert.Nil(t, got.Securities[0].PerShare())
}

func Test_acb_holds_a_millionth_when_a_split_that_rounded_the_pool_to_none_is_undone(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 1, -100),
		acbSplitTx(t, 2, "acct-1", "2024-03-01", acbMillion, 3*acbMillion),
		acbSplitTx(t, 3, "acct-1", "2024-03-02", 3*acbMillion, acbMillion),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "1/1000000", ACB: 100}}, acbPositionRows(got))
}

func Test_acb_adds_a_later_buy_to_the_acb_a_split_left_on_a_pool_of_no_millionth(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 1, -100),
		acbSplitTx(t, 2, "acct-1", "2024-03-01", acbMillion, 3*acbMillion),
		acbTx(t, 3, "acct-1", "sec-1", "2024-04-01", store.ActionBuy, "CAD", 10*acbMillion, -500),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 600}}, acbPositionRows(got))
}

func Test_acb_closes_the_no_cost_span_when_a_split_rounds_the_pool_to_no_millionth(t *testing.T) {
	got := acbWalkOf(t,
		acbNoCostAdd(t, 1, "sec-1", "2024-01-02", 1),
		acbSplitTx(t, 2, "acct-1", "2024-03-01", acbMillion, 3*acbMillion),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 0}}, acbPositionRows(got))
	assert.False(t, got.Securities[0].Incomplete)
}

func Test_acb_adds_no_cost_when_a_buy_covers_a_short_and_leaves_less_than_half_a_millionth(t *testing.T) {
	txs := acbSplitThenSell(t, acbHundredThirded(33_333_334))

	got := acbWalkOf(t, append(txs, acbTx(t, 4, "acct-1", "sec-1", "2024-05-01", store.ActionBuy, "CAD", 1, -300))...)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 0}}, acbPositionRows(got))
}

func Test_acb_opens_no_unknown_cost_span_when_added_shares_cover_a_short_and_leave_less_than_half_a_millionth(t *testing.T) {
	txs := acbSplitThenSell(t, acbHundredThirded(33_333_334))

	got := acbWalkOf(t, append(txs, acbNoCostAdd(t, 4, "sec-1", "2024-05-01", 1))...)

	assert.False(t, got.Securities[0].Incomplete)
	assert.True(t, got.Securities[0].Events[3].UnknownCost)
}
