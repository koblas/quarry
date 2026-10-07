package report_test

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_acb_pools_each_security_across_non_registered_accounts(t *testing.T) {
	commission999, commission495, commission100 := int64(99_900), int64(49_450), int64(10_000)
	xeqtBuy := acbTx(t, 1, "acct-1", "sec-1", "2023-03-01", store.ActionBuy, "CAD", 100*acbMillion, -250_999)
	xeqtBuy.Commission = &commission999
	xeqtSell2023 := acbTx(t, 10, "acct-1", "sec-1", "2023-06-15", store.ActionSell, "CAD", -30*acbMillion, 80_000)
	xeqtSell2023.Commission = &commission495
	xeqtSell2024 := acbTx(t, 3, "acct-2", "sec-1", "2024-09-03", store.ActionSell, "CAD", -100*acbMillion, 330_000)
	xeqtSell2024.Commission = &commission999
	vtiSell := acbTx(t, 7, "acct-3", "sec-2", "2024-04-02", store.ActionSell, "USD", -4*acbMillion, 90_000)
	vtiSell.Commission = &commission100
	srv := report.NewServer(report.WithStore(fakeStore{history: store.InvestmentHistory{
		Accounts:   acbAccounts(),
		Securities: []store.Security{acbSecurity("sec-1", "XEQT", "CAD"), acbSecurity("sec-2", "VTI", "USD")},
		Transactions: []store.InvestmentTransaction{
			acbTx(t, 20, "acct-9", "sec-1", "2023-05-01", store.ActionBuy, "CAD", 50*acbMillion, -125_000),
			xeqtBuy,
			acbTx(t, 11, "acct-2", "sec-1", "2023-06-15", store.ActionBuy, "CAD", 100*acbMillion, -260_000),
			xeqtSell2023,
			acbTx(t, 4, "acct-3", "sec-2", "2024-01-02", store.ActionBuy, "USD", 10*acbMillion, -200_137),
			acbTx(t, 5, "acct-1", "sec-1", "2024-02-10", store.ActionSell, "CAD", -50*acbMillion, 160_000),
			acbTx(t, 21, "acct-9", "sec-1", "2024-03-01", store.ActionSell, "CAD", -50*acbMillion, 150_000),
			vtiSell,
			xeqtSell2024,
		},
		Rates: []store.Rate{
			acbRate(t, "2023-12-29", 1_320_000),
			acbRate(t, "2024-01-03", 1_330_000),
			acbRate(t, "2024-04-02", 1_355_000),
		},
	}}))

	got, err := srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday})

	require.NoError(t, err)
	assert.Equal(t, []acbYearRow{
		{
			Year:     2023,
			Sales:    []acbSaleRow{{Date: "2023-06-15", Security: "sec-1", Shares: "30", Proceeds: 80_495, Outlays: 495, ACBRemoved: 76_650, Gain: 3_350}},
			Proceeds: 80_495, Outlays: 495, ACBRemoved: 76_650, Gain: 3_350,
		},
		{
			Year: 2024,
			Sales: []acbSaleRow{
				{Date: "2024-02-10", Security: "sec-1", Shares: "50", Proceeds: 160_000, Outlays: 0, ACBRemoved: 127_750, Gain: 32_250},
				{Date: "2024-04-02", Security: "sec-2", Shares: "4", Proceeds: 122_086, Outlays: 136, ACBRemoved: 105_672, Gain: 16_278},
				{Date: "2024-09-03", Security: "sec-1", Shares: "100", Proceeds: 330_999, Outlays: 999, ACBRemoved: 255_499, Gain: 74_501},
			},
			Proceeds: 613_085, Outlays: 1_135, ACBRemoved: 488_921, Gain: 123_029,
		},
	}, acbYearRows(got))
	assert.Equal(t, []acbPositionRow{
		{Name: "VTI", Shares: "6", ACB: 158_509},
		{Name: "XEQT", Shares: "20", ACB: 51_100},
	}, acbPositionRows(got))
}

func Test_acb_adds_a_reinvested_dividends_cost_and_splits_shares_once(t *testing.T) {
	reinvestCost := int64(1_640)
	reinvest := acbTx(t, 3, "acct-1", "sec-5", "2024-03-28", store.ActionReinvestDividend, "CAD", acbMillion/2, 0)
	reinvest.CostBasis = &reinvestCost
	splitNew, splitOld := int64(2*acbMillion), int64(acbMillion)
	splitIn1 := acbTx(t, 4, "acct-1", "sec-5", "2024-05-01", store.ActionSplit, "CAD", 0, 0)
	splitIn1.Shares, splitIn1.SplitNewShares, splitIn1.SplitOldShares = nil, &splitNew, &splitOld
	splitIn2 := acbTx(t, 5, "acct-2", "sec-5", "2024-05-01", store.ActionSplit, "CAD", 0, 0)
	splitIn2.Shares, splitIn2.SplitNewShares, splitIn2.SplitOldShares = nil, &splitNew, &splitOld
	srv := report.NewServer(report.WithStore(fakeStore{history: store.InvestmentHistory{
		Accounts:   acbAccounts(),
		Securities: []store.Security{acbSecurity("sec-5", "ZEB", "CAD")},
		Transactions: []store.InvestmentTransaction{
			acbTx(t, 1, "acct-1", "sec-5", "2024-01-10", store.ActionBuy, "CAD", 10*acbMillion, -30_000),
			acbTx(t, 2, "acct-2", "sec-5", "2024-01-10", store.ActionBuy, "CAD", 10*acbMillion, -31_000),
			reinvest,
			splitIn1,
			splitIn2,
			acbTx(t, 6, "acct-2", "sec-5", "2024-06-03", store.ActionSell, "CAD", -(10*acbMillion + acbMillion/4), 20_000),
		},
	}}))

	got, err := srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday})

	require.NoError(t, err)
	assert.Equal(t, []acbYearRow{{
		Year:     2024,
		Sales:    []acbSaleRow{{Date: "2024-06-03", Security: "sec-5", Shares: "41/4", Proceeds: 20_000, Outlays: 0, ACBRemoved: 15_660, Gain: 4_340}},
		Proceeds: 20_000, ACBRemoved: 15_660, Gain: 4_340,
	}}, acbYearRows(got))
	assert.Equal(t, []acbPositionRow{{Name: "ZEB", Shares: "123/4", ACB: 46_980}}, acbPositionRows(got))
}

// acbAdjustment is an adjustment of sec-1 or sec-2 on date, in cents; 0 leaves that kind out.
func acbAdjustment(t *testing.T, security, date string, returnOfCapital, reinvestedDistribution int64) report.ACBAdjustment {
	t.Helper()
	return report.ACBAdjustment{
		SecurityID: security, Date: dateOf(t, date), ReturnOfCapital: returnOfCapital, ReinvestedDistribution: reinvestedDistribution,
	}
}

// acbBuyOfTen is ten shares of sec-1 in acct-1 on 2024-01-02 for 10.00, so its ACB is 1,000 cents.
func acbBuyOfTen(t *testing.T) store.InvestmentTransaction {
	t.Helper()
	return acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000)
}

type acbAdjustmentRow struct {
	Action   string
	CAD, ACB int64
	Gain     int64
	Realized bool
}

// acbAdjustmentRows is the adjustment events of the first security's history; those have no transaction id.
func acbAdjustmentRows(t *testing.T, result report.ACB) []acbAdjustmentRow {
	t.Helper()
	require.NotEmpty(t, result.Securities)
	var rows []acbAdjustmentRow
	for _, event := range result.Securities[0].Events {
		if event.ID == "" {
			rows = append(rows, acbAdjustmentRow{Action: event.Action, CAD: event.CAD, ACB: event.ACB, Gain: event.Gain, Realized: event.Realized})
		}
	}
	return rows
}

func acbActions(result report.ACB) []string {
	actions := make([]string, 0, len(result.Securities[0].Events))
	for _, event := range result.Securities[0].Events {
		actions = append(actions, event.Action)
	}
	return actions
}

func Test_acb_raises_the_acb_by_a_reinvested_distribution(t *testing.T) {
	got := acbWalkAdjusted(t, []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 0, 250)}, acbBuyOfTen(t))

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 1_250}}, acbPositionRows(got))
	assert.Equal(t, []acbAdjustmentRow{{Action: "reinvested distribution", CAD: -250, ACB: 1_250}}, acbAdjustmentRows(t, got))
	assert.Empty(t, got.AdjustmentIssues)
}

func Test_acb_lowers_the_acb_by_a_return_of_capital(t *testing.T) {
	got := acbWalkAdjusted(t, []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 300, 0)}, acbBuyOfTen(t))

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 700}}, acbPositionRows(got))
	assert.Equal(t, []acbAdjustmentRow{{Action: "return of capital", CAD: 300, ACB: 700}}, acbAdjustmentRows(t, got))
}

func Test_acb_describes_an_adjustment_event_as_moving_no_shares_and_belonging_to_no_transaction(t *testing.T) {
	got := acbWalkAdjusted(t, []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 0, 250)}, acbBuyOfTen(t))

	event := got.Securities[0].Events[1]
	assert.Equal(t, report.ACBEvent{
		Date: dateOf(t, "2024-06-30"), Action: "reinvested distribution", Shares: event.Shares, CAD: -250, Held: event.Held, ACB: 1_250,
	}, event)
	assert.Equal(t, "0", event.Shares.RatString())
	assert.Equal(t, "10", event.Held.RatString())
}

func Test_acb_applies_both_amounts_of_one_item(t *testing.T) {
	got := acbWalkAdjusted(t, []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 50, 200)}, acbBuyOfTen(t))

	assert.Equal(t, []acbAdjustmentRow{
		{Action: "reinvested distribution", CAD: -200, ACB: 1_200},
		{Action: "return of capital", CAD: 50, ACB: 1_150},
	}, acbAdjustmentRows(t, got))
	assert.Empty(t, got.AdjustmentIssues)
}

func Test_acb_orders_a_days_buy_split_distribution_return_of_capital_then_sale_whatever_their_source_ids(t *testing.T) {
	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{
			acbAdjustment(t, "sec-1", "2024-02-01", 50, 0),
			acbAdjustment(t, "sec-1", "2024-02-01", 0, 100),
		},
		acbTx(t, 10, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 1, "acct-1", "sec-1", "2024-02-01", store.ActionSell, "CAD", -5*acbMillion, 3_000),
		acbSplitTx(t, 2, "acct-1", "2024-02-01", 2*acbMillion, acbMillion),
		acbTx(t, 3, "acct-1", "sec-1", "2024-02-01", store.ActionBuy, "CAD", 2*acbMillion, -400),
	)

	assert.Equal(t, []string{"buy", "buy", "split", "reinvested distribution", "return of capital", "sell"}, acbActions(got))
	assert.Equal(t, []acbSaleRow{{Date: "2024-02-01", Security: "sec-1", Shares: "5", Proceeds: 3_000, ACBRemoved: 302, Gain: 2_698}}, acbSaleRows(got))
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "19", ACB: 1_148}}, acbPositionRows(got))
}

func Test_acb_applies_a_days_reinvested_distributions_before_its_returns_of_capital_in_item_order(t *testing.T) {
	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{
			acbAdjustment(t, "sec-1", "2024-06-30", 100, 0),
			acbAdjustment(t, "sec-1", "2024-06-30", 30, 80),
		},
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -50),
	)

	assert.Equal(t, []acbAdjustmentRow{
		{Action: "reinvested distribution", CAD: -80, ACB: 130},
		{Action: "return of capital", CAD: 100, ACB: 30},
		{Action: "return of capital", CAD: 30, ACB: 0},
	}, acbAdjustmentRows(t, got))
}

func Test_acb_applies_adjustments_in_date_order_whatever_their_item_order(t *testing.T) {
	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{
			acbAdjustment(t, "sec-1", "2024-08-31", 600, 0),
			acbAdjustment(t, "sec-1", "2024-06-30", 0, 400),
		},
		acbBuyOfTen(t),
	)

	assert.Equal(t, []acbAdjustmentRow{
		{Action: "reinvested distribution", CAD: -400, ACB: 1_400},
		{Action: "return of capital", CAD: 600, ACB: 800},
	}, acbAdjustmentRows(t, got))
}

func Test_acb_leaves_out_an_adjustment_after_today(t *testing.T) {
	tomorrow := acbToday.AddDate(0, 0, 1).Format(time.DateOnly)
	today := acbToday.Format(time.DateOnly)

	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{acbAdjustment(t, "sec-1", tomorrow, 300, 0), acbAdjustment(t, "sec-1", today, 0, 50)},
		acbBuyOfTen(t),
	)

	assert.Equal(t, []acbAdjustmentRow{{Action: "reinvested distribution", CAD: -50, ACB: 1_050}}, acbAdjustmentRows(t, got))
	assert.Empty(t, got.AdjustmentIssues)
}

func Test_acb_counts_only_return_of_capital_above_the_acb_as_a_gain(t *testing.T) {
	cases := []struct {
		name         string
		cents        int64
		wantACB      int64
		wantGain     int64
		wantRealized bool
		wantYears    []acbYearRow
	}{
		{name: "a cent under the acb leaves a cent", cents: 999, wantACB: 1, wantYears: []acbYearRow{}},
		{name: "exactly the acb leaves none and no gain", cents: 1_000, wantYears: []acbYearRow{}},
		{
			name: "a cent over the acb is a cent of gain", cents: 1_001, wantGain: 1, wantRealized: true,
			wantYears: []acbYearRow{{Year: 2024, ReturnOfCapitalGain: 1}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkAdjusted(t, []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", c.cents, 0)}, acbBuyOfTen(t))

			assert.Equal(t, []acbAdjustmentRow{{Action: "return of capital", CAD: c.cents, ACB: c.wantACB, Gain: c.wantGain, Realized: c.wantRealized}}, acbAdjustmentRows(t, got))
			assert.Equal(t, c.wantACB, got.Securities[0].ACB)
			assert.Equal(t, c.wantYears, acbYearRows(got))
		})
	}
}

func Test_acb_counts_a_whole_return_of_capital_as_a_gain_on_shares_with_no_acb(t *testing.T) {
	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 500, 0)},
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionAddShares, "CAD", 10*acbMillion, 0),
	)

	assert.Equal(t, []acbAdjustmentRow{{Action: "return of capital", CAD: 500, ACB: 0, Gain: 500, Realized: true}}, acbAdjustmentRows(t, got))
	assert.Equal(t, []acbYearRow{{Year: 2024, ReturnOfCapitalGain: 500}}, acbYearRows(got))
}

func Test_acb_gives_a_return_of_capital_gain_no_outlays(t *testing.T) {
	got := acbWalkAdjusted(t, []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 1_500, 0)}, acbBuyOfTen(t))

	assert.Nil(t, got.Securities[0].Events[1].Outlays)
}

func Test_acb_lists_a_year_with_only_a_return_of_capital_gain(t *testing.T) {
	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{acbAdjustment(t, "sec-1", "2023-06-30", 1_500, 0)},
		acbTx(t, 1, "acct-1", "sec-1", "2022-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -2_000),
		acbTx(t, 3, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -10*acbMillion, 2_500),
	)

	assert.Equal(t, []acbYearRow{
		{Year: 2023, ReturnOfCapitalGain: 500},
		{
			Year: 2024, Sales: []acbSaleRow{{Date: "2024-02-02", Security: "sec-1", Shares: "10", Proceeds: 2_500, ACBRemoved: 1_000, Gain: 1_500}},
			Proceeds: 2_500, ACBRemoved: 1_000, Gain: 1_500,
		},
	}, acbYearRows(got))
}

func Test_acb_keeps_a_years_sales_gain_apart_from_its_return_of_capital_gain(t *testing.T) {
	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 3_500, 0)},
		acbBuyOfTen(t),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionBuy, "CAD", 10*acbMillion, -2_000),
		acbTx(t, 3, "acct-1", "sec-1", "2024-09-02", store.ActionSell, "CAD", -5*acbMillion, 900),
	)

	assert.Equal(t, []acbYearRow{{
		Year: 2024, Sales: []acbSaleRow{{Date: "2024-09-02", Security: "sec-1", Shares: "5", Proceeds: 900, ACBRemoved: 0, Gain: 900}},
		Proceeds: 900, Gain: 900, ReturnOfCapitalGain: 500,
	}}, acbYearRows(got))
}

func Test_acb_sums_a_years_returns_of_capital_above_the_acb_across_securities(t *testing.T) {
	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{
			acbAdjustment(t, "sec-1", "2024-06-30", 1_200, 0),
			acbAdjustment(t, "sec-2", "2024-07-31", 450, 0),
		},
		acbBuyOfTen(t),
		acbTx(t, 2, "acct-1", "sec-2", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -300),
	)

	assert.Equal(t, []acbYearRow{{Year: 2024, ReturnOfCapitalGain: 350}}, acbYearRows(got))
}

func Test_acb_skips_an_adjustment_for_a_security_not_in_the_store(t *testing.T) {
	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{acbAdjustment(t, "sec-99", "2024-06-30", 300, 0), acbAdjustment(t, "sec-1", "2024-06-30", 0, 50)},
		acbBuyOfTen(t),
	)

	assert.Equal(t, []report.ACBAdjustmentIssue{
		{Kind: report.ACBAdjustmentUnknownSecurity, Item: 1, SecurityID: "sec-99", Date: dateOf(t, "2024-06-30")},
	}, got.AdjustmentIssues)
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 1_050}}, acbPositionRows(got))
}

func Test_acb_skips_an_adjustment_no_non_registered_account_holds(t *testing.T) {
	notHeld := func(date string) []report.ACBAdjustmentIssue {
		return []report.ACBAdjustmentIssue{{Kind: report.ACBAdjustmentNotHeld, Item: 1, SecurityID: "sec-1", Security: "XEQT", Date: dateOf(t, date)}}
	}
	sellOut := acbTx(t, 2, "acct-1", "sec-1", "2024-02-01", store.ActionSell, "CAD", -10*acbMillion, 1_500)
	cases := []struct {
		name        string
		date        string
		txs         []store.InvestmentTransaction
		wantIssues  []report.ACBAdjustmentIssue
		wantActions []string
	}{
		{
			name: "before the first buy", date: "2024-02-01", wantIssues: notHeld("2024-02-01"), wantActions: []string{"buy"},
			txs: []store.InvestmentTransaction{acbTx(t, 1, "acct-1", "sec-1", "2024-03-01", store.ActionBuy, "CAD", 10*acbMillion, -1_000)},
		},
		{
			name: "only a registered account holds the security", date: "2024-02-01", wantIssues: notHeld("2024-02-01"),
			txs: []store.InvestmentTransaction{acbTx(t, 1, "acct-9", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000)},
		},
		{
			name: "after selling out", date: "2024-03-01", wantIssues: notHeld("2024-03-01"), wantActions: []string{"buy", "sell"},
			txs: []store.InvestmentTransaction{acbBuyOfTen(t), sellOut},
		},
		{
			name: "on the day of the first buy", date: "2024-01-02", wantActions: []string{"buy", "reinvested distribution"},
			txs: []store.InvestmentTransaction{acbBuyOfTen(t)},
		},
		{
			name: "on the day the shares are sold out", date: "2024-02-01", wantActions: []string{"buy", "reinvested distribution", "sell"},
			txs: []store.InvestmentTransaction{acbBuyOfTen(t), sellOut},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkAdjusted(t, []report.ACBAdjustment{acbAdjustment(t, "sec-1", c.date, 0, 100)}, c.txs...)

			assert.Equal(t, c.wantIssues, got.AdjustmentIssues)
			assert.Equal(t, c.wantActions, acbActionsOf(got))
		})
	}
}

// acbActionsOf is the actions of the first security's events, nil when there is no security.
func acbActionsOf(result report.ACB) []string {
	if len(result.Securities) == 0 {
		return nil
	}
	return acbActions(result)
}

func Test_acb_names_a_repeated_adjustment_with_the_first_item(t *testing.T) {
	repeated := func(item, first int) report.ACBAdjustmentIssue {
		return report.ACBAdjustmentIssue{
			Kind: report.ACBAdjustmentRepeated, Item: item, First: first, SecurityID: "sec-1", Security: "XEQT", Date: dateOf(t, "2024-06-30"),
		}
	}
	vtiBuy := acbTx(t, 2, "acct-1", "sec-2", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000)
	cases := []struct {
		name        string
		adjustments []report.ACBAdjustment
		txs         []store.InvestmentTransaction
		want        []report.ACBAdjustmentIssue
	}{
		{
			name: "a pair", txs: []store.InvestmentTransaction{acbBuyOfTen(t)},
			adjustments: []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 0, 10), acbAdjustment(t, "sec-1", "2024-06-30", 5, 0)},
			want:        []report.ACBAdjustmentIssue{repeated(2, 1)},
		},
		{
			name: "a triple pairs each later item with the first", txs: []store.InvestmentTransaction{acbBuyOfTen(t)},
			adjustments: []report.ACBAdjustment{
				acbAdjustment(t, "sec-1", "2024-06-30", 0, 10), acbAdjustment(t, "sec-1", "2024-06-30", 5, 0), acbAdjustment(t, "sec-1", "2024-06-30", 0, 7),
			},
			want: []report.ACBAdjustmentIssue{repeated(2, 1), repeated(3, 1)},
		},
		{
			name: "both skipped as not held", txs: []store.InvestmentTransaction{acbBuyOfTen(t)},
			adjustments: []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-01-01", 0, 10), acbAdjustment(t, "sec-1", "2024-01-01", 5, 0)},
			want: []report.ACBAdjustmentIssue{
				{Kind: report.ACBAdjustmentNotHeld, Item: 1, SecurityID: "sec-1", Security: "XEQT", Date: dateOf(t, "2024-01-01")},
				{Kind: report.ACBAdjustmentNotHeld, Item: 2, SecurityID: "sec-1", Security: "XEQT", Date: dateOf(t, "2024-01-01")},
			},
		},
		{
			name: "the same day for another security", txs: []store.InvestmentTransaction{acbBuyOfTen(t), vtiBuy},
			adjustments: []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 0, 10), acbAdjustment(t, "sec-2", "2024-06-30", 5, 0)},
		},
		{
			name: "the same security another day", txs: []store.InvestmentTransaction{acbBuyOfTen(t)},
			adjustments: []report.ACBAdjustment{acbAdjustment(t, "sec-1", "2024-06-30", 0, 10), acbAdjustment(t, "sec-1", "2024-07-01", 5, 0)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkAdjusted(t, c.adjustments, c.txs...)

			assert.Equal(t, c.want, got.AdjustmentIssues)
		})
	}
}

func Test_acb_lists_adjustment_issues_by_item_number(t *testing.T) {
	got := acbWalkAdjusted(t,
		[]report.ACBAdjustment{
			acbAdjustment(t, "sec-1", "2024-01-01", 0, 10),
			acbAdjustment(t, "sec-99", "2024-06-30", 5, 0),
			acbAdjustment(t, "sec-1", "2024-06-30", 0, 10),
			acbAdjustment(t, "sec-1", "2024-06-30", 5, 0),
		},
		acbBuyOfTen(t),
	)

	items := make([]int, 0, len(got.AdjustmentIssues))
	for _, issue := range got.AdjustmentIssues {
		items = append(items, issue.Item)
	}
	assert.Equal(t, []int{1, 2, 4}, items)
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
	sell := acbTx(t, 2, "acct-3", "sec-1", "2024-01-10", store.ActionSell, "USD", -10*acbMillion, 12_000)
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
		acbTx(t, 1, "acct-1", "sec-1", "2024-02-01", store.ActionSell, "CAD", -10*acbMillion, 800),
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
		acbTx(t, 6, "acct-1", "sec-1", "2024-04-01", store.ActionSell, "CAD", -2*acbMillion, 2_000),
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
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -4*acbMillion, 1_200),
	)

	assert.Equal(t, "5/2", held.Securities[0].PerShare().RatString())
	assert.Nil(t, soldOut.Securities[0].PerShare())
}

func Test_acb_converts_a_usd_sales_proceeds_and_outlays_together_before_rounding(t *testing.T) {
	commission := int64(10_100)
	sell := acbTx(t, 2, "acct-3", "sec-1", "2024-01-10", store.ActionSell, "USD", -10*acbMillion, 1_001)
	sell.Commission = &commission

	got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "USD")},
		[]store.Rate{acbRate(t, "2024-01-02", 1_500_000)},
		acbTx(t, 1, "acct-3", "sec-1", "2024-01-02", store.ActionBuy, "USD", 10*acbMillion, -1_000),
		sell,
	)

	assert.Equal(t, []acbSaleRow{
		{Date: "2024-01-10", Security: "sec-1", Shares: "10", Proceeds: 1_653, Outlays: 152, ACBRemoved: 1_500, Gain: 1},
	}, acbSaleRows(got))
}

const (
	acbUnclassifiedOne = "acb needs every brokerage and retirement account classified; 1 account is in neither accounts.registered nor accounts.non-registered in " +
		"~/Library/Application Support/quarry/config.toml; quarry findings --type unclassified-account --status all lists it"
	acbUnclassifiedTwo = "acb needs every brokerage and retirement account classified; 2 accounts are in neither accounts.registered nor accounts.non-registered in " +
		"~/Library/Application Support/quarry/config.toml; quarry findings --type unclassified-account --status all lists them"
)

var (
	openBrokerage    = store.Account{ID: "acct-20", Name: "Open", Type: store.AccountTypeBrokerage, Currency: "CAD"}
	closedRetirement = store.Account{ID: "acct-21", Name: "Closed", Type: store.AccountTypeRetirement, Currency: "CAD", Closed: true}
	chequingUnlisted = store.Account{ID: "acct-22", Name: "Chequing", Type: "chequing", Currency: "CAD"}
)

func acbWithAccounts(t *testing.T, extra ...store.Account) (report.ACB, error) {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{history: store.InvestmentHistory{
		Accounts:   append(acbAccounts(), extra...),
		Securities: []store.Security{acbSecurity("sec-1", "XEQT", "CAD")},
	}}))

	return srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday})
}

func Test_acb_refuses_while_an_investment_account_is_unclassified(t *testing.T) {
	cases := []struct {
		name    string
		extra   []store.Account
		message string
		count   int
	}{
		{name: "one open brokerage account", extra: []store.Account{openBrokerage}, message: acbUnclassifiedOne, count: 1},
		{name: "one closed retirement account counts", extra: []store.Account{closedRetirement}, message: acbUnclassifiedOne, count: 1},
		{name: "two accounts take the plural wording", extra: []store.Account{openBrokerage, closedRetirement}, message: acbUnclassifiedTwo, count: 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := acbWithAccounts(t, c.extra...)

			refusal, ok := errors.AsType[report.RefusalError](err)
			require.True(t, ok)
			assert.Equal(t, c.message, refusal.Error())
			assert.Equal(t, report.RefusalUnclassifiedAccounts, refusal.Kind)
			assert.Equal(t, c.count, refusal.Count)
		})
	}
}

func Test_acb_does_not_count_an_account_that_needs_no_classification(t *testing.T) {
	cases := []struct {
		name  string
		extra []store.Account
	}{
		{name: "every investment account is listed", extra: nil},
		{name: "a chequing account in neither list", extra: []store.Account{chequingUnlisted}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := acbWithAccounts(t, c.extra...)

			assert.NoError(t, err)
		})
	}
}

func Test_acb_refuses_an_unclassified_account_before_an_unknown_security(t *testing.T) {
	history := selectHistory(t)
	history.Accounts = append(history.Accounts, openBrokerage)

	_, err := selectACB(t, history, "sec-9")

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, acbUnclassifiedOne, refusal.Error())
}

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
	assert.False(t, event.Unvalued)
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

func Test_acb_leaves_a_usd_event_with_no_rate_on_file_unvalued(t *testing.T) {
	got := acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "USD")}, nil,
		acbTx(t, 1, "acct-3", "sec-1", "2024-01-02", store.ActionBuy, "USD", 10*acbMillion, -10_000),
	)

	event := got.Securities[0].Events[0]
	assert.Equal(t, int64(-10_000), *event.Amount)
	assert.Zero(t, event.Rate)
	assert.True(t, event.Unvalued)
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

// acbTickered is a CAD security with the given ticker, nil when ticker is nil.
func acbTickered(id, name string, ticker *string) store.Security {
	currency := "CAD"
	return store.Security{ID: id, Name: name, Ticker: ticker, Currency: &currency}
}

// acbBoughtIn is the walk of one CAD buy of each security, security i in accounts[i].
func acbBoughtIn(t *testing.T, securities []store.Security, accounts ...string) report.ACB {
	t.Helper()
	txs := make([]store.InvestmentTransaction, len(securities))
	for i, security := range securities {
		txs[i] = acbTx(t, int64(i+1), accounts[i], security.ID, "2024-01-05", store.ActionBuy, "CAD", acbMillion, -100)
	}

	return acbWalkRequest(t, securities, nil, nil, txs...)
}

// acbSharedNames is the names of each shared group, under its ticker.
func acbSharedNames(a report.ACB) map[string][]string {
	names := make(map[string][]string)
	for _, group := range a.SharedTickers() {
		for _, security := range group.Securities {
			names[group.Ticker] = append(names[group.Ticker], security.Name)
		}
	}

	return names
}

func Test_acb_shared_tickers_match_exactly_and_ignore_an_empty_ticker(t *testing.T) {
	cases := []struct {
		name       string
		securities []store.Security
		want       map[string][]string
	}{
		{
			name:       "tickers differing only by case are not shared",
			securities: []store.Security{acbTickered("sec-1", "Upper", new("VTI")), acbTickered("sec-2", "Lower", new("vti"))},
			want:       map[string][]string{},
		},
		{
			name:       "two securities with an empty ticker are not shared",
			securities: []store.Security{acbTickered("sec-1", "One", new("")), acbTickered("sec-2", "Two", new(""))},
			want:       map[string][]string{},
		},
		{
			name:       "two securities with no ticker are not shared",
			securities: []store.Security{acbTickered("sec-1", "One", nil), acbTickered("sec-2", "Two", nil)},
			want:       map[string][]string{},
		},
		{
			name:       "a ticker held by one security is not shared",
			securities: []store.Security{acbTickered("sec-1", "One", new("VTI")), acbTickered("sec-2", "Two", new("XEQT"))},
			want:       map[string][]string{},
		},
		{
			name: "two securities with one ticker are one group of two",
			securities: []store.Security{
				acbTickered("sec-1", "Alpha", new("VTI")), acbTickered("sec-2", "Beta", new("VTI")),
			},
			want: map[string][]string{"VTI": {"Alpha", "Beta"}},
		},
		{
			name: "three securities with one ticker are one group of three in security order",
			securities: []store.Security{
				acbTickered("sec-1", "Alpha", new("VTI")), acbTickered("sec-2", "Beta", new("VTI")), acbTickered("sec-3", "Gamma", new("VTI")),
			},
			want: map[string][]string{"VTI": {"Alpha", "Beta", "Gamma"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbBoughtIn(t, c.securities, "acct-1", "acct-1", "acct-1")

			assert.Equal(t, c.want, acbSharedNames(got))
		})
	}
}

func Test_acb_shared_tickers_order_groups_by_their_first_member(t *testing.T) {
	securities := []store.Security{
		acbTickered("sec-1", "Alpha", new("XEQT")), acbTickered("sec-2", "Beta", new("VTI")),
		acbTickered("sec-3", "Gamma", new("VTI")), acbTickered("sec-4", "Delta", new("XEQT")),
	}
	got := acbBoughtIn(t, securities, "acct-1", "acct-1", "acct-1", "acct-1")

	groups := got.SharedTickers()

	assert.Equal(t, []string{"XEQT", "VTI"}, []string{groups[0].Ticker, groups[1].Ticker})
	assert.Equal(t, []string{"Alpha", "Delta"}, []string{groups[0].Securities[0].Name, groups[0].Securities[1].Name})
}

func Test_acb_shared_tickers_leave_out_a_security_held_only_in_a_registered_account(t *testing.T) {
	securities := []store.Security{acbTickered("sec-1", "Alpha", new("VTI")), acbTickered("sec-2", "Beta", new("VTI"))}

	got := acbBoughtIn(t, securities, "acct-1", "acct-9")

	assert.Empty(t, got.SharedTickers())
}

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
