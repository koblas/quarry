// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/require"
)

// syncBundle runs sync on bundle under the test's HOME, failing t unless it succeeds.
func syncBundle(t *testing.T, bundle v9fixture.Bundle) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr), stderr.String())
}

// onlyFileWithSuffix fails the test unless exactly one entry in dir ends in
// suffix, returning its full path.
func onlyFileWithSuffix(t *testing.T, dir, suffix string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var found []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			found = append(found, e.Name())
		}
	}
	require.Len(t, found, 1, "expected exactly one %s file in %s, found %v", suffix, dir, found)
	return filepath.Join(dir, found[0])
}

// abbreviated mirrors the CLI's ~-abbreviation so the expected string is
// built from the real path, not a re-derived one.
func abbreviated(t *testing.T, path, home string) string {
	t.Helper()
	require.True(t, strings.HasPrefix(path, home+string(filepath.Separator)))
	return "~" + strings.TrimPrefix(path, home)
}

// megabytes mirrors the CLI's decimal-MB rounding for a fixture small enough
// that thousands-grouping never applies.
func megabytes(bytes int64) string {
	tenths := (bytes*10 + 500000) / 1000000
	return fmt.Sprintf("%d.%d MB", tenths/10, tenths%10)
}

// snapshotID mirrors the CLI's <id> derivation: a snapshot path's basename
// with the .sqlite extension removed.
func snapshotID(snapshotPath string) string {
	return strings.TrimSuffix(filepath.Base(snapshotPath), ".sqlite")
}

// skipAsRoot skips t under root, whom file modes do not stop.
func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
}

// replaceStore builds quarry's store under home straight from rows, skipping
// Quicken: view-level tests own every row the store holds.
func replaceStore(t *testing.T, home string, rows store.Rows) {
	t.Helper()
	_, err := duckstore.New(storeDirUnder(home)).Replace(context.Background(), rows)
	require.NoError(t, err)
}

// spendEnv is an env writing to stdout and stderr, with the clock at 2026-09-29 12:00 UTC.
func spendEnv(stdout, stderr *bytes.Buffer) cli.Env {
	return spendEnvAt(stdout, stderr, time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
}

// spendEnvAt is spendEnv with the clock at now.
func spendEnvAt(stdout, stderr *bytes.Buffer, now time.Time) cli.Env {
	e := defaultEnv(stdout, stderr)
	e.Now = func() time.Time { return now }
	return e
}

// chequingAccount is an active CAD chequing account named "Chequing".
func chequingAccount(id string, sourceID int64) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true}
}

// usdChequingAccount is an active USD chequing account named "US Chequing".
func usdChequingAccount(id string, sourceID int64) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: "US Chequing", Type: "chequing", Currency: "USD", Active: true}
}

// utcMinus5 is a zone whose evening is already the next day in UTC.
var utcMinus5 = time.FixedZone("UTC-5", -5*60*60)

// spendSplit is one single-split transaction; "" category or payee means none,
// negative cents is money out, and tags are tag ids.
type spendSplit struct {
	id, account, category, payee, currency string
	day                                    time.Time
	cents                                  int64
	tags                                   []string
}

// spendRows is a store of accounts holding splits, inside one import run. It
// always holds the same reference data, which a split names by id:
//
//	cat-fuel "Auto:Fuel", cat-groceries "Food:Groceries"
//	payee-costco "Costco", payee-bakery "Bakery"
//	tag-vacation "Vacation", tag-alpha "alpha"
//
// Both categories are referenced, so no store of it has an unused-category finding.
func spendRows(accounts []store.Account, splits ...spendSplit) store.Rows {
	rows := store.Rows{
		Accounts: accounts,
		Payees: []store.Payee{
			{ID: "payee-costco", SourceID: 1, Name: "Costco"},
			{ID: "payee-bakery", SourceID: 2, Name: "Bakery"},
		},
		Tags: []store.Tag{
			{ID: "tag-vacation", SourceID: 1, Name: "Vacation"},
			{ID: "tag-alpha", SourceID: 2, Name: "alpha"},
		},
		Categories: []store.Category{
			{ID: "cat-fuel", SourceID: 1, Name: "Fuel", FullPath: "Auto:Fuel", Kind: "expense"},
			{ID: "cat-groceries", SourceID: 2, Name: "Groceries", FullPath: "Food:Groceries", Kind: "expense"},
		},
		ReferencedCategoryIDs: []string{"cat-fuel", "cat-groceries"},
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
		var payee *string
		if s.payee != "" {
			payee = new(s.payee)
		}
		rows.Transactions = append(rows.Transactions, store.Transaction{
			ID: "txn-" + s.id, SourceID: 1, AccountID: s.account, Date: s.day,
			Amount: s.cents, Currency: s.currency, Status: "uncleared", PayeeID: payee,
		})
		rows.Splits = append(rows.Splits, store.Split{
			ID: "split-" + s.id, SourceID: 1, TransactionID: "txn-" + s.id, CategoryID: category, Amount: s.cents,
		})
		for _, tag := range s.tags {
			rows.SplitTags = append(rows.SplitTags, store.SplitTag{SplitID: "split-" + s.id, TagID: tag})
		}
	}
	return rows
}

// chargeSplit is one split of a chargeTxn; "" category means none, negative cents is money out.
type chargeSplit struct {
	category string
	cents    int64
}

// chargeTxn is one transaction of any number of splits; "" payee means none, and the
// payee is stored under the id "payee-<name>".
type chargeTxn struct {
	id, account, payee, currency string
	day                          time.Time
	splits                       []chargeSplit
}

// chargeRows is spendRows' reference data (without its payees) plus txns, each given the
// next source id from 1 and a payee row per distinct name, so a transaction may hold several splits.
func chargeRows(accounts []store.Account, txns ...chargeTxn) store.Rows {
	rows := spendRows(accounts)
	rows.Payees = nil
	known := map[string]bool{}
	for i, tx := range txns {
		var payeeID *string
		if tx.payee != "" {
			id := "payee-" + tx.payee
			if !known[id] {
				known[id] = true
				rows.Payees = append(rows.Payees, store.Payee{ID: id, SourceID: int64(len(rows.Payees) + 1), Name: tx.payee})
			}
			payeeID = &id
		}
		var amount int64
		for j, sp := range tx.splits {
			var category *string
			if sp.category != "" {
				category = new(sp.category)
			}
			amount += sp.cents
			rows.Splits = append(rows.Splits, store.Split{
				ID: fmt.Sprintf("split-%s-%d", tx.id, j), SourceID: int64(j + 1), TransactionID: "txn-" + tx.id,
				CategoryID: category, Amount: sp.cents,
			})
		}
		rows.Transactions = append(rows.Transactions, store.Transaction{
			ID: "txn-" + tx.id, SourceID: int64(i + 1), AccountID: tx.account, Date: tx.day,
			Amount: amount, Currency: tx.currency, Status: "uncleared", PayeeID: payeeID,
		})
	}
	return rows
}

// day is the civil day y-m-d at UTC midnight, as the store dates transactions.
func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// cashFlowRows is spendRows plus an income category, cat-salary.
func cashFlowRows(accounts []store.Account, splits ...spendSplit) store.Rows {
	rows := spendRows(accounts, splits...)
	rows.Categories = append(rows.Categories,
		store.Category{ID: "cat-salary", SourceID: 3, Name: "Salary", FullPath: "Income:Salary", Kind: "income"})
	return rows
}
