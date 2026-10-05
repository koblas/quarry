package report_test

import (
	"math/big"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_acb_dates_the_walk_as_of_today(t *testing.T) {
	got := acbWalkOf(t)

	assert.Equal(t, acbToday, got.AsOf)
}

func Test_acb_names_the_account_of_each_event_and_sale(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-2", "sec-1", "2024-01-03", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 3, "acct-2", "sec-1", "2024-02-02", store.ActionSell, "CAD", -4*acbMillion, 800),
	)

	require.Len(t, got.Securities, 1)
	accounts := make([][2]string, 0, len(got.Securities[0].Events))
	for _, event := range got.Securities[0].Events {
		accounts = append(accounts, [2]string{event.AccountID, event.Account})
	}
	assert.Equal(t, [][2]string{{"acct-1", "Margin"}, {"acct-2", "Old margin"}, {"acct-2", "Old margin"}}, accounts)
	sale := got.Years[0].Sales[0]
	assert.Equal(t, [2]string{"acct-2", "Old margin"}, [2]string{sale.AccountID, sale.Account})
}

func Test_acb_gives_a_usd_event_its_amount_currency_rate_and_cad(t *testing.T) {
	got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "USD")},
		[]store.Rate{acbRate(t, "2024-01-02", 1_300_000)},
		acbTx(t, 1, "acct-3", "sec-1", "2024-01-02", store.ActionBuy, "USD", 10*acbMillion, -10_000),
	)

	event := got.Securities[0].Events[0]
	assert.Equal(t, int64(-10_000), *event.Amount)
	assert.Equal(t, "USD", event.Currency)
	assert.Equal(t, int64(1_300_000), int64(event.Rate))
	assert.Equal(t, int64(-13_000), event.CAD)
}

func Test_acb_gives_a_cad_event_no_rate_even_when_rates_are_stored(t *testing.T) {
	got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "CAD")},
		[]store.Rate{acbRate(t, "2024-01-02", 1_300_000)},
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
	)

	event := got.Securities[0].Events[0]
	assert.Equal(t, int64(-10_000), *event.Amount)
	assert.Equal(t, "CAD", event.Currency)
	assert.Zero(t, event.Rate)
	assert.Equal(t, int64(-10_000), event.CAD)
}

func Test_acb_gives_a_usd_event_with_no_rate_on_file_a_zero_rate(t *testing.T) {
	got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "USD")}, nil,
		acbTx(t, 1, "acct-3", "sec-1", "2024-01-02", store.ActionBuy, "USD", 10*acbMillion, -10_000),
	)

	event := got.Securities[0].Events[0]
	assert.Equal(t, int64(-10_000), *event.Amount)
	assert.Zero(t, event.Rate)
	assert.Zero(t, event.CAD)
}

func Test_acb_marks_only_a_sale_realized_and_gives_it_outlays_in_cad(t *testing.T) {
	commission := int64(10_050)
	sell := acbTx(t, 2, "acct-3", "sec-1", "2024-01-10", store.ActionSell, "USD", -10*acbMillion, 12_000)
	sell.Commission = &commission

	got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "USD")},
		[]store.Rate{acbRate(t, "2024-01-02", 1_300_000), acbRate(t, "2024-01-10", 1_350_000)},
		acbTx(t, 1, "acct-3", "sec-1", "2024-01-02", store.ActionBuy, "USD", 10*acbMillion, -10_000),
		sell,
	)

	buyEvent, sellEvent := got.Securities[0].Events[0], got.Securities[0].Events[1]
	assert.False(t, buyEvent.Realized)
	assert.Nil(t, buyEvent.Outlays)
	assert.True(t, sellEvent.Realized)
	assert.Equal(t, int64(136), *sellEvent.Outlays)
	assert.Equal(t, int64(3_200), sellEvent.Gain)
	assert.Equal(t, int64(16_200), sellEvent.CAD)
}

func Test_acb_marks_a_break_even_sale_realized_with_a_zero_gain(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -10*acbMillion, 1_000),
	)

	sell := got.Securities[0].Events[1]
	assert.True(t, sell.Realized)
	assert.Zero(t, sell.Gain)
}

func Test_acb_gives_a_sale_without_a_commission_zero_outlays_not_none(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -10*acbMillion, 1_600),
	)

	require.NotNil(t, got.Securities[0].Events[1].Outlays)
	assert.Zero(t, *got.Securities[0].Events[1].Outlays)
}

func Test_acb_leaves_a_sale_of_costed_shares_unmarked_and_complete(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -4*acbMillion, 1_600),
	)

	assert.False(t, got.Securities[0].Incomplete)
	assert.False(t, got.Years[0].Sales[0].PossibleSuperficialLoss)
	assert.False(t, got.Years[0].Sales[0].UnknownCost)
}

func Test_millionths_rounds_a_share_count_half_away_from_zero(t *testing.T) {
	cases := []struct {
		name   string
		shares *big.Rat
		want   int64
	}{
		{name: "a whole count", shares: big.NewRat(5, 1), want: 5_000_000},
		{name: "under half a millionth rounds down", shares: big.NewRat(1, 3_000_000), want: 0},
		{name: "half a millionth rounds up", shares: big.NewRat(1, 2_000_000), want: 1},
		{name: "a third rounds down", shares: big.NewRat(1, 3), want: 333_333},
		{name: "two thirds round up", shares: big.NewRat(2, 3), want: 666_667},
		{name: "a negative half rounds away from zero", shares: big.NewRat(-1, 2_000_000), want: -1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.Millionths(c.shares))
		})
	}
}
