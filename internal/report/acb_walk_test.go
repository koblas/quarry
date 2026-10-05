package report_test

import (
	"cmp"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbWalkOf walks txs over acct-1, acct-2 (closed) and acct-3 non-registered, acct-9 registered and acct-7,
// a chequing account, in neither list, with securities sec-1 XEQT and sec-2 VTI.
func acbWalkOf(t *testing.T, txs ...store.InvestmentTransaction) report.ACB {
	t.Helper()
	return acbWalkWith(t, []store.Security{acbSecurity("sec-1", "XEQT", "CAD"), acbSecurity("sec-2", "VTI", "CAD")}, nil, txs...)
}

// acbWalkWith is acbWalkOf over the given securities and exchange rates, which are in date order.
func acbWalkWith(t *testing.T, securities []store.Security, rates []store.Rate, txs ...store.InvestmentTransaction) report.ACB {
	t.Helper()
	return acbWalkRequest(t, securities, rates, nil, txs...)
}

// acbWalkAdjusted is acbWalkOf with the adjustments, whose item numbers are their places in the list.
func acbWalkAdjusted(t *testing.T, adjustments []report.ACBAdjustment, txs ...store.InvestmentTransaction) report.ACB {
	t.Helper()
	return acbWalkRequest(t, []store.Security{acbSecurity("sec-1", "XEQT", "CAD"), acbSecurity("sec-2", "VTI", "CAD")}, nil, adjustments, txs...)
}

func acbWalkRequest(t *testing.T, securities []store.Security, rates []store.Rate, adjustments []report.ACBAdjustment, txs ...store.InvestmentTransaction) report.ACB {
	t.Helper()
	txs = slices.SortedStableFunc(slices.Values(txs), func(a, b store.InvestmentTransaction) int {
		return cmp.Or(a.Date.Compare(b.Date), cmp.Compare(a.SourceID, b.SourceID))
	})
	unlisted := store.Account{ID: "acct-7", Name: "Cash margin", Type: "chequing", Currency: "CAD"}
	srv := report.NewServer(report.WithStore(fakeStore{history: store.InvestmentHistory{
		Accounts:     append(acbAccounts(), unlisted),
		Securities:   securities,
		Transactions: txs,
		Rates:        rates,
	}}))

	got, err := srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday, Adjustments: adjustments})

	require.NoError(t, err)
	return got
}

func acbSaleRows(result report.ACB) []acbSaleRow {
	var rows []acbSaleRow
	for _, year := range acbYearRows(result) {
		rows = append(rows, year.Sales...)
	}
	return rows
}

func Test_acb_rounds_the_acb_removed_half_away_from_zero(t *testing.T) {
	cases := []struct {
		name         string
		boughtCents  int64
		boughtShares int64
		soldShares   int64
		wantRemoved  int64
	}{
		{name: "a half cent rounds up", boughtCents: 5, boughtShares: 2, soldShares: 1, wantRemoved: 3},
		{name: "under a half cent rounds down", boughtCents: 4, boughtShares: 3, soldShares: 1, wantRemoved: 1},
		{name: "over a half cent rounds up", boughtCents: 5, boughtShares: 3, soldShares: 1, wantRemoved: 2},
		{name: "a negative half cent rounds away from zero", boughtCents: -5, boughtShares: 2, soldShares: 1, wantRemoved: -3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t,
				acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", c.boughtShares*acbMillion, -c.boughtCents),
				acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -c.soldShares*acbMillion, 100),
			)

			sales := acbSaleRows(got)
			require.Len(t, sales, 1)
			assert.Equal(t, c.wantRemoved, sales[0].ACBRemoved)
			assert.Equal(t, c.boughtCents-c.wantRemoved, got.Securities[0].ACB)
		})
	}
}

func Test_acb_removes_the_whole_acb_when_a_sale_exceeds_the_pool(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -35_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -12*acbMillion, 50_000),
	)

	assert.Equal(t, []acbSaleRow{
		{Date: "2024-02-02", Security: "sec-1", Shares: "12", Proceeds: 50_000, ACBRemoved: 35_000, Gain: 15_000},
	}, acbSaleRows(got))
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 0}}, acbPositionRows(got))
}

func Test_acb_restarts_from_zero_after_selling_out(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -10_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -12*acbMillion, 12_000),
		acbTx(t, 3, "acct-1", "sec-1", "2024-03-02", store.ActionBuy, "CAD", 10*acbMillion, -35_000),
		acbTx(t, 4, "acct-1", "sec-1", "2024-04-02", store.ActionSell, "CAD", -5*acbMillion, 20_000),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "5", ACB: 17_500}}, acbPositionRows(got))
	assert.Equal(t, int64(17_500), acbSaleRows(got)[1].ACBRemoved)
}

func Test_acb_pools_only_non_registered_accounts(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-9", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -5_000),
		acbTx(t, 3, "acct-7", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -7_000),
		acbTx(t, 4, "acct-2", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -2_000),
		acbTx(t, 5, "acct-9", "sec-2", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -9_000),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "20", ACB: 3_000}}, acbPositionRows(got))
}

func Test_acb_keeps_fractional_shares_exact(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", acbMillion/2, -3_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-01-03", store.ActionBuy, "CAD", acbMillion/4, -1_500),
		acbTx(t, 3, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -333_333, 2_500),
	)

	assert.Equal(t, int64(2_000), acbSaleRows(got)[0].ACBRemoved)
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "416667/1000000", ACB: 2_500}}, acbPositionRows(got))
}

func Test_acb_orders_a_days_buys_before_its_sales(t *testing.T) {
	cases := []struct {
		name               string
		buySource, sellSrc int64
	}{
		{name: "the sale has the lower source id", buySource: 9, sellSrc: 1},
		{name: "the sale has the higher source id", buySource: 1, sellSrc: 9},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t,
				acbTx(t, c.sellSrc, "acct-1", "sec-1", "2024-01-02", store.ActionSell, "CAD", -10*acbMillion, 1_500),
				acbTx(t, c.buySource, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
			)

			assert.Equal(t, int64(1_000), acbSaleRows(got)[0].ACBRemoved)
			assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 0}}, acbPositionRows(got))
		})
	}
}

func Test_acb_leaves_out_events_after_today(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", acbToday.Format(time.DateOnly), store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", acbToday.AddDate(0, 0, 1).Format(time.DateOnly), store.ActionBuy, "CAD", 5*acbMillion, -900),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 1_000}}, acbPositionRows(got))
}

func Test_acb_counts_a_sale_in_the_calendar_year_of_its_date(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-12-31", store.ActionSell, "CAD", -4*acbMillion, 800),
		acbTx(t, 3, "acct-1", "sec-1", "2025-01-01", store.ActionSell, "CAD", -4*acbMillion, 900),
	)

	assert.Equal(t, []acbYearRow{
		{
			Year: 2024, Sales: []acbSaleRow{{Date: "2024-12-31", Security: "sec-1", Shares: "4", Proceeds: 800, ACBRemoved: 400, Gain: 400}},
			Proceeds: 800, ACBRemoved: 400, Gain: 400,
		},
		{
			Year: 2025, Sales: []acbSaleRow{{Date: "2025-01-01", Security: "sec-1", Shares: "4", Proceeds: 900, ACBRemoved: 400, Gain: 500}},
			Proceeds: 900, ACBRemoved: 400, Gain: 500,
		},
	}, acbYearRows(got))
}

func Test_acb_reads_the_store_once(t *testing.T) {
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{historyReads: &reads, history: store.InvestmentHistory{
		Accounts:   acbAccounts(),
		Securities: []store.Security{acbSecurity("sec-1", "XEQT", "CAD")},
	}}))

	_, err := srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
}

func Test_acb_refuses_a_store_it_cannot_read(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday})

	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_acb_reports_an_interrupt_during_the_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.ACB(ctx, report.ACBRequest{Classification: acbClassification(), Today: acbToday})

	assert.EqualError(t, err, "acb interrupted")
}

func Test_acb_returns_any_other_read_failure_unchanged(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday})

	assert.Equal(t, errDiskRead, err)
}

func Test_acb_counts_a_sale_without_a_commission_as_having_no_outlays(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -10*acbMillion, 1_600),
	)

	assert.Equal(t, []acbSaleRow{
		{Date: "2024-02-02", Security: "sec-1", Shares: "10", Proceeds: 1_600, Outlays: 0, ACBRemoved: 1_000, Gain: 600},
	}, acbSaleRows(got))
}

func Test_acb_adds_a_sales_commission_to_its_proceeds_and_rounds_it_half_away(t *testing.T) {
	cases := []struct {
		name        string
		commission  int64
		wantOutlays int64
	}{
		{name: "a half cent rounds up", commission: 49_450, wantOutlays: 495},
		{name: "under a half cent rounds down", commission: 49_440, wantOutlays: 494},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sell := acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -10*acbMillion, 80_000)
			sell.Commission = &c.commission

			got := acbWalkOf(t,
				acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -50_000),
				sell,
			)

			assert.Equal(t, []acbSaleRow{{
				Date: "2024-02-02", Security: "sec-1", Shares: "10",
				Proceeds: 80_000 + c.wantOutlays, Outlays: c.wantOutlays, ACBRemoved: 50_000, Gain: 30_000,
			}}, acbSaleRows(got))
		})
	}
}

func Test_acb_leaves_the_pool_alone_for_actions_it_does_not_walk(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-01-03", store.ActionDividend, "CAD", 0, 50),
		acbTx(t, 3, "acct-1", "sec-1", "2024-01-04", store.ActionCapitalGainLong, "CAD", 0, 60),
		acbTx(t, 4, "acct-1", "sec-1", "2024-01-05", store.ActionMiscIncome, "CAD", 0, 70),
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 1_000}}, acbPositionRows(got))
	assert.Empty(t, acbSaleRows(got))
	assert.Len(t, got.Securities[0].Events, 1)
}

func Test_acb_reads_a_missing_share_count_as_none(t *testing.T) {
	missingBuy := acbTx(t, 3, "acct-1", "sec-1", "2024-03-02", store.ActionBuy, "CAD", 0, -200)
	missingBuy.Shares = nil
	missingSell := acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", 0, 300)
	missingSell.Shares = nil

	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		missingSell,
		missingBuy,
	)

	assert.Equal(t, []acbSaleRow{{Date: "2024-02-02", Security: "sec-1", Shares: "0", Proceeds: 300, Gain: 300}}, acbSaleRows(got))
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "10", ACB: 1_200}}, acbPositionRows(got))
}

func Test_acb_reads_a_sales_negative_stored_shares_as_the_units_sold(t *testing.T) {
	got := acbWalkOf(t,
		acbTx(t, 1, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 2, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -4*acbMillion, 800),
	)

	assert.Equal(t, []acbSaleRow{
		{Date: "2024-02-02", Security: "sec-1", Shares: "4", Proceeds: 800, ACBRemoved: 400, Gain: 400},
	}, acbSaleRows(got))
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "6", ACB: 600}}, acbPositionRows(got))
	assert.Equal(t, "4", got.Securities[0].Events[1].Shares.RatString())
}

func Test_acb_gains_the_proceeds_less_outlays_of_a_sale_into_an_empty_pool(t *testing.T) {
	commission := int64(10_000)
	sell := acbTx(t, 1, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -5*acbMillion, 800)
	sell.Commission = &commission

	got := acbWalkOf(t, sell)

	assert.Equal(t, []acbSaleRow{
		{Date: "2024-02-02", Security: "sec-1", Shares: "5", Proceeds: 900, Outlays: 100, Gain: 800},
	}, acbSaleRows(got))
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "0", ACB: 0}}, acbPositionRows(got))
}

func Test_acb_orders_two_securities_sales_on_one_date_by_source_id(t *testing.T) {
	cases := []struct {
		name              string
		xeqtSell, vtiSell int64
		wantSecurityOrder []string
	}{
		{name: "the alphabetically later security sells at the lower source id", xeqtSell: 1, vtiSell: 2, wantSecurityOrder: []string{"sec-1", "sec-2"}},
		{name: "the alphabetically earlier security sells at the lower source id", xeqtSell: 2, vtiSell: 1, wantSecurityOrder: []string{"sec-2", "sec-1"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := acbWalkOf(t,
				acbTx(t, 11, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
				acbTx(t, 12, "acct-1", "sec-2", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
				acbTx(t, c.xeqtSell, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -4*acbMillion, 800),
				acbTx(t, c.vtiSell, "acct-1", "sec-2", "2024-02-02", store.ActionSell, "CAD", -4*acbMillion, 800),
			)

			var order []string
			for _, sale := range got.Years[0].Sales {
				order = append(order, sale.SecurityID)
			}
			assert.Equal(t, c.wantSecurityOrder, order)
		})
	}
}

func Test_acb_adds_a_days_reinvestment_before_its_sale(t *testing.T) {
	cost := int64(500)
	reinvest := acbTx(t, 9, "acct-1", "sec-1", "2024-02-02", store.ActionReinvestDividend, "CAD", 5*acbMillion, 0)
	reinvest.CostBasis = &cost

	got := acbWalkOf(t,
		acbTx(t, 5, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbTx(t, 1, "acct-1", "sec-1", "2024-02-02", store.ActionSell, "CAD", -12*acbMillion, 3_000),
		reinvest,
	)

	assert.Equal(t, []acbSaleRow{
		{Date: "2024-02-02", Security: "sec-1", Shares: "12", Proceeds: 3_000, ACBRemoved: 1_200, Gain: 1_800},
	}, acbSaleRows(got))
	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "3", ACB: 300}}, acbPositionRows(got))
}

func Test_acb_splits_a_days_reinvested_units(t *testing.T) {
	cost := int64(500)
	reinvest := acbTx(t, 9, "acct-1", "sec-1", "2024-02-02", store.ActionReinvestDividend, "CAD", 5*acbMillion, 0)
	reinvest.CostBasis = &cost

	got := acbWalkOf(t,
		acbTx(t, 5, "acct-1", "sec-1", "2024-01-02", store.ActionBuy, "CAD", 10*acbMillion, -1_000),
		acbSplitTx(t, 1, "acct-1", "2024-02-02", 2*acbMillion, acbMillion),
		reinvest,
	)

	assert.Equal(t, []acbPositionRow{{Name: "XEQT", Shares: "30", ACB: 1_500}}, acbPositionRows(got))
}
