package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
