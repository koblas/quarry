package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	leftOutNoPriceLine = `quarry: warning: "Brokerage" holds 1 security with no price on or before 2026-03-12, ` +
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`
	leftOutNoCurrencyLine = `quarry: warning: "Mystery Fund" has no currency in Quicken, so quarry leaves its value out of ` +
		`"Brokerage"'s balance; set its currency in Quicken, then run quarry sync`
	leftOutOtherCurrencyLine = `quarry: warning: "Euro Fund" is priced in EUR, which quarry does not convert, ` +
		`so its value is left out of "Brokerage"'s balance`
)

// seedLeftOutHoldingsStore is seedHoldingsStore plus, in the brokerage, 40 shares of Bare Fund (never priced),
// 5 of Mystery Fund (no currency) and 20 of Euro Fund (EUR), the last two priced on day 9.
func seedLeftOutHoldingsStore(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-bare", SourceID: 4, Name: "Bare Fund", Ticker: new("BARE"), Currency: new("CAD")},
		store.Security{ID: "sec-null", SourceID: 5, Name: "Mystery Fund", Ticker: new("MYST")},
		store.Security{ID: "sec-eur", SourceID: 6, Name: "Euro Fund", Ticker: new("EURO"), Currency: new("EUR")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		holdingsBuy("inv-bare", 4, "acct-cad", "sec-bare", "CAD", 40_000_000),
		holdingsBuy("inv-null", 5, "acct-cad", "sec-null", "CAD", 5_000_000),
		holdingsBuy("inv-eur", 6, "acct-cad", "sec-eur", "CAD", 20_000_000))
	rows.Prices = append(rows.Prices,
		store.Price{SecurityID: "sec-null", SourceID: 5, Date: holdingsDay(9), Price: 10_000_000},
		store.Price{SecurityID: "sec-eur", SourceID: 6, Date: holdingsDay(9), Price: 12_500_000})
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
}

// accounts reads today from the store's own clock, so its as-of date is matched, not pinned.
func Test_run_networth_warns_about_each_holding_it_leaves_out_and_accounts_gives_the_same_lines(t *testing.T) {
	seedLeftOutHoldingsStore(t)
	var netWorthOut, netWorthErr, accountsOut, accountsErr bytes.Buffer

	netWorthExit := runWith(context.Background(), []string{"networth"}, spendEnvAt(&netWorthOut, &netWorthErr, holdingsClock()))
	accountsExit := runWith(context.Background(), []string{"accounts"}, spendEnvAt(&accountsOut, &accountsErr, holdingsClock()))

	require.Equal(t, 0, netWorthExit, netWorthErr.String())
	assert.Equal(t, leftOutNoPriceLine+"\n"+leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", netWorthErr.String())
	require.Equal(t, 0, accountsExit, accountsErr.String())
	accountsLines := strings.Split(strings.TrimSuffix(accountsErr.String(), "\n"), "\n")
	require.Len(t, accountsLines, 3)
	assert.Regexp(t, `^quarry: warning: "Brokerage" holds 1 security with no price on or before \d{4}-\d{2}-\d{2}, `+
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync$`, accountsLines[0])
	assert.Equal(t, []string{leftOutNoCurrencyLine, leftOutOtherCurrencyLine}, accountsLines[1:])
}
