package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	noRateHoldingBeforeFirstRate = `quarry: warning: "Brokerage" holds 1 USD security valued on %s, before %s, ` +
		`the first exchange rate in the store, so its CAD balance leaves it out` + "\n"
	noRateHoldingNoRates = `quarry: warning: "Brokerage" holds a USD security and the store has no exchange rates, ` +
		`so its CAD balance leaves it out; run quarry sync to fetch rates` + "\n"
)

// noRateHoldingRows is holdingsRows plus 10 shares of Dollar Fund, priced in USD, in the CAD brokerage from
// 2026-01-15, priced on 2026-01-20.
func noRateHoldingRows() store.Rows {
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-usd", SourceID: 4, Name: "Dollar Fund", Ticker: new("DOLR"), Currency: new("USD")})
	buy := holdingsBuy("inv-usd", 4, "acct-cad", "sec-usd", "CAD", 10_000_000)
	buy.Date = day(2026, time.January, 15)
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, buy)
	rows.Prices = append(rows.Prices, store.Price{SecurityID: "sec-usd", SourceID: 4, Date: day(2026, time.January, 20), Price: 10_000_000})
	return rows
}

// seedNoRateHoldingStore stores noRateHoldingRows under a fresh HOME with rates.
func seedNoRateHoldingStore(t *testing.T, rates ...store.Rate) {
	t.Helper()
	home := newHome(t)
	replaceStoreWithRates(t, home, noRateHoldingRows(), rates...)
}

func Test_run_networth_before_the_first_rate_warns_that_a_usd_holding_in_a_cad_account_is_left_out(t *testing.T) {
	seedNoRateHoldingStore(t, usdRate(holdingsDay(10), 1_360_000))

	_, stderr := runNetWorthAtMarch12(t, "--as-of", "2026-02-28")

	assert.Equal(t, fmt.Sprintf(noRateHoldingBeforeFirstRate, "2026-02-28", "2026-03-10"), stderr)
}

func Test_run_networth_history_counts_the_month_ends_a_usd_holding_in_a_cad_account_lacked_a_rate(t *testing.T) {
	seedNoRateHoldingStore(t, usdRate(holdingsDay(10), 1_360_000))

	_, stderr := runNetWorthAtMarch12(t, "--since", "2026-01", "--until", "2026-03")

	assert.Equal(t, `quarry: warning: "Brokerage" holds a USD security on 2 of the month ends listed, before 2026-03-10, `+
		`the first exchange rate in the store, so its CAD balance leaves it out on those days`+"\n", stderr)
}

func Test_run_networth_native_still_warns_that_a_usd_holding_in_a_cad_account_has_no_rate(t *testing.T) {
	seedNoRateHoldingStore(t, usdRate(holdingsDay(10), 1_360_000))

	_, stderr := runNetWorthAtMarch12(t, "--currency", "native", "--as-of", "2026-02-28")

	assert.Equal(t, fmt.Sprintf(noRateHoldingBeforeFirstRate, "2026-02-28", "2026-03-10"), stderr)
}

func Test_run_networth_in_a_store_with_no_rates_warns_of_the_holding_before_the_rate_line(t *testing.T) {
	seedNoRateHoldingStore(t)

	_, stderr := runNetWorthAtMarch12(t)

	assert.Equal(t, noRateHoldingNoRates+fmt.Sprintf(netWorthNoRatesNote, "USD", "CAD", "CAD"), stderr)
}

// accounts reads today from the store's own clock, so its as-of date is matched, not pinned.
func Test_run_accounts_warns_of_a_usd_holding_in_a_cad_account_before_the_first_rate_before_the_rate_line(t *testing.T) {
	seedNoRateHoldingStore(t, usdRate(day(2099, time.January, 1), 1_360_000))

	exitCode, _, stderr := runSpendCaptureAt(context.Background(), []string{"accounts"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Regexp(t, `^quarry: warning: "Brokerage" holds 1 USD security valued on \d{4}-\d{2}-\d{2}, before 2099-01-01, `+
		`the first exchange rate in the store, so its CAD balance leaves it out$`, lines[0])
	assert.Equal(t, "quarry: warning: the first exchange rate in the store, 2099-01-01, is dated after today, "+
		"so USD balances show no rate in the In CAD column; check the Mac's date and time", lines[1])
}

func Test_run_accounts_in_a_store_with_no_rates_warns_of_the_holding_before_the_rate_line(t *testing.T) {
	seedNoRateHoldingStore(t)

	exitCode, _, stderr := runSpendCaptureAt(context.Background(), []string{"accounts"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, noRateHoldingNoRates+"quarry: warning: the store has no exchange rates, so USD balances show no rate in the "+
		"In CAD column; run quarry sync to fetch them\n", stderr.String())
}
