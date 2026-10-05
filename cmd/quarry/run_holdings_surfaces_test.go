package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Needs run(): the help text is assembled with the flags, not by calling a command directly.
// The expected blocks are hand copies at the help's wrap width, so a re-wrap fails here.
func Test_run_accounts_and_sql_help_carry_the_holdings_copy(t *testing.T) {
	t.Setenv("HOME", "")
	var accountsOut, accountsErr, sqlOut, sqlErr bytes.Buffer

	accountsCode := run(context.Background(), []string{"accounts", "--help"}, &accountsOut, &accountsErr)
	sqlCode := run(context.Background(), []string{"sql", "--help"}, &sqlOut, &sqlErr)

	require.Equal(t, 0, accountsCode)
	require.Equal(t, 0, sqlCode)
	assert.Contains(t, accountsOut.String(), `Brokerage and retirement accounts' balance is the cash in them plus the
value of their holdings today, each at the latest price Quicken recorded
(quarry holdings lists them).`)
	assert.Contains(t, sqlOut.String(), `records none).
holding_shares holds each account's count of each security, one row per span
of days it is unchanged and not zero (from_date through to_date, NULL while
still held), splits applied; these are the counts quarry sync checks against
Quicken. v_holdings has one row per holding per day held, through today:
price is the latest on or before date and price_date its day (NULL when
none), value is shares times price rounded to the cent, value_cad and
value_usd convert it at the rate for date, as quarry holdings does; filter
it by date. Neither includes cash in investment accounts.`)
	assert.Contains(t, sqlOut.String(), `Neither includes cash in investment accounts. action is one of
add_shares, buy, capital_gain_long, capital_gain_short, dividend, interest,
margin_interest, misc_expense, misc_income, reinvest_dividend,
remove_shares, sell, split.`)
}

// Needs run(): the help text is assembled with the flags, not by calling a command directly.
func Test_run_holdings_help_ends_by_pointing_at_accounts_for_the_balance_with_cash(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"holdings", "--help"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Contains(t, stdout.String(), `The total is the value of the securities only, without the cash held in
investment accounts; quarry accounts shows each account's balance, cash
included.`)
}
