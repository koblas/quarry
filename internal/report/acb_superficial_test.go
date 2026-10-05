package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// acbLossDay is the day of the loss sale acbLossRows makes.
const acbLossDay = "2024-06-03"

// acbShift is date moved by days.
func acbShift(t *testing.T, date string, days int) string {
	t.Helper()
	return dateOf(t, date).AddDate(0, 0, days).Format(time.DateOnly)
}

// acbLossRows is 10 shares of sec-1 bought in acct-1 for 1,000.00 and all sold there on saleDay for proceeds cents.
func acbLossRows(t *testing.T, saleDay string, proceeds int64) []store.InvestmentTransaction {
	t.Helper()
	return []store.InvestmentTransaction{
		acbTx(t, 1, "acct-1", "sec-1", "2023-12-01", store.ActionBuy, "CAD", 10*acbMillion, -100_000),
		acbTx(t, 2, "acct-1", "sec-1", saleDay, store.ActionSell, "CAD", -10*acbMillion, proceeds),
	}
}

// acbKept is one share of sec-1 held in acct-9 since long before any window, so a sale is never what empties it.
func acbKept(t *testing.T) store.InvestmentTransaction {
	t.Helper()
	return acbTx(t, 90, "acct-9", "sec-1", "2023-01-01", store.ActionBuy, "CAD", acbMillion, -10_000)
}

// acbFlags is each sale's possible-superficial-loss mark, in the order the years list the sales.
func acbFlags(result report.ACB) []bool {
	var flags []bool
	for _, year := range result.Years {
		for _, sale := range year.Sales {
			flags = append(flags, sale.PossibleSuperficialLoss)
		}
	}
	return flags
}

func acbSecurityTicker(id string, ticker *string) store.Security {
	return store.Security{ID: id, Name: "Security " + id, Ticker: ticker}
}

func Test_acb_marks_a_loss_sale_with_an_acquisition_in_the_window(t *testing.T) {
	cases := []struct {
		name    string
		account string
		action  string
		offset  int
	}{
		{name: "a re-buy in a registered account", account: "acct-9", action: store.ActionBuy, offset: 10},
		{name: "a re-buy in the account that sold", account: "acct-1", action: store.ActionBuy, offset: 10},
		{name: "a re-buy in another non-registered account", account: "acct-3", action: store.ActionBuy, offset: 10},
		{name: "a re-buy before the sale", account: "acct-9", action: store.ActionBuy, offset: -10},
		{name: "a re-buy the same day", account: "acct-9", action: store.ActionBuy, offset: 0},
		{name: "a reinvested dividend", account: "acct-9", action: store.ActionReinvestDividend, offset: 3},
		{name: "added shares", account: "acct-9", action: store.ActionAddShares, offset: 5},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			acquisition := acbTx(t, 3, c.account, "sec-1", acbShift(t, acbLossDay, c.offset), c.action, "CAD", 5*acbMillion, -30_000)

			got := acbWalkOf(t, append(acbLossRows(t, acbLossDay, 50_000), acquisition)...)

			assert.Equal(t, []bool{true}, acbFlags(got))
		})
	}
}

func Test_acb_marks_a_loss_sale_with_an_acquisition_30_days_before(t *testing.T) {
	rebuy := acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, -30), store.ActionBuy, "CAD", 5*acbMillion, -30_000)

	got := acbWalkOf(t, append(acbLossRows(t, acbLossDay, 50_000), rebuy)...)

	assert.Equal(t, []bool{true}, acbFlags(got))
}

func Test_acb_marks_a_loss_sale_with_an_acquisition_30_days_after(t *testing.T) {
	rebuy := acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 30), store.ActionBuy, "CAD", 5*acbMillion, -30_000)

	got := acbWalkOf(t, append(acbLossRows(t, acbLossDay, 50_000), rebuy)...)

	assert.Equal(t, []bool{true}, acbFlags(got))
}

func Test_acb_does_not_mark_a_loss_sale_with_an_acquisition_31_days_away(t *testing.T) {
	cases := []struct {
		name   string
		offset int
	}{
		{name: "31 days before", offset: -31},
		{name: "31 days after", offset: 31},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rebuy := acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, c.offset), store.ActionBuy, "CAD", 5*acbMillion, -30_000)

			got := acbWalkOf(t, append(acbLossRows(t, acbLossDay, 50_000), acbKept(t), rebuy)...)

			assert.Equal(t, []bool{false}, acbFlags(got))
		})
	}
}

func Test_acb_does_not_mark_a_loss_sale_when_nothing_else_acquires_the_security(t *testing.T) {
	cases := []struct {
		name string
		row  store.InvestmentTransaction
	}{
		{name: "a sale in another account", row: acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 5), store.ActionSell, "CAD", -acbMillion/2, 5_000)},
		{name: "shares removed from another account", row: acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 5), store.ActionRemoveShares, "CAD", -acbMillion/2, 0)},
		{name: "added shares of no units", row: acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 5), store.ActionAddShares, "CAD", 0, 0)},
		{name: "a re-buy of another security", row: acbTx(t, 3, "acct-9", "sec-2", acbShift(t, acbLossDay, 5), store.ActionBuy, "CAD", 5*acbMillion, -30_000)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t, append(acbLossRows(t, acbLossDay, 50_000), acbKept(t), c.row)...)

			assert.Equal(t, []bool{false}, acbFlags(got))
		})
	}
}

func Test_acb_does_not_count_a_reinvested_distribution_adjustment_as_an_acquisition(t *testing.T) {
	adjustment := []report.ACBAdjustment{{SecurityID: "sec-1", Date: dateOf(t, acbShift(t, acbLossDay, 5)), ReinvestedDistribution: 1_000}}
	rows := []store.InvestmentTransaction{
		acbTx(t, 1, "acct-1", "sec-1", "2023-12-01", store.ActionBuy, "CAD", 10*acbMillion, -100_000),
		acbTx(t, 2, "acct-1", "sec-1", acbLossDay, store.ActionSell, "CAD", -5*acbMillion, 20_000),
		acbKept(t),
	}

	got := acbWalkAdjusted(t, adjustment, rows...)

	assert.Equal(t, []bool{false}, acbFlags(got))
	assert.Equal(t, int64(51_000), got.Securities[0].ACB)
}

func Test_acb_does_not_mark_a_sale_that_is_not_at_a_loss(t *testing.T) {
	cases := []struct {
		name     string
		proceeds int64
	}{
		{name: "a gain", proceeds: 150_000},
		{name: "break-even", proceeds: 100_000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rebuy := acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 5), store.ActionBuy, "CAD", 5*acbMillion, -30_000)

			got := acbWalkOf(t, append(acbLossRows(t, acbLossDay, c.proceeds), rebuy)...)

			assert.Equal(t, []bool{false}, acbFlags(got))
		})
	}
}

func Test_acb_marks_a_loss_when_a_security_of_the_same_ticker_is_acquired(t *testing.T) {
	securities := []store.Security{acbSecurityTicker("sec-1", new("AAA")), acbSecurityTicker("sec-3", new("AAA"))}
	rebuy := acbTx(t, 3, "acct-9", "sec-3", acbShift(t, acbLossDay, 10), store.ActionBuy, "CAD", 5*acbMillion, -30_000)

	got := acbWalkWith(t, securities, nil, append(acbLossRows(t, acbLossDay, 50_000), rebuy)...)

	assert.Equal(t, []bool{true}, acbFlags(got))
}

func Test_acb_does_not_mark_a_loss_when_the_other_security_is_not_the_same_ticker(t *testing.T) {
	cases := []struct {
		name  string
		other *string
		this  *string
	}{
		{name: "a different ticker", this: new("AAA"), other: new("BBB")},
		{name: "a ticker differing in case", this: new("AAA"), other: new("aaa")},
		{name: "no ticker on either", this: nil, other: nil},
		{name: "an empty ticker on both", this: new(""), other: new("")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			securities := []store.Security{acbSecurityTicker("sec-1", c.this), acbSecurityTicker("sec-3", c.other)}
			rebuy := acbTx(t, 3, "acct-9", "sec-3", acbShift(t, acbLossDay, 10), store.ActionBuy, "CAD", 5*acbMillion, -30_000)

			got := acbWalkWith(t, securities, nil, append(acbLossRows(t, acbLossDay, 50_000), acbKept(t), rebuy)...)

			assert.Equal(t, []bool{false}, acbFlags(got))
		})
	}
}

func Test_acb_ignores_an_acquisition_dated_after_today(t *testing.T) {
	cases := []struct {
		name string
		date string
		want bool
	}{
		{name: "a re-buy today", date: acbToday.Format(time.DateOnly), want: true},
		{name: "a re-buy tomorrow", date: acbToday.AddDate(0, 0, 1).Format(time.DateOnly), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			saleDay := acbShift(t, acbToday.Format(time.DateOnly), -10)
			rebuy := acbTx(t, 3, "acct-9", "sec-1", c.date, store.ActionBuy, "CAD", 5*acbMillion, -30_000)

			got := acbWalkOf(t, append(acbLossRows(t, saleDay, 50_000), acbKept(t), rebuy)...)

			assert.Equal(t, []bool{c.want}, acbFlags(got))
		})
	}
}

func Test_acb_marks_a_loss_sale_of_shares_bought_within_30_days_before_it(t *testing.T) {
	rows := []store.InvestmentTransaction{
		acbKept(t),
		acbTx(t, 1, "acct-1", "sec-1", acbShift(t, acbLossDay, -5), store.ActionBuy, "CAD", 10*acbMillion, -100_000),
		acbTx(t, 2, "acct-1", "sec-1", acbLossDay, store.ActionSell, "CAD", -10*acbMillion, 50_000),
	}

	got := acbWalkOf(t, rows...)

	assert.Equal(t, []bool{true}, acbFlags(got))
}

func Test_acb_does_not_mark_a_loss_bought_and_sold_the_same_day_when_nothing_is_held_after(t *testing.T) {
	rows := []store.InvestmentTransaction{
		acbTx(t, 1, "acct-1", "sec-1", acbLossDay, store.ActionBuy, "CAD", 10*acbMillion, -100_000),
		acbTx(t, 2, "acct-1", "sec-1", acbLossDay, store.ActionSell, "CAD", -10*acbMillion, 50_000),
	}

	got := acbWalkOf(t, rows...)

	assert.Equal(t, []bool{false}, acbFlags(got))
}

// acbSplit is a split of sec-1 recorded in account, newShares for every oldShares held.
func acbSplit(t *testing.T, sourceID int64, account, date string, newShares, oldShares int64) store.InvestmentTransaction {
	t.Helper()
	tx := acbTx(t, sourceID, account, "sec-1", date, store.ActionSplit, "CAD", 0, 0)
	tx.Shares = nil
	tx.SplitNewShares, tx.SplitOldShares = &newShares, &oldShares
	return tx
}

func Test_acb_does_not_mark_a_loss_sale_when_nothing_is_held_30_days_after(t *testing.T) {
	rows := append(acbLossRows(t, acbLossDay, 50_000),
		acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 10), store.ActionBuy, "CAD", 5*acbMillion, -30_000),
		acbTx(t, 4, "acct-9", "sec-1", acbShift(t, acbLossDay, 20), store.ActionSell, "CAD", -5*acbMillion, 40_000),
	)

	got := acbWalkOf(t, rows...)

	assert.Equal(t, []bool{false}, acbFlags(got))
}

func Test_acb_counts_the_acquired_shares_as_held_until_the_end_of_day_30(t *testing.T) {
	cases := []struct {
		name    string
		soldDay int
		want    bool
	}{
		{name: "sold on day 30", soldDay: 30, want: false},
		{name: "sold on day 31", soldDay: 31, want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows := append(acbLossRows(t, acbLossDay, 50_000),
				acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 10), store.ActionBuy, "CAD", 5*acbMillion, -30_000),
				acbTx(t, 4, "acct-9", "sec-1", acbShift(t, acbLossDay, c.soldDay), store.ActionSell, "CAD", -5*acbMillion, 40_000),
			)

			got := acbWalkOf(t, rows...)

			assert.Equal(t, []bool{c.want}, acbFlags(got))
		})
	}
}

func Test_acb_counts_a_security_of_the_same_ticker_as_held(t *testing.T) {
	cases := []struct {
		name  string
		other *string
		want  bool
	}{
		{name: "the same ticker is held", other: new("AAA"), want: true},
		{name: "another ticker is held", other: new("BBB"), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			securities := []store.Security{acbSecurityTicker("sec-1", new("AAA")), acbSecurityTicker("sec-3", c.other)}
			rows := append(acbLossRows(t, acbLossDay, 50_000),
				acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 5), store.ActionBuy, "CAD", 3*acbMillion, -20_000),
				acbTx(t, 4, "acct-9", "sec-1", acbShift(t, acbLossDay, 10), store.ActionSell, "CAD", -3*acbMillion, 20_000),
				acbTx(t, 5, "acct-9", "sec-3", "2023-01-01", store.ActionBuy, "CAD", acbMillion, -10_000),
			)

			got := acbWalkWith(t, securities, nil, rows...)

			assert.Equal(t, []bool{c.want}, acbFlags(got))
		})
	}
}

func Test_acb_counts_a_holding_after_the_splits_of_its_own_account(t *testing.T) {
	cases := []struct {
		name   string
		splits []store.InvestmentTransaction
		want   bool
	}{
		{name: "no split", want: true},
		{name: "a split that takes the holding below a millionth", splits: []store.InvestmentTransaction{acbSplit(t, 4, "acct-9", acbShift(t, acbLossDay, 10), 1, 3)}, want: false},
		{name: "a split that leaves a millionth", splits: []store.InvestmentTransaction{acbSplit(t, 4, "acct-9", acbShift(t, acbLossDay, 10), 7, 10)}, want: true},
		{
			name: "the same split recorded in another account too",
			splits: []store.InvestmentTransaction{
				acbSplit(t, 4, "acct-9", acbShift(t, acbLossDay, 10), 7, 10),
				acbSplit(t, 5, "acct-3", acbShift(t, acbLossDay, 10), 7, 10),
			},
			want: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows := append(acbLossRows(t, acbLossDay, 50_000),
				acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 5), store.ActionBuy, "CAD", 1, -1))

			got := acbWalkOf(t, append(rows, c.splits...)...)

			assert.Equal(t, []bool{c.want}, acbFlags(got))
		})
	}
}

func Test_acb_does_not_let_a_negative_holding_cancel_a_positive_one(t *testing.T) {
	rows := append(acbLossRows(t, acbLossDay, 50_000),
		acbTx(t, 3, "acct-9", "sec-1", acbShift(t, acbLossDay, 10), store.ActionBuy, "CAD", 2*acbMillion, -10_000),
		acbTx(t, 4, "acct-7", "sec-1", acbShift(t, acbLossDay, 20), store.ActionSell, "CAD", -5*acbMillion, 20_000),
	)

	got := acbWalkOf(t, rows...)

	assert.Equal(t, []bool{true}, acbFlags(got))
}

func Test_acb_measures_a_window_still_open_at_the_end_of_today(t *testing.T) {
	cases := []struct {
		name    string
		soldDay string
		want    bool
	}{
		{name: "the acquired shares sold today", soldDay: acbToday.Format(time.DateOnly), want: false},
		{name: "the acquired shares sold tomorrow", soldDay: acbToday.AddDate(0, 0, 1).Format(time.DateOnly), want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			saleDay := acbShift(t, acbToday.Format(time.DateOnly), -29)
			rows := append(acbLossRows(t, saleDay, 50_000),
				acbTx(t, 3, "acct-9", "sec-1", acbShift(t, saleDay, 1), store.ActionBuy, "CAD", 5*acbMillion, -30_000),
				acbTx(t, 4, "acct-9", "sec-1", c.soldDay, store.ActionSell, "CAD", -5*acbMillion, 40_000),
			)

			got := acbWalkOf(t, rows...)

			assert.Equal(t, []bool{c.want}, acbFlags(got))
		})
	}
}

func Test_ACBYear_counts_the_sales_marked_as_possible_superficial_losses(t *testing.T) {
	cases := []struct {
		name  string
		sales []report.ACBSale
		want  int
	}{
		{name: "no sales"},
		{name: "none marked", sales: []report.ACBSale{{}, {}}},
		{name: "two of three marked", sales: []report.ACBSale{{PossibleSuperficialLoss: true}, {}, {PossibleSuperficialLoss: true}}, want: 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.ACBYear{Sales: c.sales}.PossibleSuperficialLosses())
		})
	}
}

func Test_acb_never_marks_a_return_of_capital_above_the_acb(t *testing.T) {
	adjustment := []report.ACBAdjustment{{SecurityID: "sec-1", Date: dateOf(t, acbLossDay), ReturnOfCapital: 150_000}}
	rows := []store.InvestmentTransaction{
		acbTx(t, 1, "acct-1", "sec-1", "2023-12-01", store.ActionBuy, "CAD", 10*acbMillion, -100_000),
		acbTx(t, 2, "acct-9", "sec-1", acbShift(t, acbLossDay, 5), store.ActionBuy, "CAD", 5*acbMillion, -30_000),
	}

	got := acbWalkAdjusted(t, adjustment, rows...)

	assert.Equal(t, int64(50_000), got.Years[0].ReturnOfCapitalGain)
	assert.Zero(t, got.Years[0].PossibleSuperficialLosses())
}
