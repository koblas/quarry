package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acbMillion = 1_000_000

var acbToday = time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)

// acbClassification lists acct-1, acct-2 and acct-3 non-registered and acct-9 registered.
func acbClassification() report.Classification {
	return report.Classification{
		Registered:    []string{"acct-9"},
		NonRegistered: []string{"acct-1", "acct-2", "acct-3"},
	}
}

func acbAccounts() []store.Account {
	return []store.Account{
		{ID: "acct-1", Name: "Margin", Type: store.AccountTypeBrokerage, Currency: "CAD"},
		{ID: "acct-2", Name: "Old margin", Type: store.AccountTypeBrokerage, Currency: "CAD", Closed: true},
		{ID: "acct-3", Name: "US margin", Type: store.AccountTypeBrokerage, Currency: "USD"},
		{ID: "acct-9", Name: "RRSP", Type: store.AccountTypeRetirement, Currency: "CAD"},
	}
}

func acbSecurity(id, name, currency string) store.Security {
	return store.Security{ID: id, Name: name, Ticker: &name, Currency: &currency}
}

// acbTx is one investment transaction with shares in millionths and amount in cents, no commission.
func acbTx(t *testing.T, sourceID int64, account, security, date, action, currency string, shares, amount int64) store.InvestmentTransaction {
	t.Helper()
	return store.InvestmentTransaction{
		ID:         "itxn-" + date + "-" + account + "-" + security + "-" + action,
		SourceID:   sourceID,
		AccountID:  account,
		SecurityID: &security,
		Date:       dateOf(t, date),
		Action:     action,
		Shares:     &shares,
		Amount:     amount,
		Currency:   currency,
	}
}

func acbRate(t *testing.T, date string, usdcad money.Rate) store.Rate {
	t.Helper()
	return store.Rate{Date: dateOf(t, date), USDCAD: usdcad, Series: store.SeriesCurrent}
}

type acbSaleRow struct {
	Date                                string
	Security                            string
	Shares                              string
	Proceeds, Outlays, ACBRemoved, Gain int64
}

type acbYearRow struct {
	Year                                int
	Sales                               []acbSaleRow
	Proceeds, Outlays, ACBRemoved, Gain int64
	ReturnOfCapitalGain                 int64
}

type acbPositionRow struct {
	Name   string
	Shares string
	ACB    int64
}

func acbYearRows(result report.ACB) []acbYearRow {
	rows := make([]acbYearRow, 0, len(result.Years))
	for _, year := range result.Years {
		row := acbYearRow{
			Year: year.Year, Proceeds: year.Proceeds, Outlays: year.Outlays, ACBRemoved: year.ACBRemoved, Gain: year.Gain,
			ReturnOfCapitalGain: year.ReturnOfCapitalGain,
		}
		for _, sale := range year.Sales {
			row.Sales = append(row.Sales, acbSaleRow{
				Date:       sale.Date.Format(time.DateOnly),
				Security:   sale.SecurityID,
				Shares:     sale.Shares.RatString(),
				Proceeds:   sale.Proceeds,
				Outlays:    sale.Outlays,
				ACBRemoved: sale.ACBRemoved,
				Gain:       sale.Gain,
			})
		}
		rows = append(rows, row)
	}
	return rows
}

func acbPositionRows(result report.ACB) []acbPositionRow {
	rows := make([]acbPositionRow, 0, len(result.Securities))
	for _, position := range result.Securities {
		rows = append(rows, acbPositionRow{Name: position.Security.Name, Shares: position.Shares.RatString(), ACB: position.ACB})
	}
	return rows
}

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
