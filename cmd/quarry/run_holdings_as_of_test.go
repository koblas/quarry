package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holdingsAsOfLine is one table line of seedSplitHoldingsStore's single holding, each cell as wide as its widest.
func holdingsAsOfLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return fmt.Sprintf("%-9s  %-16s  %6s  %5s  %-10s  %-8s  %8s  %8s\n",
		account, security, shares, price, pricedOn, currency, value, in)
}

// seedSplitHoldingsStore builds the store under a temp HOME with one CAD holding whose shares change by
// trade and by a 2:1 split: 100 bought 2025-06-02, doubled 2025-09-15, 50 more bought 2026-02-10. Its
// prices are 10.00 on 2025-06-02, 12.00 on 2025-12-30 and 15.00 on 2026-01-05.
func seedSplitHoldingsStore(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	day := func(year int, month time.Month, d int) time.Time {
		return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
	}
	security := "sec-acme"
	txn := func(id string, sourceID int64, date time.Time, action string) store.InvestmentTransaction {
		return store.InvestmentTransaction{
			ID: id, SourceID: sourceID, AccountID: "acct-cad", SecurityID: &security, Date: date,
			Action: action, Currency: "CAD",
		}
	}
	firstBuy := txn("inv-buy-1", 1, day(2025, time.June, 2), store.ActionBuy)
	firstBuy.Shares, firstBuy.Amount = new(int64(100_000_000)), -100_000
	split := txn("inv-split", 2, day(2025, time.September, 15), store.ActionSplit)
	split.SplitNewShares, split.SplitOldShares = new(int64(2_000_000)), new(int64(1_000_000))
	secondBuy := txn("inv-buy-2", 3, day(2026, time.February, 10), store.ActionBuy)
	secondBuy.Shares, secondBuy.Amount = new(int64(50_000_000)), -75_000
	rows := spendRows([]store.Account{brokerageAccount("acct-cad", 1, "CAD")})
	rows.Securities = []store.Security{
		{ID: security, SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
	}
	rows.InvestmentTransactions = []store.InvestmentTransaction{firstBuy, split, secondBuy}
	rows.Prices = []store.Price{
		{SecurityID: security, SourceID: 1, Date: day(2025, time.June, 2), Price: 10_000_000},
		{SecurityID: security, SourceID: 2, Date: day(2025, time.December, 30), Price: 12_000_000},
		{SecurityID: security, SourceID: 3, Date: day(2026, time.January, 5), Price: 15_000_000},
	}
	replaceStoreWithRates(t, home, rows, usdRate(day(2025, time.June, 2), 1_360_000))
}

func Test_run_holdings_as_of_a_year_lists_the_shares_after_a_split_before_it(t *testing.T) {
	seedSplitHoldingsStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--as-of", "2025"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Holdings on 2025-12-31 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsAsOfLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsAsOfLine("Brokerage", "Acme Corp (ACME)", "200", "12.00", "2025-12-30", "CAD", "2,400.00", "2,400.00")+
		holdingsAsOfLine("Total", "", "", "", "", "", "", "2,400.00"),
		stdout.String())
}

func Test_run_holdings_refuses_a_date_it_cannot_use(t *testing.T) {
	tests := []struct {
		name   string
		asOf   string
		stderr string
	}{
		{"not a date", "2024-13",
			`quarry: --as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` + "\n"},
		{"a day after today", "2099-01-01",
			"quarry: --as-of 2099-01-01 is after today; holdings are valued up to today only, so pass an earlier --as-of\n"},
		{"a year after today, quoted as typed", "2099",
			"quarry: --as-of 2099 is after today; holdings are valued up to today only, so pass an earlier --as-of\n"},
		{"a month after today, quoted as typed", "2026-11",
			"quarry: --as-of 2026-11 is after today; holdings are valued up to today only, so pass an earlier --as-of\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seedHoldingsStore(t)
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), []string{"holdings", "--as-of", tt.asOf},
				spendEnvAt(&stdout, &stderr, holdingsClock()))

			assert.Equal(t, 2, exitCode)
			assert.Equal(t, tt.stderr, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}
