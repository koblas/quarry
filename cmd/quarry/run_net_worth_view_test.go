package main

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sql_sums_net_worth_by_type_and_currency_over_the_accounts_quickens_reports_count(t *testing.T) {
	home := newHome(t)
	day := func(d int) time.Time { return time.Date(2026, time.March, d, 0, 0, 0, 0, time.UTC) }
	closed := chequingAccount("acct-closed", 2)
	closed.Name, closed.Closed, closed.Active = "Old Chequing", true, false
	notInReports := chequingAccount("acct-out", 4)
	notInReports.Name, notInReports.NotInReports = "Not In Reports", true
	linked := chequingAccount("acct-linked", 5)
	linked.Name, linked.LinkedTracking = "Linked", true
	rows := spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), closed, usdChequingAccount("acct-usd", 3), notInReports, linked},
		spendSplit{id: "cad", account: "acct-cad", currency: "CAD", day: day(2), cents: 100_000},
		spendSplit{id: "closed", account: "acct-closed", currency: "CAD", day: day(2), cents: 25_000},
		spendSplit{id: "usd", account: "acct-usd", currency: "USD", day: day(2), cents: 80_000},
		spendSplit{id: "out", account: "acct-out", currency: "CAD", day: day(2), cents: 5_000},
		spendSplit{id: "linked", account: "acct-linked", currency: "CAD", day: day(2), cents: 7_000},
	)
	replaceStoreWithRates(t, home, rows, usdRate(day(5), 1_250_000))
	const query = `SELECT date, type, currency, accounts, balance, balance_cad, balance_usd
		FROM v_net_worth WHERE date = '2026-03-10' ORDER BY currency`

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "--csv", query})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"date,type,currency,accounts,balance,balance_cad,balance_usd\n"+
		"2026-03-10,chequing,CAD,2,1250.00,1250.00,1000.00\n"+
		"2026-03-10,chequing,USD,1,800.00,1000.00,800.00\n",
		stdout.String())
}
