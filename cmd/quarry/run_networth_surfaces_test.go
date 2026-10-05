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
func Test_run_networth_help_carries_its_copy_and_the_currency_flag(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"networth", "--help"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `Show net worth on one day (--as-of, default today), or at the end of each
month from --since to --until: account balances added up by account type
and currency. A balance is the sum of the account's transactions dated
that day or earlier; a brokerage or retirement account adds the value of
its holdings that day, each at the latest price Quicken recorded on or
before it (quarry holdings lists them). Credit card, loan and other
liability balances are negative, so they reduce the total. Closed
accounts count with their balance on the day. Accounts Quicken leaves out
of reports ("not in reports" or "linked tracking" in quarry accounts) are
left out, as Quicken's reports do.

Amounts are in CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
Each account's balance converts at the Bank of Canada rate for the day it
is valued on, or the latest earlier rate, and is rounded to the cent
before it is added. With --currency native, CAD and USD are listed
separately, never added together.

Month ends after today are not listed; a history that reaches this month
ends with today.
`)
	assert.Contains(t, stdout.String(), `Examples:
  quarry networth
  quarry networth --as-of 2025-12-31
  quarry networth --since 2020 --currency native --json
`)
	assert.Contains(t, stdout.String(), "      --as-of date      value net worth on date "+
		"(YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day; default today)\n")
	assert.Contains(t, stdout.String(), "      --since date      list net worth at each month end on or after date "+
		"(YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year when --until is given)\n")
	assert.Contains(t, stdout.String(), "      --until date      list net worth at each month end on or before date "+
		"(YYYY, YYYY-MM or YYYY-MM-DD; default today; a later date means today)\n")
	assert.Contains(t, stdout.String(), "      --currency code   show amounts in currency code: CAD, USD, or native for each account's own "+
		"(default reporting.currency in the config file, else CAD)\n")
}
