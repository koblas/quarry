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

// searchStore is the store the search tests share, in insertion order oldest first (the same-date
// pair lowest source id first), so a listing that kept insertion order would be oldest first:
//
//	Tracked Loan: txn-linked 2026-01-20
//	Old Card: txn-left-out 2026-02-01 (not in reports)
//	Chequing: txn-marked 2026-02-14 (marked excluded), txn-low and txn-high 2026-03-10, txn-out 2026-03-20 (transfer leg),
//	  txn-split 2026-04-02 (two splits, inserted fuel first)
//	Savings: txn-in 2026-03-20 (other leg)
func searchStore() store.Rows {
	chequing := chequingAccount("acct-chq", 1)
	savings := store.Account{ID: "acct-sav", SourceID: 2, Name: "Savings", Type: "savings", Currency: "CAD", Active: true}
	oldCard := store.Account{ID: "acct-old", SourceID: 3, Name: "Old Card", Type: "credit", Currency: "CAD", Active: true, NotInReports: true}
	tracked := store.Account{ID: "acct-trk", SourceID: 4, Name: "Tracked Loan", Type: "loan", Currency: "CAD", Active: true, LinkedTracking: true}
	return searchRows([]store.Account{chequing, savings, oldCard, tracked},
		[][2]string{{"split-out-0", "split-in-0"}},
		searchTxn{
			id: "linked", account: "acct-trk", sourceID: 1, day: day(2026, time.January, 20),
			splits: []searchSplit{{category: "cat-fuel", sourceID: 1, cents: -3000}},
		},
		searchTxn{
			id: "left-out", account: "acct-old", sourceID: 2, day: day(2026, time.February, 1),
			splits: []searchSplit{{category: "cat-groceries", sourceID: 1, cents: -2500}},
		},
		searchTxn{
			id: "marked", account: "acct-chq", payee: "Bakery", memo: "hold for refund", sourceID: 5, day: day(2026, time.February, 14), excluded: true,
			splits: []searchSplit{{category: "cat-fuel", sourceID: 1, cents: -1999}},
		},
		searchTxn{
			id: "low", account: "acct-chq", payee: "Bakery", sourceID: 3, day: day(2026, time.March, 10),
			splits: []searchSplit{{category: "cat-groceries", sourceID: 1, cents: -850}},
		},
		searchTxn{
			id: "high", account: "acct-chq", payee: "Costco", sourceID: 7, day: day(2026, time.March, 10),
			splits: []searchSplit{{category: "cat-groceries", sourceID: 1, cents: -4217}},
		},
		searchTxn{
			id: "out", account: "acct-chq", sourceID: 11, day: day(2026, time.March, 20),
			splits: []searchSplit{{transferTo: "acct-sav", sourceID: 1, cents: -50000}},
		},
		searchTxn{
			id: "in", account: "acct-sav", sourceID: 12, day: day(2026, time.March, 20),
			splits: []searchSplit{{transferTo: "acct-chq", sourceID: 1, cents: 50000}},
		},
		searchTxn{
			id: "split", account: "acct-chq", payee: "Costco", memo: "bulk run", sourceID: 20, day: day(2026, time.April, 2),
			splits: []searchSplit{
				{category: "cat-fuel", sourceID: 2, cents: -4000},
				{category: "cat-groceries", memo: "milk and eggs", sourceID: 1, cents: -6000},
			},
		},
	)
}

func Test_run_search_json_lists_every_transaction_newest_first_flagged_with_its_splits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, searchStore())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"search", "--json"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, searchJSONDoc{
		AccountFilter: []recurringIDName{},
		Limit:         500,
		Matched:       9,
		Transactions: []searchTransactionJSON{
			{
				TransactionID: "txn-split", Date: "2026-04-02", AccountID: "acct-chq", Account: "Chequing",
				Payee: new("Costco"), Memo: new("bulk run"), Amount: "-100.00", Currency: "CAD",
				Splits: []searchSplitJSON{
					{Category: new("Food:Groceries"), Memo: new("milk and eggs"), Amount: "-60.00"},
					{Category: new("Auto:Fuel"), Amount: "-40.00"},
				},
			},
			{
				TransactionID: "txn-in", Date: "2026-03-20", AccountID: "acct-sav", Account: "Savings",
				Amount: "500.00", Currency: "CAD", Transfer: true,
				Splits: []searchSplitJSON{{Amount: "500.00", Transfer: true}},
			},
			{
				TransactionID: "txn-out", Date: "2026-03-20", AccountID: "acct-chq", Account: "Chequing",
				Amount: "-500.00", Currency: "CAD", Transfer: true,
				Splits: []searchSplitJSON{{Amount: "-500.00", Transfer: true}},
			},
			{
				TransactionID: "txn-high", Date: "2026-03-10", AccountID: "acct-chq", Account: "Chequing",
				Payee: new("Costco"), Amount: "-42.17", Currency: "CAD",
				Splits: []searchSplitJSON{{Category: new("Food:Groceries"), Amount: "-42.17"}},
			},
			{
				TransactionID: "txn-low", Date: "2026-03-10", AccountID: "acct-chq", Account: "Chequing",
				Payee: new("Bakery"), Amount: "-8.50", Currency: "CAD",
				Splits: []searchSplitJSON{{Category: new("Food:Groceries"), Amount: "-8.50"}},
			},
			{
				TransactionID: "txn-marked", Date: "2026-02-14", AccountID: "acct-chq", Account: "Chequing",
				Payee: new("Bakery"), Memo: new("hold for refund"), Amount: "-19.99", Currency: "CAD", Excluded: true,
				Splits: []searchSplitJSON{{Category: new("Auto:Fuel"), Amount: "-19.99"}},
			},
			{
				TransactionID: "txn-left-out", Date: "2026-02-01", AccountID: "acct-old", Account: "Old Card",
				Amount: "-25.00", Currency: "CAD", Excluded: true,
				Splits: []searchSplitJSON{{Category: new("Food:Groceries"), Amount: "-25.00"}},
			},
			{
				TransactionID: "txn-linked", Date: "2026-01-20", AccountID: "acct-trk", Account: "Tracked Loan",
				Amount: "-30.00", Currency: "CAD", Excluded: true,
				Splits: []searchSplitJSON{{Category: new("Auto:Fuel"), Amount: "-30.00"}},
			},
		},
		Warnings: []string{},
	}, decodeSearchJSON(t, stdout.String()))
}
