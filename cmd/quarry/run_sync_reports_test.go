// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncedStore syncs the bundle build describes under a fresh HOME and opens the store read-only.
func syncedStore(t *testing.T, build func(b *v9fixture.Builder)) *duckdb.DB {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	build(b)
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	db, err := duckdb.OpenReadOnly(t.Context(), filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func Test_run_sync_records_which_transactions_are_excluded_from_reports(t *testing.T) {
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	var excludedPK, includedPK int64
	db := syncedStore(t, func(b *v9fixture.Builder) {
		acct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
		excludedPK = b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-5.00", PostedDate: &day, ExcludeFromReports: new(int64(1))})
		b.Entry(v9fixture.EntryRow{Parent: excludedPK, Amount: "-5.00"})
		includedPK = b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-7.00", PostedDate: &day})
		b.Entry(v9fixture.EntryRow{Parent: includedPK, Amount: "-7.00"})
	})

	got := stringMap(t, db, "SELECT id, CAST(excluded_from_reports AS VARCHAR) FROM transactions")

	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", excludedPK): "true",
		fmt.Sprintf("txn-%d", includedPK): "false",
	}, got)
}

func Test_run_sync_records_which_accounts_are_used_in_reports(t *testing.T) {
	var offPK, onPK, unsetPK int64
	db := syncedStore(t, func(b *v9fixture.Builder) {
		offPK = b.Account(v9fixture.AccountRow{Name: "Off", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: new(int64(0))})
		onPK = b.Account(v9fixture.AccountRow{Name: "On", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: new(int64(1))})
		unsetPK = b.Account(v9fixture.AccountRow{Name: "Unset", Type: "CHECKING", Currency: "CAD", Active: true})
	})

	got := stringMap(t, db, "SELECT id, CAST(in_reports AS VARCHAR) FROM accounts")

	assert.Equal(t, map[string]string{
		fmt.Sprintf("acct-%d", offPK):   "false",
		fmt.Sprintf("acct-%d", onPK):    "true",
		fmt.Sprintf("acct-%d", unsetPK): "true",
	}, got)
}

func Test_run_sync_dates_each_transaction_by_its_register_date(t *testing.T) {
	entered := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	posted := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	var bothPK, enteredOnlyPK int64
	db := syncedStore(t, func(b *v9fixture.Builder) {
		acct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
		bothPK = b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-5.00", EnteredDate: &entered, PostedDate: &posted})
		b.Entry(v9fixture.EntryRow{Parent: bothPK, Amount: "-5.00"})
		enteredOnlyPK = b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-7.00", EnteredDate: &entered})
		b.Entry(v9fixture.EntryRow{Parent: enteredOnlyPK, Amount: "-7.00"})
	})

	dates := stringMap(t, db, "SELECT id, CAST(date AS VARCHAR) FROM transactions")
	postedDates := stringMap(t, db, "SELECT id, COALESCE(CAST(posted_date AS VARCHAR), 'NULL') FROM transactions")

	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", bothPK):        "2026-06-01",
		fmt.Sprintf("txn-%d", enteredOnlyPK): "2026-06-01",
	}, dates)
	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", bothPK):        "2026-05-31",
		fmt.Sprintf("txn-%d", enteredOnlyPK): "NULL",
	}, postedDates)
}

func Test_run_sync_stores_uncategorized_splits_with_no_category(t *testing.T) {
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	var uncategorizedSplitPK int64
	db := syncedStore(t, func(b *v9fixture.Builder) {
		acct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
		uncategorizedPK := b.Category(v9fixture.TagRow{Name: "Uncategorized", Type: new(int64(0))})
		txn := b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-5.00", PostedDate: &day})
		uncategorizedSplitPK = b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-5.00", CategoryTag: uncategorizedPK})
	})

	got := stringMap(t, db, "SELECT id, COALESCE(category_id, 'NULL') FROM splits")

	assert.Equal(t, map[string]string{fmt.Sprintf("split-%d", uncategorizedSplitPK): "NULL"}, got)
}
