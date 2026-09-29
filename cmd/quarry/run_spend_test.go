// run is unexported, so its tests live in package main rather than
// importing main from outside.
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

// utcMinus5 is a zone whose evening is already the next day in UTC.
var utcMinus5 = time.FixedZone("UTC-5", -5*60*60)

// spendSplit is one single-split transaction in a currency on a day, its
// category id "" for uncategorized; cents is negative for money out.
type spendSplit struct {
	id, account, category, currency string
	day                             time.Time
	cents                           int64
}

// spendRows is a store of accounts holding splits, inside one import run.
func spendRows(accounts []store.Account, splits ...spendSplit) store.Rows {
	rows := store.Rows{
		Accounts: accounts,
		Categories: []store.Category{
			{ID: "cat-fuel", SourceID: 1, Name: "Fuel", FullPath: "Auto:Fuel", Kind: "expense"},
			{ID: "cat-groceries", SourceID: 2, Name: "Groceries", FullPath: "Food:Groceries", Kind: "expense"},
		},
		ImportRuns: []store.ImportRun{{
			ID: 1, StartedAt: time.Unix(0, 0).UTC(), FinishedAt: time.Unix(0, 0).UTC(),
			Snapshot: store.SnapshotRef{Path: "/snapshots/20260929T000000Z.sqlite", SHA256: "9f86", SchemaFingerprint: "sha256:abc"},
		}},
	}
	for _, s := range splits {
		var category *string
		if s.category != "" {
			category = new(s.category)
		}
		rows.Transactions = append(rows.Transactions, store.Transaction{
			ID: "txn-" + s.id, SourceID: 1, AccountID: s.account, Date: s.day,
			Amount: s.cents, Currency: s.currency, Status: "uncleared",
		})
		rows.Splits = append(rows.Splits, store.Split{
			ID: "split-" + s.id, SourceID: 1, TransactionID: "txn-" + s.id, CategoryID: category, Amount: s.cents,
		})
	}
	return rows
}

// day is the civil day y-m-d at UTC midnight, as the store dates transactions.
func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func Test_run_spend_shows_this_years_spending_by_category_in_each_currency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows(
		[]store.Account{
			{ID: "acct-cad", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
			{ID: "acct-usd", SourceID: 2, Name: "US Chequing", Type: "chequing", Currency: "USD", Active: true},
		},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 9, 29), cents: -1000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 2), cents: -120450},
		spendSplit{id: "s04", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -4208},
		spendSplit{id: "s05", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -31210},
		spendSplit{id: "s06", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2025, 12, 31), cents: -99900},
		spendSplit{id: "s07", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 9, 30), cents: -77700},
	))
	var stdout, stderr bytes.Buffer
	env := defaultEnv(&stdout, &stderr)
	env.Now = func() time.Time { return time.Date(2026, 9, 29, 22, 0, 0, 0, utcMinus5) }

	exitCode := runWith(context.Background(), []string{"spend"}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const row = "%-15s  %-8s  %8s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "(uncategorized)", "CAD", "42.08")+
		fmt.Sprintf(row, "Auto:Fuel", "CAD", "1,204.50")+
		fmt.Sprintf(row, "Food:Groceries", "CAD", "133.45")+
		fmt.Sprintf(row, "Food:Groceries", "USD", "312.10")+
		fmt.Sprintf(row, "Total", "CAD", "1,380.03")+
		fmt.Sprintf(row, "Total", "USD", "312.10"),
		stdout.String())
}

func Test_run_spend_leaves_out_accounts_quicken_does_not_use_in_reports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows(
		[]store.Account{
			{ID: "acct-in", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
			{ID: "acct-out", SourceID: 2, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
		},
		spendSplit{id: "s01", account: "acct-in", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -2500},
		spendSplit{id: "s02", account: "acct-out", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 11), cents: -900},
	))
	var stdout, stderr bytes.Buffer
	env := defaultEnv(&stdout, &stderr)
	env.Now = func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }

	exitCode := runWith(context.Background(), []string{"spend"}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const row = "%-14s  %-8s  %5s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "Food:Groceries", "CAD", "25.00")+
		fmt.Sprintf(row, "Total", "CAD", "25.00"),
		stdout.String())
}
