// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// viewRows is a store with one split of every kind the report views keep or
// leave out. s01-s02 and s08-s12 are the ones kept. It is not spendRows: the
// tests read split ids and bare category paths off the view, and it needs
// transfers, excluded transactions and system/income categories.
func viewRows() store.Rows {
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txn := func(id string, amount int64) store.Transaction {
		return store.Transaction{
			ID: "txn-" + id, SourceID: 1, AccountID: "acct-1", Date: day,
			Amount: amount, Currency: "CAD", Status: "uncleared",
		}
	}
	split := func(id string, category *string, amount int64) store.Split {
		return store.Split{ID: "s" + id, SourceID: 1, TransactionID: "txn-s" + id, CategoryID: category, Amount: amount}
	}
	excluded := txn("s07", -700)
	excluded.ExcludedFromReports = true
	transferLeg := split("03", nil, -5000)
	transferLeg.TransferAccountID = new("acct-1")
	transferIn := split("04", nil, 5000)
	transferIn.TransferAccountID = new("acct-1")

	return store.Rows{
		Accounts: []store.Account{chequingAccount("acct-1", 1)},
		Categories: []store.Category{
			{ID: "cat-groceries", SourceID: 1, Name: "Groceries", FullPath: "Groceries", Kind: "expense"},
			{ID: "cat-salary", SourceID: 2, Name: "Salary", FullPath: "Salary", Kind: "income"},
			{ID: "cat-adjustment", SourceID: 3, Name: "Adjustment", FullPath: "Adjustment", Kind: "system"},
			{ID: "cat-returns", SourceID: 4, Name: "Returns", FullPath: "Returns", Kind: "expense"},
		},
		Transactions: []store.Transaction{
			txn("s01", -10000), txn("s02", 250000), txn("s03", -5000), txn("s04", 5000), txn("s05", -2000),
			txn("s06", -500), excluded, txn("s08", -300), txn("s09", 400), txn("s10", 3000),
			txn("s11", -1000), txn("s12", 2500),
		},
		Splits: []store.Split{
			split("01", new("cat-groceries"), -10000),
			split("02", new("cat-salary"), 250000),
			transferLeg,
			transferIn,
			split("05", nil, -2000),
			split("06", new("cat-adjustment"), -500),
			split("07", new("cat-groceries"), -700),
			split("08", nil, -300),
			split("09", nil, 400),
			split("10", new("cat-groceries"), 3000),
			split("11", new("cat-returns"), -1000),
			split("12", new("cat-returns"), 2500),
		},
		Transfers: []store.Transfer{
			{ID: "xfer-1", FromSplitID: "s03", ToSplitID: new("s04")},
			{ID: "xfer-2", FromSplitID: "s05"},
		},
		ImportRuns: []store.ImportRun{{
			ID: 1, StartedAt: day, FinishedAt: day,
			Snapshot: store.SnapshotRef{Path: "/snapshots/20260315T000000Z.sqlite", SHA256: "9f86", SchemaFingerprint: "sha256:abc", TakenAt: day},
		}},
	}
}

func Test_run_sql_cash_flow_keeps_only_real_income_and_spending(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, viewRows())
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "SELECT split_id, flow, amount FROM v_cash_flow ORDER BY split_id"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"split_id  flow      amount\n"+
		"s01       expense  -100.00\n"+
		"s02       income   2500.00\n"+
		"s08       expense    -3.00\n"+
		"s09       income      4.00\n"+
		"s10       expense    30.00\n"+
		"s11       expense   -10.00\n"+
		"s12       expense    25.00\n",
		stdout.String())
}

func Test_run_sql_spending_nets_refunds_against_their_category(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, viewRows())
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "SELECT category, sum(spent) AS spent FROM v_spending GROUP BY category ORDER BY category"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"category    spent\n"+
		"Groceries   70.00\n"+
		"Returns    -15.00\n"+
		"NULL         3.00\n",
		stdout.String())
}
