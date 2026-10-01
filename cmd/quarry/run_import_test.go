// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stringMap runs query (two string-shaped columns) and returns the first
// column mapped to the second, for asserting a table's rows by id without
// depending on read order.
func stringMap(t *testing.T, db *duckdb.DB, query string) map[string]string {
	t.Helper()
	got := make(map[string]string)
	err := db.QueryRows(t.Context(), query, nil, func(scan func(dest ...any) error) error {
		var k, v string
		if err := scan(&k, &v); err != nil {
			return err
		}
		got[k] = v
		return nil
	})
	require.NoError(t, err)
	return got
}

// No ZTRANSFER legs, so Rows counts 0 transfers and Transfers says none.
func Test_run_imports_the_quicken_data_into_a_new_store(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1)), ParentCategory: foodPK})
	coffeeShopPK := b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	groceryStorePK := b.Payee(v9fixture.PayeeRow{Name: "Grocery Store"})
	reimbursablePK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	businessPK := b.UserTag(v9fixture.TagRow{Name: "Business"})

	day1 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txn1PK := b.Transaction(v9fixture.TransactionRow{
		Account: chequingPK, Amount: "12.34", PostedDate: &day1, Payee: coffeeShopPK,
	})
	split1PK := b.Entry(v9fixture.EntryRow{Parent: txn1PK, Amount: "7.00", CategoryTag: foodPK})
	split2PK := b.Entry(v9fixture.EntryRow{Parent: txn1PK, Amount: "5.34", CategoryTag: groceriesPK})
	b.LinkUserTag(split1PK, reimbursablePK)

	day2 := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	txn2PK := b.Transaction(v9fixture.TransactionRow{
		Account: savingsPK, Amount: "-50.00", PostedDate: &day2, Payee: groceryStorePK,
	})
	split3PK := b.Entry(v9fixture.EntryRow{Parent: txn2PK, Amount: "-30.00", CategoryTag: groceriesPK})
	split4PK := b.Entry(v9fixture.EntryRow{Parent: txn2PK, Amount: "-20.00", CategoryTag: foodPK})
	b.LinkUserTag(split3PK, businessPK)

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())

	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")

	want := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 2 accounts\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)",
		"Store", abbreviated(t, storePath, home),
		"Rows", "2 transactions, 4 splits, 0 transfers, 2 payees, 2 categories, 2 tags",
		"Balances", "no accounts to check; 2 never reconciled",
		"Splits", "all 2 transactions equal the sum of their splits",
		"Transfers", "none",
		"Findings", "none open",
	)
	require.Equal(t, want, stdout.String())

	db, err := duckdb.OpenReadOnly(t.Context(), storePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	acctID := func(pk int64) string { return fmt.Sprintf("acct-%d", pk) }
	catID := func(pk int64) string { return fmt.Sprintf("cat-%d", pk) }
	payeeID := func(pk int64) string { return fmt.Sprintf("payee-%d", pk) }
	tagID := func(pk int64) string { return fmt.Sprintf("tag-%d", pk) }
	txnID := func(pk int64) string { return fmt.Sprintf("txn-%d", pk) }
	splitID := func(pk int64) string { return fmt.Sprintf("split-%d", pk) }

	assert.Equal(t, map[string]string{
		acctID(chequingPK): strconv.FormatInt(chequingPK, 10), acctID(savingsPK): strconv.FormatInt(savingsPK, 10),
	}, stringMap(t, db, "SELECT id, CAST(source_id AS VARCHAR) FROM accounts"))
	assert.Equal(t, map[string]string{
		acctID(chequingPK): "CAD", acctID(savingsPK): "USD",
	}, stringMap(t, db, "SELECT id, currency FROM accounts"))

	assert.Equal(t, map[string]string{
		catID(foodPK): strconv.FormatInt(foodPK, 10), catID(groceriesPK): strconv.FormatInt(groceriesPK, 10),
	}, stringMap(t, db, "SELECT id, CAST(source_id AS VARCHAR) FROM categories"))

	assert.Equal(t, map[string]string{
		payeeID(coffeeShopPK): strconv.FormatInt(coffeeShopPK, 10), payeeID(groceryStorePK): strconv.FormatInt(groceryStorePK, 10),
	}, stringMap(t, db, "SELECT id, CAST(source_id AS VARCHAR) FROM payees"))

	assert.Equal(t, map[string]string{
		tagID(reimbursablePK): strconv.FormatInt(reimbursablePK, 10), tagID(businessPK): strconv.FormatInt(businessPK, 10),
	}, stringMap(t, db, "SELECT id, CAST(source_id AS VARCHAR) FROM tags"))

	assert.Equal(t, map[string]string{
		txnID(txn1PK): strconv.FormatInt(txn1PK, 10), txnID(txn2PK): strconv.FormatInt(txn2PK, 10),
	}, stringMap(t, db, "SELECT id, CAST(source_id AS VARCHAR) FROM transactions"))
	assert.Equal(t, map[string]string{
		txnID(txn1PK): "CAD", txnID(txn2PK): "USD",
	}, stringMap(t, db, "SELECT id, currency FROM transactions"))
	assert.Equal(t, map[string]string{
		txnID(txn1PK): "12.34", txnID(txn2PK): "-50.00",
	}, stringMap(t, db, "SELECT id, CAST(amount AS VARCHAR) FROM transactions"))

	assert.Equal(t, map[string]string{
		splitID(split1PK): strconv.FormatInt(split1PK, 10), splitID(split2PK): strconv.FormatInt(split2PK, 10),
		splitID(split3PK): strconv.FormatInt(split3PK, 10), splitID(split4PK): strconv.FormatInt(split4PK, 10),
	}, stringMap(t, db, "SELECT id, CAST(source_id AS VARCHAR) FROM splits"))
	assert.Equal(t, map[string]string{
		splitID(split1PK): "7.00", splitID(split2PK): "5.34",
		splitID(split3PK): "-30.00", splitID(split4PK): "-20.00",
	}, stringMap(t, db, "SELECT id, CAST(amount AS VARCHAR) FROM splits"))

	assert.Equal(t, map[string]string{
		splitID(split1PK): tagID(reimbursablePK), splitID(split3PK): tagID(businessPK),
	}, stringMap(t, db, "SELECT split_id, tag_id FROM split_tags"))
}

func Test_run_refuses_an_unmappable_value_and_keeps_the_snapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	quarryDir := filepath.Join(home, "Library", "Application Support", "quarry")
	require.NoError(t, os.MkdirAll(quarryDir, 0o700))
	storePath := filepath.Join(quarryDir, "quarry.duckdb")
	sentinel := []byte("previous store bytes, untouched by an unmappable value")
	require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Euro Savings", Type: "SAVINGS", Currency: "EUR", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	snapshotsDir := filepath.Join(quarryDir, "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	onlyFileWithSuffix(t, snapshotsDir, ".json")
	assert.Equal(t,
		"quarry: cannot import snapshot "+id+`: account "Euro Savings" uses currency EUR; quarry supports CAD and USD accounts; `+
			abbreviated(t, storePath, home)+" was not changed; run quarry sync --from "+id+" once quarry supports it\n",
		stderr.String())
	entries, err := os.ReadDir(quarryDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"quarry.duckdb", "snapshots"}, entryNames(entries))
	after, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.Equal(t, sentinel, after)
}
