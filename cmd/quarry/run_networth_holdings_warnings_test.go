package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

// seedLeftOutHoldingsStore stores leftOutHoldingsRows under a fresh HOME.
func seedLeftOutHoldingsStore(t *testing.T) string {
	t.Helper()
	return seedLeftOutHoldingsRows(t, leftOutHoldingsRows())
}

// seedLeftOutHoldingsRows stores rows under a fresh HOME, which it returns.
func seedLeftOutHoldingsRows(t *testing.T, rows store.Rows) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	return home
}

// runNetWorthAtMarch12 runs networth with args at holdingsClock and returns its stdout and stderr.
func runNetWorthAtMarch12(t *testing.T, args ...string) (string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), append([]string{"networth"}, args...), spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

func unprefixed(line string) string { return strings.TrimPrefix(line, "quarry: warning: ") }

// leftOutHoldingsRows is holdingsRows plus, in the brokerage, 40 shares of Bare Fund (never priced),
// 5 of Mystery Fund (no currency) and 20 of Euro Fund (EUR), the last two priced on day 9.
func leftOutHoldingsRows() store.Rows {
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
	return rows
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

func Test_run_networth_history_counts_the_month_ends_on_which_an_account_held_an_unpriced_security(t *testing.T) {
	rows := leftOutHoldingsRows()
	rows.InvestmentTransactions[3].Date = time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	seedLeftOutHoldingsRows(t, rows)

	_, stderr := runNetWorthAtMarch12(t, "--since", "2026-01", "--until", "2027")

	assert.Equal(t, `quarry: warning: "Brokerage" holds a security with no price on 3 of the month ends listed, `+
		`so its balance leaves it out on those days; enter prices in Quicken, then run quarry sync`+"\n"+
		leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_json_lists_the_config_warning_before_the_holdings_warnings(t *testing.T) {
	home := seedLeftOutHoldingsStore(t)
	writeConfig(t, home, "snapshot.keep = 3\n")

	stdout, stderr := runNetWorthAtMarch12(t, "--json")

	var got struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, []string{
		configPath(home) + ": unknown key snapshot.keep; quarry ignores it",
		unprefixed(leftOutNoPriceLine), unprefixed(leftOutNoCurrencyLine), unprefixed(leftOutOtherCurrencyLine),
	}, got.Warnings)
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n"+
		leftOutNoPriceLine+"\n"+leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_in_native_currency_still_warns_about_a_holding_in_a_currency_quarry_does_not_convert(t *testing.T) {
	seedLeftOutHoldingsStore(t)

	_, stderr := runNetWorthAtMarch12(t, "--currency", "native")

	assert.Equal(t, leftOutNoPriceLine+"\n"+leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_as_of_a_day_before_the_prices_warns_that_every_holding_is_unpriced(t *testing.T) {
	seedLeftOutHoldingsStore(t)

	_, stderr := runNetWorthAtMarch12(t, "--as-of", "2026-03-05")

	assert.Equal(t, `quarry: warning: "Brokerage" holds 4 securities with no price on or before 2026-03-05, `+
		`so its balance leaves them out; enter prices in Quicken, then run quarry sync`+"\n"+
		`quarry: warning: "IRA" holds 1 security with no price on or before 2026-03-05, `+
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`+"\n"+
		leftOutNoCurrencyLine+"\n", stderr)
}

func Test_run_networth_leaves_a_not_in_reports_accounts_unpriced_holding_out_of_its_warnings(t *testing.T) {
	rows := leftOutHoldingsRows()
	rows.Accounts = append(rows.Accounts, store.Account{
		ID: "acct-hidden", SourceID: 7, Name: "Hidden Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true, NotInReports: true,
	})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, holdingsBuy("inv-hidden", 7, "acct-hidden", "sec-bare", "CAD", 1_000_000))
	seedLeftOutHoldingsRows(t, rows)

	_, stderr := runNetWorthAtMarch12(t)

	assert.Equal(t, leftOutNoPriceLine+"\n"+leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_warns_about_a_closed_accounts_unpriced_holding(t *testing.T) {
	rows := leftOutHoldingsRows()
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, holdingsBuy("inv-old-bare", 7, "acct-old", "sec-bare", "CAD", 1_000_000))
	seedLeftOutHoldingsRows(t, rows)

	_, stderr := runNetWorthAtMarch12(t)

	assert.Equal(t, leftOutNoPriceLine+"\n"+
		`quarry: warning: "Old RRSP" holds 1 security with no price on or before 2026-03-12, `+
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`+"\n"+
		leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_before_the_first_rate_prints_every_holding_warning_before_the_rate_line(t *testing.T) {
	rows := leftOutHoldingsRows()
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, holdingsBuy("inv-vti-cad", 8, "acct-cad", "sec-vti", "CAD", 3_000_000))
	seedLeftOutHoldingsRows(t, rows)

	_, stderr := runNetWorthAtMarch12(t, "--as-of", "2026-03-09")

	assert.Equal(t, `quarry: warning: "Brokerage" holds 1 security with no price on or before 2026-03-09, `+
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`+"\n"+
		leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n"+
		`quarry: warning: "Brokerage" holds 1 USD security valued on 2026-03-09, before 2026-03-10, `+
		`the first exchange rate in the store, so its CAD balance leaves it out`+"\n"+
		`quarry: warning: USD balances on 2026-03-09, before 2026-03-10, the first exchange rate in the store, `+
		`are not converted to CAD and are left out of the CAD total; pass --currency native to list them`+"\n", stderr)
}
