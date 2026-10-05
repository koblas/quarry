package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbSplitTx is a sec-1 split of newShares for oldShares, both in millionths.
func acbSplitTx(t *testing.T, sourceID int64, account, date string, newShares, oldShares int64) store.InvestmentTransaction {
	t.Helper()
	split := acbTx(t, sourceID, account, "sec-1", date, store.ActionSplit, "CAD", 0, 0)
	split.Shares, split.SplitNewShares, split.SplitOldShares = nil, &newShares, &oldShares
	return split
}

func Test_acb_converts_usd_at_the_rate_on_or_before_the_date(t *testing.T) {
	rates := []store.Rate{
		acbRate(t, "2024-01-02", 1_300_000),
		acbRate(t, "2024-01-10", 1_400_000),
		acbRate(t, "2024-01-20", 1_500_000),
	}
	cases := []struct {
		name    string
		date    string
		wantACB int64
	}{
		{name: "a rate dated that day", date: "2024-01-02", wantACB: 13_000},
		{name: "the latest earlier rate, a later one ignored", date: "2024-01-05", wantACB: 13_000},
		{name: "a second rate dated that day", date: "2024-01-10", wantACB: 14_000},
		{name: "the latest of several earlier rates", date: "2024-01-15", wantACB: 14_000},
		{name: "after the last rate", date: "2024-01-25", wantACB: 15_000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "USD")}, rates,
				acbTx(t, 1, "acct-3", "sec-1", c.date, store.ActionBuy, "USD", 10*acbMillion, -10_000),
			)

			assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: c.wantACB}}, acbPositionRows(got))
		})
	}
}

func Test_acb_converts_a_usd_sales_proceeds_and_outlays_each_at_its_rate(t *testing.T) {
	commission := int64(10_050)
	sell := acbTx(t, 2, "acct-3", "sec-1", "2024-01-10", store.ActionSell, "USD", 10*acbMillion, 12_000)
	sell.Commission = &commission

	got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "USD")},
		[]store.Rate{acbRate(t, "2024-01-02", 1_300_000), acbRate(t, "2024-01-10", 1_350_000)},
		acbTx(t, 1, "acct-3", "sec-1", "2024-01-02", store.ActionBuy, "USD", 10*acbMillion, -10_000),
		sell,
	)

	assert.Equal(t, []acbSaleRow{
		{Date: "2024-01-10", Security: "sec-1", Shares: "10", Proceeds: 16_336, Outlays: 136, ACBRemoved: 13_000, Gain: 3_200},
	}, acbSaleRows(got))
}

func Test_acb_splits_and_consolidates_shares_not_acb(t *testing.T) {
	buy := acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000)
	twoForOne := func(sourceID int64, account, date string) store.InvestmentTransaction {
		return acbSplitTx(t, sourceID, account, date, 2*acbMillion, acbMillion)
	}
	cases := []struct {
		name       string
		txs        []store.InvestmentTransaction
		wantShares string
	}{
		{name: "a two for one split doubles the units", txs: []store.InvestmentTransaction{twoForOne(2, "acct-1", "2024-02-01")}, wantShares: "20"},
		{name: "a one for two consolidation halves them", txs: []store.InvestmentTransaction{acbSplitTx(t, 2, "acct-1", "2024-02-01", acbMillion, 2*acbMillion)}, wantShares: "5"},
		{name: "a split recorded only in a registered account is not applied", txs: []store.InvestmentTransaction{twoForOne(2, "acct-9", "2024-02-01")}, wantShares: "10"},
		{
			name:       "a split recorded in two pool accounts applies once",
			txs:        []store.InvestmentTransaction{twoForOne(2, "acct-1", "2024-02-01"), twoForOne(3, "acct-2", "2024-02-01")},
			wantShares: "20",
		},
		{
			name: "the first of two different ratios on a date by source id wins",
			txs: []store.InvestmentTransaction{
				twoForOne(4, "acct-1", "2024-02-01"),
				acbSplitTx(t, 3, "acct-2", "2024-02-01", 3*acbMillion, acbMillion),
			},
			wantShares: "30",
		},
		{
			name:       "two splits on different dates both apply",
			txs:        []store.InvestmentTransaction{twoForOne(2, "acct-1", "2024-02-01"), twoForOne(3, "acct-1", "2024-03-01")},
			wantShares: "40",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, append([]store.InvestmentTransaction{buy}, c.txs...)...)

			assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: c.wantShares, ACB: 1_000}}, acbPositionRows(got))
		})
	}
}

func Test_acb_applies_a_days_buy_then_split_then_sale_in_that_order(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-02-01", store.ActionSell, "CAD", 10*acbMillion, 800),
		acbSplitTx(t, 2, "acct-1", "2024-02-01", 2*acbMillion, acbMillion),
		acbTx(t, 3, "acct-1", "sec-1", "2024-02-01", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
	)

	assert.Equal(t, []acbSaleRow{{Date: "2024-02-01", Security: "sec-1", Shares: "10", Proceeds: 800, ACBRemoved: 500, Gain: 300}}, acbSaleRows(got))
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 500}}, acbPositionRows(got))
}

func Test_acb_reinvests_at_quickens_cost(t *testing.T) {
	cost := int64(1_640)
	withCost := acbTx(t, 2, "acct-1", "sec-1", "2024-03-28", store.ActionReinvestDividend, "CAD", acbMillion/2, 0)
	withCost.CostBasis = &cost
	withoutCost := acbTx(t, 3, "acct-1", "sec-1", "2024-03-29", store.ActionReinvestDividend, "CAD", acbMillion/2, 0)
	buy := acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000)

	got := acbWalkOf(t, buy, withCost)
	gotNull := acbWalkOf(t, buy, withCost, withoutCost)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "21/2", ACB: 2_640}}, acbPositionRows(got))
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "11", ACB: 2_640}}, acbPositionRows(gotNull))
}

func Test_acb_walks_units_of_a_trade_it_cannot_convert(t *testing.T) {
	cases := []struct {
		name     string
		currency string
		rates    []store.Rate
	}{
		{name: "usd dated before the first rate", currency: "USD", rates: []store.Rate{acbRate(t, "2024-06-01", 1_300_000)}},
		{name: "usd with no rate at all", currency: "USD"},
		{name: "a currency that is neither", currency: "EUR", rates: []store.Rate{acbRate(t, "2024-01-01", 1_300_000)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", c.currency)}, c.rates,
				acbTx(t, 1, "acct-1", "sec-1", "2024-02-01", store.ActionBuy, c.currency, 10*acbMillion, -1_000),
			)

			require.Len(t, got.Securities, 1)
			assert.Equal(t, "10", got.Securities[0].Shares.RatString())
		})
	}
}

func Test_acb_orders_securities_by_name_ignoring_case_then_id(t *testing.T) {
	securities := []store.Security{
		acbSecurity("sec-8", "zeb", "CAD"),
		acbSecurity("sec-7", "ABD", "CAD"),
		acbSecurity("sec-2", "VTI", "CAD"),
		acbSecurity("sec-3", "abc", "CAD"),
		acbSecurity("sec-1", "vti", "CAD"),
	}
	txs := make([]store.InvestmentTransaction, 0, len(securities))
	for i, security := range securities {
		txs = append(txs, acbTx(t, int64(i+1), "acct-1", security.ID, "2024-01-02", store.ActionBuy, "CAD", acbMillion, -100))
	}

	got := acbWalkWith(t, securities, nil, txs...)

	ids := make([]string, 0, len(got.Securities))
	for _, position := range got.Securities {
		ids = append(ids, position.Security.ID)
	}
	assert.Equal(t, []string{"sec-3", "sec-7", "sec-1", "sec-2", "sec-8"}, ids)
}

type acbEventRow struct {
	ID, Action, Shares, Held string
	ACB, Gain                int64
}

func Test_acb_lists_each_securitys_events_and_position(t *testing.T) {
	cost := int64(1_640)
	reinvest := acbTx(t, 4, "acct-1", "sec-1", "2024-03-01", store.ActionReinvestDividend, "CAD", acbMillion/2, 0)
	reinvest.CostBasis = &cost

	got := acbWalkOf(t,
		acbTx(t, 2, "acct-2", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 2*acbMillion, -600),
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 2*acbMillion, -400),
		acbSplitTx(t, 3, "acct-1", "2024-02-01", 2*acbMillion, acbMillion),
		acbSplitTx(t, 5, "acct-2", "2024-02-01", 2*acbMillion, acbMillion),
		reinvest,
		acbTx(t, 6, "acct-1", "sec-1", "2024-04-01", store.ActionSell, "CAD", 2*acbMillion, 2_000),
	)

	require.Len(t, got.Securities, 1)
	events := make([]acbEventRow, 0, len(got.Securities[0].Events))
	for _, event := range got.Securities[0].Events {
		events = append(events, acbEventRow{
			ID: event.ID, Action: event.Action, Shares: event.Shares.RatString(), Held: event.Held.RatString(), ACB: event.ACB, Gain: event.Gain,
		})
	}
	assert.Equal(t, []acbEventRow{
		{ID: "itxn-2024-01-02-acct-1-sec-1-buy", Action: "buy", Shares: "2", Held: "2", ACB: 400},
		{ID: "itxn-2024-01-02-acct-2-sec-1-buy", Action: "buy", Shares: "2", Held: "4", ACB: 1_000},
		{ID: "itxn-2024-02-01-acct-1-sec-1-split", Action: "split", Shares: "0", Held: "8", ACB: 1_000},
		{ID: "itxn-2024-03-01-acct-1-sec-1-reinvest_dividend", Action: "reinvest_dividend", Shares: "1/2", Held: "17/2", ACB: 2_640},
		{ID: "itxn-2024-04-01-acct-1-sec-1-sell", Action: "sell", Shares: "2", Held: "13/2", ACB: 2_640 - 621, Gain: 2_000 - 621},
	}, events)
}

func Test_acb_gives_the_acb_per_share_in_dollars_and_none_for_an_empty_pool(t *testing.T) {
	held := acbWalkOf(t, acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 4*acbMillion, -1_000))
	soldOut := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 4*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", 4*acbMillion, 1_200),
	)

	assert.Equal(t, "5/2", held.Securities[0].PerShare().RatString())
	assert.Nil(t, soldOut.Securities[0].PerShare())
}
