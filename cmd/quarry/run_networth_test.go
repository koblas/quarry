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

// netWorthLine is one converted net worth table line, each cell as wide as the fixture's widest.
func netWorthLine(typ, currency, balance, in string) string {
	return fmt.Sprintf("%-11s  %-8s  %8s  %8s\n", typ, currency, balance, in)
}

// netWorthNativeLine is netWorthLine without the In column.
func netWorthNativeLine(typ, currency, balance string) string {
	return fmt.Sprintf("%-11s  %-8s  %8s\n", typ, currency, balance)
}

// seedNetWorthStore builds the net worth store (CAD and USD accounts, a USD brokerage, a USD rate from March 10) under a temp HOME.
func seedNetWorthStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	march := func(d int) time.Time { return day(2026, time.March, d) }
	card := store.Account{ID: "acct-card", SourceID: 3, Name: "Card", Type: "credit_card", Currency: "CAD", Active: true}
	closed := closedAccount(chequingAccount("acct-closed", 5))
	closed.Name = "Old Chequing"
	notInReports := chequingAccount("acct-out", 6)
	notInReports.Name, notInReports.NotInReports = "Not In Reports", true
	savings := store.Account{ID: "acct-savings", SourceID: 7, Name: "Savings", Type: "savings", Currency: "CAD", Active: true}
	rows := spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2), card,
			brokerageAccount("acct-brokerage", 4, "USD"), closed, notInReports, savings,
		},
		spendSplit{id: "cad", account: "acct-cad", currency: "CAD", day: march(2), cents: 100_000},
		spendSplit{id: "usd", account: "acct-usd", currency: "USD", day: march(2), cents: 80_000},
		spendSplit{id: "card", account: "acct-card", currency: "CAD", day: march(2), cents: -25_050},
		spendSplit{id: "deposit", account: "acct-brokerage", currency: "USD", day: march(2), cents: 100_000},
		spendSplit{id: "closed", account: "acct-closed", currency: "CAD", day: march(2), cents: 2_500},
		spendSplit{id: "out", account: "acct-out", currency: "CAD", day: march(2), cents: 500},
		spendSplit{id: "saved", account: "acct-savings", currency: "CAD", day: march(2), cents: 5_000},
		spendSplit{id: "spent", account: "acct-savings", currency: "CAD", day: march(2), cents: -5_000},
	)
	rows.Securities = []store.Security{{ID: "sec-vti", SourceID: 1, Name: "Vanguard Total Stock", Ticker: new("VTI"), Currency: new("USD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{{
		ID: "inv-vti", SourceID: 1, AccountID: "acct-brokerage", SecurityID: new("sec-vti"), Date: march(2),
		Action: store.ActionBuy, Shares: new(int64(2_000_000)), Amount: -10_000, Currency: "USD",
	}}
	rows.Prices = []store.Price{{SecurityID: "sec-vti", SourceID: 1, Date: march(1), Price: 10_000_000}}
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-inv-vti", SourceID: 99, AccountID: "acct-brokerage", Date: march(2), Amount: -10_000, Currency: "USD",
		Status: "uncleared", InvestmentTransactionID: new("inv-vti"),
	})
	rows.Splits = append(rows.Splits, store.Split{ID: "split-inv-vti", SourceID: 99, TransactionID: "txn-inv-vti", Amount: -10_000})
	replaceStoreWithRates(t, home, rows, usdRate(march(10), 1_360_000))
}

func Test_run_networth_prints_todays_balances_by_type_and_currency_with_a_total_in_the_reporting_currency(t *testing.T) {
	seedNetWorthStore(t)

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"networth"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Net worth on 2026-03-12, amounts in CAD\n\n"+
		netWorthLine("Type", "Currency", "Balance", "In CAD")+
		netWorthLine("brokerage", "USD", "920.00", "1,251.20")+
		netWorthLine("chequing", "CAD", "1,025.00", "1,025.00")+
		netWorthLine("chequing", "USD", "800.00", "1,088.00")+
		netWorthLine("credit_card", "CAD", "-250.50", "-250.50")+
		netWorthLine("Total", "", "", "3,113.70"),
		stdout.String())
}

func Test_run_networth_lists_cad_and_usd_separately_with_one_total_each_in_native_mode(t *testing.T) {
	seedNetWorthStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--currency", "native"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Net worth on 2026-03-12\n\n"+
		netWorthNativeLine("Type", "Currency", "Balance")+
		netWorthNativeLine("brokerage", "USD", "920.00")+
		netWorthNativeLine("chequing", "CAD", "1,025.00")+
		netWorthNativeLine("chequing", "USD", "800.00")+
		netWorthNativeLine("credit_card", "CAD", "-250.50")+
		netWorthNativeLine("Total", "CAD", "774.50")+
		netWorthNativeLine("Total", "USD", "1,720.00"),
		stdout.String())
}
