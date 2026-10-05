package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbMixedSecurities is XEQT, whose USD trades the fixtures leave unconvertible, and VTI, which trades in CAD.
func acbMixedSecurities() []store.Security {
	return []store.Security{acbSecurity("sec-1", "XEQT", "USD"), acbSecurity("sec-2", "VTI", "CAD")}
}

// acbNoRateFixture is XEQT bought in USD on 2023-12-01 and part-sold on 2024-02-01 with a return of capital
// above its ACB, and VTI bought and part-sold in CAD, over a store whose first rate is dated firstRate.
func acbNoRateFixture(t *testing.T, firstRate string) report.ACB {
	t.Helper()
	adjustment := report.ACBAdjustment{SecurityID: "sec-1", Date: dateOf(t, "2024-03-01"), ReturnOfCapital: 50_000}

	return acbWalkRequest(t, acbMixedSecurities(), []store.Rate{acbRate(t, firstRate, 1_300_000)}, []report.ACBAdjustment{adjustment},
		acbTx(t, 1, "acct-3", "sec-1", "2023-12-01", store.ActionBuy, "USD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-3", "sec-1", "2024-02-01", store.ActionSell, "USD", -5*acbMillion, 2_000),
		acbTx(t, 3, "acct-1", "sec-2", "2024-01-05", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 4, "acct-1", "sec-2", "2024-02-02", store.ActionSell, "CAD", -5*acbMillion, 800),
	)
}

func Test_acb_leaves_a_no_rate_securitys_sales_and_excess_out_of_the_years(t *testing.T) {
	got := acbNoRateFixture(t, "2024-01-02")

	assert.Equal(t, []acbYearRow{{
		Year:     2024,
		Sales:    []acbSaleRow{{Date: "2024-02-02", Security: "sec-2", Shares: "5", Proceeds: 800, ACBRemoved: 500, Gain: 300}},
		Proceeds: 800, ACBRemoved: 500, Gain: 300,
	}}, acbYearRows(got))
}

func Test_acb_keeps_the_events_of_a_no_rate_security_with_their_realized_gain(t *testing.T) {
	got := acbNoRateFixture(t, "2024-01-02")

	events := got.Securities[1].Events
	require.Len(t, events, 3)
	excess := events[2]
	assert.Equal(t, report.ACBActionReturnOfCapital, excess.Action)
	assert.True(t, excess.Realized)
	assert.Equal(t, int64(50_000), excess.Gain)
}

func Test_acb_counts_a_usd_securitys_sales_and_excess_when_every_trade_has_a_rate(t *testing.T) {
	got := acbNoRateFixture(t, "2023-11-01")

	assert.Equal(t, []acbYearRow{{
		Year: 2024,
		Sales: []acbSaleRow{
			{Date: "2024-02-01", Security: "sec-1", Shares: "5", Proceeds: 2_600, ACBRemoved: 650, Gain: 1_950},
			{Date: "2024-02-02", Security: "sec-2", Shares: "5", Proceeds: 800, ACBRemoved: 500, Gain: 300},
		},
		Proceeds: 3_400, ACBRemoved: 1_150, Gain: 2_250, ReturnOfCapitalGain: 49_350,
	}}, acbYearRows(got))
}

func Test_acb_flags_a_no_rate_security_incomplete_while_it_is_held(t *testing.T) {
	got := acbNoRateFixture(t, "2024-01-02")

	vti, xeqt := got.Securities[0], got.Securities[1]
	assert.Equal(t, "5", xeqt.Shares.RatString())
	assert.True(t, xeqt.Incomplete)
	assert.False(t, vti.Incomplete)
}

func Test_acb_dates_the_first_rate_of_the_store(t *testing.T) {
	got := acbNoRateFixture(t, "2024-01-02")

	assert.Equal(t, dateOf(t, "2024-01-02"), got.FirstRate)
	assert.True(t, acbWalkOf(t).FirstRate.IsZero())
}

func Test_acb_names_the_earliest_trade_it_cannot_convert(t *testing.T) {
	firstRate := []store.Rate{acbRate(t, "2024-01-02", 1_300_000)}
	cases := []struct {
		name     string
		currency string
		date     string
		rates    []store.Rate
		want     *report.ACBNoRate
	}{
		{name: "a usd trade the day before the first rate is one", currency: "USD", date: "2024-01-01", rates: firstRate, want: &report.ACBNoRate{Date: dateOf(t, "2024-01-01"), Currency: "USD"}},
		{name: "a usd trade on the first rate date is not", currency: "USD", date: "2024-01-02", rates: firstRate},
		{name: "a usd trade with no rate in the store is one", currency: "USD", date: "2024-02-01", want: &report.ACBNoRate{Date: dateOf(t, "2024-02-01"), Currency: "USD"}},
		{name: "a cad trade with no rate in the store is not", currency: "CAD", date: "2024-02-01"},
		{name: "a euro trade is one", currency: "EUR", date: "2024-02-01", rates: firstRate, want: &report.ACBNoRate{Date: dateOf(t, "2024-02-01"), Currency: "EUR"}},
		{name: "a trade with no currency code is one", currency: "", date: "2024-02-01", rates: firstRate, want: &report.ACBNoRate{Date: dateOf(t, "2024-02-01")}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkWith(t, acbMixedSecurities(), c.rates,
				acbTx(t, 1, "acct-3", "sec-1", c.date, store.ActionBuy, c.currency, 10*acbMillion, -1_000),
			)

			assert.Equal(t, c.want, got.Securities[0].NoRate)
		})
	}
}

func Test_acb_names_the_first_of_two_trades_it_cannot_convert(t *testing.T) {
	rates := []store.Rate{acbRate(t, "2024-01-02", 1_300_000)}

	got := acbWalkWith(t, acbMixedSecurities(), rates,
		acbTx(t, 2, "acct-3", "sec-1", "2023-12-15", store.ActionBuy, "USD", 10*acbMillion, -1_000),
		acbTx(t, 1, "acct-3", "sec-1", "2023-12-01", store.ActionBuy, "USD", 10*acbMillion, -1_000),
	)

	assert.Equal(t, &report.ACBNoRate{Date: dateOf(t, "2023-12-01"), Currency: "USD"}, got.Securities[0].NoRate)
}

func Test_acb_counts_only_a_trade_that_carries_a_value_as_one_it_cannot_convert(t *testing.T) {
	usd := func(tx store.InvestmentTransaction) store.InvestmentTransaction {
		tx.Currency = "USD"
		return tx
	}
	cost := int64(1_000)
	const date = "2023-12-01"
	cases := []struct {
		name string
		tx   store.InvestmentTransaction
		want bool
	}{
		{name: "a buy", tx: acbTx(t, 1, "acct-3", "sec-1", date, store.ActionBuy, "USD", 10*acbMillion, -1_000), want: true},
		{name: "a sale", tx: acbTx(t, 1, "acct-3", "sec-1", date, store.ActionSell, "USD", -5*acbMillion, 2_000), want: true},
		{name: "added shares with a cost", tx: usd(acbCosted(acbNoCostAdd(t, 1, "sec-1", date, 10*acbMillion), cost)), want: true},
		{name: "a reinvested dividend with a cost", tx: usd(acbCosted(acbReinvest(t, 1, date, 10*acbMillion), cost)), want: true},
		{name: "added shares with no cost", tx: usd(acbNoCostAdd(t, 1, "sec-1", date, 10*acbMillion))},
		{name: "a reinvested dividend with no cost", tx: usd(acbReinvest(t, 1, date, 10*acbMillion))},
		{name: "a removal of shares", tx: acbTx(t, 1, "acct-3", "sec-1", date, store.ActionRemoveShares, "USD", -10*acbMillion, 0)},
		{name: "a split", tx: usd(acbSplitTx(t, 1, "acct-3", date, 2*acbMillion, acbMillion))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkWith(t, acbMixedSecurities(), []store.Rate{acbRate(t, "2024-01-02", 1_300_000)},
				acbTx(t, 0, "acct-1", "sec-1", "2023-11-01", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
				c.tx,
			)

			assert.Equal(t, c.want, got.Securities[0].NoRate != nil)
		})
	}
}

func Test_acb_keeps_a_sold_out_security_out_of_the_years_after_it_is_bought_again_with_a_rate(t *testing.T) {
	got := acbWalkWith(t, acbMixedSecurities(), []store.Rate{acbRate(t, "2024-01-02", 1_300_000)},
		acbTx(t, 1, "acct-3", "sec-1", "2023-12-01", store.ActionBuy, "USD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-3", "sec-1", "2023-12-15", store.ActionSell, "USD", -10*acbMillion, 2_000),
		acbTx(t, 3, "acct-3", "sec-1", "2024-02-01", store.ActionBuy, "USD", 10*acbMillion, -1_000),
		acbTx(t, 4, "acct-3", "sec-1", "2024-03-01", store.ActionSell, "USD", -10*acbMillion, 2_000),
	)

	assert.Empty(t, got.Years)
	assert.True(t, got.Securities[0].Incomplete)
	assert.Zero(t, got.Securities[0].Shares.Sign())
}

// acbUnvaluedEvents is whether each of security's events could not be converted to CAD, in event order.
func acbUnvaluedEvents(security report.ACBSecurity) []bool {
	unvalued := make([]bool, 0, len(security.Events))
	for _, event := range security.Events {
		unvalued = append(unvalued, event.Unvalued)
	}

	return unvalued
}

func Test_acb_marks_an_event_unvalued_only_when_it_could_not_be_converted(t *testing.T) {
	got := acbWalkWith(t, acbMixedSecurities(), []store.Rate{acbRate(t, "2024-01-02", 1_300_000)},
		acbTx(t, 1, "acct-3", "sec-1", "2023-12-01", store.ActionBuy, "USD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-3", "sec-1", "2024-02-01", store.ActionBuy, "USD", 10*acbMillion, -1_000),
		acbTx(t, 3, "acct-1", "sec-1", "2024-02-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
	)

	assert.Equal(t, []bool{true, false, false}, acbUnvaluedEvents(got.Securities[0]))
}

func Test_acb_leaves_a_sale_out_of_the_years_only_when_it_could_not_be_converted(t *testing.T) {
	cases := []struct {
		name         string
		saleDate     string
		wantYears    int
		wantUnvalued []bool
		wantNoRate   bool
	}{
		{name: "a usd sale the day before the first rate is left out", saleDate: "2023-12-01", wantUnvalued: []bool{false, true}, wantNoRate: true},
		{name: "a usd sale on the first rate date is counted", saleDate: "2024-01-02", wantYears: 1, wantUnvalued: []bool{false, false}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkWith(t, acbMixedSecurities(), []store.Rate{acbRate(t, "2024-01-02", 1_300_000)},
				acbTx(t, 1, "acct-1", "sec-1", "2023-11-01", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
				acbTx(t, 2, "acct-3", "sec-1", c.saleDate, store.ActionSell, "USD", -5*acbMillion, 2_000),
			)

			assert.Len(t, got.Years, c.wantYears)
			assert.Equal(t, c.wantUnvalued, acbUnvaluedEvents(got.Securities[0]))
			assert.Equal(t, c.wantNoRate, got.Securities[0].Incomplete)
		})
	}
}

func Test_acb_leaves_a_no_rate_securitys_return_of_capital_excess_out_of_the_years(t *testing.T) {
	adjustment := report.ACBAdjustment{SecurityID: "sec-1", Date: dateOf(t, "2024-03-01"), ReturnOfCapital: 50_000}

	got := acbWalkRequest(t, acbMixedSecurities(), []store.Rate{acbRate(t, "2024-01-02", 1_300_000)}, []report.ACBAdjustment{adjustment},
		acbTx(t, 1, "acct-3", "sec-1", "2023-12-01", store.ActionBuy, "USD", 10*acbMillion, -1_000),
	)

	require.Len(t, got.Securities[0].Events, 2)
	assert.True(t, got.Securities[0].Events[1].Realized)
	assert.Empty(t, got.Years)
}

func Test_acb_marks_a_loss_superficial_only_when_its_security_has_a_rate_for_every_trade(t *testing.T) {
	cases := []struct {
		name      string
		firstRate string
		wantMarks []bool
	}{
		{name: "a no-rate loss sale rebought under its ticker is in no year, so marked nowhere", firstRate: "2024-01-02"},
		{name: "the same loss sale with a rate is marked", firstRate: "2023-11-01", wantMarks: []bool{true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			securities := []store.Security{acbSecurity("sec-1", "XEQT", "USD"), acbSecurity("sec-2", "XEQT", "CAD")}

			got := acbWalkWith(t, securities, []store.Rate{acbRate(t, c.firstRate, 1_300_000)},
				acbTx(t, 1, "acct-1", "sec-1", "2023-11-01", store.ActionBuy, "CAD", 10*acbMillion, -100_000),
				acbTx(t, 2, "acct-3", "sec-1", "2023-12-01", store.ActionSell, "USD", -5*acbMillion, 20_000),
				acbTx(t, 3, "acct-1", "sec-2", "2023-12-10", store.ActionBuy, "CAD", 10*acbMillion, -50_000),
			)

			assert.Equal(t, c.wantMarks, acbFlags(got))
		})
	}
}
