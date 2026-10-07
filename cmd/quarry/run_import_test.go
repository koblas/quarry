// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	home := newHome(t)

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

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

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
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 2 accounts\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)",
		"Store", abbreviated(t, storePath, home),
		"Rows", "2 transactions, 4 splits, 0 transfers, 2 payees, 2 categories, 2 tags; 0 investment transactions, 0 securities, 0 prices",
		"Balances", "no accounts to check; 2 never reconciled",
		"Splits", "all 2 transactions equal the sum of their splits",
		"Shares", "no holdings to check",
		"Transfers", "none",
		"Findings", "none open",
		"Rates", fakeRatesText,
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
	home := newHome(t)
	quarryDir := filepath.Join(home, "Library", "Application Support", "quarry")
	require.NoError(t, os.MkdirAll(quarryDir, 0o700))
	storePath := filepath.Join(quarryDir, "quarry.duckdb")
	sentinel := []byte("previous store bytes, untouched by an unmappable value")
	require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Euro Savings", Type: "SAVINGS", Currency: "EUR", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

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
	assert.Equal(t, []string{"quarry.duckdb", "quarry.lock", "snapshots"}, entryNames(entries))
	after, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.Equal(t, sentinel, after)
}

func Test_run_records_an_import_runs_row_for_the_build(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	depositTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: depositTxn, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.00"})
	legTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: legTxn, Amount: "-5.00", QuickenID: 3001, Transfer: "Old Visa"})
	buyTxn := b.InvestmentTransaction(v9fixture.TransactionRow{
		Type: new(int64(3)), Account: brokeragePK, Amount: "-40.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: buyTxn, Amount: "-40.00"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, _, _ := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	raw, err := os.ReadFile(onlyFileWithSuffix(t, snapshotsDir, ".json"))
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"snapshot"`
		Schema struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"schema"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))

	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{"1": manifest.Snapshot.Path + " " + manifest.Snapshot.SHA256 + " " + manifest.Schema.Fingerprint},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), snapshot_path || ' ' || snapshot_sha256 || ' ' || schema_fingerprint FROM import_runs"))
	assert.Equal(t, map[string]string{"1": "2 0 0 0 3 3 0 1"},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), concat_ws(' ', accounts_rows, categories_rows, payees_rows, tags_rows, "+
			"transactions_rows, splits_rows, split_tags_rows, transfers_rows) FROM import_runs"))
	assert.Equal(t, map[string]string{"1": "1 0 0 1"},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), concat_ws(' ', balances_checked, balances_mismatched, splits_mismatched, "+
			"transfers_one_sided) FROM import_runs"))
	assert.Equal(t, map[string]string{"1": "true"},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), CAST(started_at <= finished_at AS VARCHAR) FROM import_runs"))
}

func Test_run_sync_from_keeps_the_earlier_build_in_import_runs(t *testing.T) {
	home := newHome(t)
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "DocumentsA"), "Chequing"))
	earlierManifest := onlyFileWithSuffix(t, snapshotsDir, ".json")
	earlierStore := filepath.Join(home, "earlier.duckdb")
	require.NoError(t, os.Rename(storePathUnder(home), earlierStore))
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "DocumentsB"), "Savings"))
	laterManifest := manifestOtherThan(t, snapshotsDir, earlierManifest)
	require.NoError(t, os.Rename(earlierStore, storePathUnder(home)))
	rowsBefore := importRunRowsAsText(t, home)

	exitCode, _, stderr := runCapture(context.Background(), []string{"sync", "--from", strings.TrimSuffix(filepath.Base(laterManifest), ".json")})

	require.Equal(t, 0, exitCode, stderr.String())
	rowsAfter := importRunRowsAsText(t, home)
	require.Len(t, rowsAfter, 2)
	assert.Equal(t, rowsBefore["1"], rowsAfter["1"])
	assert.Equal(t, manifestSHA256(t, earlierManifest), importRunSHA256(t, home, "1"))
	assert.Equal(t, manifestSHA256(t, laterManifest), importRunSHA256(t, home, "2"))
}

func Test_run_sync_from_warns_and_restarts_history_when_the_previous_store_is_not_a_duckdb_database(t *testing.T) {
	home := newHome(t)
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing"))
	manifest := onlyFileWithSuffix(t, filepath.Join(storeDirUnder(home), "snapshots"), ".json")
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a database"), 0o600))

	exitCode, _, stderr := runCapture(context.Background(), []string{"sync", "--from", strings.TrimSuffix(filepath.Base(manifest), ".json")})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: cannot carry import history, findings or exchange rates forward from the previous store (the file is not a DuckDB database); "+
		"all three start again with this sync\n", stderr.String())
	assert.Equal(t, map[string]string{"1": manifestSHA256(t, manifest)}, importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), snapshot_sha256 FROM import_runs"))
}

func Test_run_sync_from_warns_and_restarts_history_when_the_previous_run_has_the_largest_id(t *testing.T) {
	home := newHome(t)
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing"))
	id := snapshotID(onlyFileWithSuffix(t, filepath.Join(storeDirUnder(home), "snapshots"), ".sqlite"))
	editStore(t, home, "UPDATE import_runs SET id = 9223372036854775807")

	exitCode, _, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: cannot carry import history forward from the previous store (its import_runs table has an id too large to follow); "+
		"import_runs starts again with this sync\n", stderr)
	assert.Equal(t, map[string]string{"1": id + ".sqlite"}, importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), regexp_extract(snapshot_path, '[^/]+$') FROM import_runs"))
}

// writeNamedAccountBundle writes a bundle whose one account carries name, so two bundles hash differently.
func writeNamedAccountBundle(t *testing.T, dir, name string) v9fixture.Bundle {
	t.Helper()
	b := v9fixture.NewBuilder()
	account := b.Account(v9fixture.AccountRow{Name: name, Type: "CHECKING", Currency: "CAD", Active: true})
	addTransaction(b, account, "10.00", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	return b.WriteBundle(t, dir)
}

// manifestOtherThan returns the one snapshot manifest in dir that is not excluded.
func manifestOtherThan(t *testing.T, dir, excluded string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	var others []string
	for _, m := range matches {
		if m != excluded {
			others = append(others, m)
		}
	}
	require.Len(t, others, 1)
	return others[0]
}

// manifestSHA256 reads the snapshot sha256 a manifest records.
func manifestSHA256(t *testing.T, manifestPath string) string {
	t.Helper()
	raw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			SHA256 string `json:"sha256"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	return manifest.Snapshot.SHA256
}

// importRunRowsAsText maps each import_runs id to its whole row rendered as text, so every column takes part in a comparison.
func importRunRowsAsText(t *testing.T, home string) map[string]string {
	t.Helper()
	return importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), CAST(r AS VARCHAR) FROM import_runs r")
}

// importRunSHA256 reads the snapshot_sha256 of import run id.
func importRunSHA256(t *testing.T, home, id string) string {
	t.Helper()
	return importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), snapshot_sha256 FROM import_runs")[id]
}

func importRunQuery(t *testing.T, home, query string) map[string]string {
	t.Helper()
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	return stringMap(t, db, query)
}

// storeTextRows runs query (one string-shaped column) against the store under home and returns each row in query order.
func storeTextRows(t *testing.T, home, query string) []string {
	t.Helper()
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var got []string
	err = db.QueryRows(t.Context(), query, nil, func(scan func(dest ...any) error) error {
		var row string
		if err := scan(&row); err != nil {
			return err
		}
		got = append(got, row)
		return nil
	})
	require.NoError(t, err)
	return got
}
