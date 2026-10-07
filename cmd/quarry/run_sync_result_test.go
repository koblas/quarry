// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingWriter fails every Write with err, so a test can prove what happens
// when stdout itself cannot be written to (a full disk on the far end of a
// pipe, for example).
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func Test_run_writes_a_verified_snapshot_and_reports_success(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())

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
		"Rows", "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices",
		"Balances", "no accounts to check; 2 never reconciled",
		"Splits", "no transactions to check",
		"Shares", "no holdings to check",
		"Transfers", "none",
		"Findings", "none open",
		"Rates", fakeRatesText,
	)
	assert.Equal(t, want, stdout.String())
}

// The leftovers are backdated past the sweep's age gate: a fresh leftover
// could belong to another sync still in flight.
func Test_run_removes_leftover_partials_silently_before_syncing(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	require.NoError(t, os.MkdirAll(snapshotsDir, 0o700))
	leftover := filepath.Join(snapshotsDir, ".20260101T000000Z.sqlite.partial")
	require.NoError(t, os.WriteFile(leftover, []byte("crash debris"), 0o600))
	leftoverWAL := leftover + "-wal"
	require.NoError(t, os.WriteFile(leftoverWAL, []byte("wal"), 0o600))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(leftover, old, old))
	require.NoError(t, os.Chtimes(leftoverWAL, old, old))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Snapshot  ")
	assert.NotContains(t, stdout.String(), ".partial")
	_, err := os.Stat(leftover)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(leftoverWAL)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// Once the build was reached, a stdout write failure points at --from
// --json instead of the manifest: the store result no longer lives there alone.
func Test_run_points_to_from_when_writing_stdout_fails_after_the_build(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	writeErr := errNoSpace
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, failingWriter{err: writeErr}, &stderr)

	assert.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	onlyFileWithSuffix(t, snapshotsDir, ".json")
	assert.Equal(t,
		"quarry: cannot write the result to stdout: "+writeErr.Error()+"; run quarry sync --from "+
			snapshotID(snapshotPath)+" --json to see it again\n",
		stderr.String())
}

func Test_run_prints_the_manifest_as_json_with_the_json_flag(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	manifestBytes, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	assert.ElementsMatch(t, []string{"snapshot", "schema", "store", "pruned", "warnings"}, slices.Collect(maps.Keys(parsed)))
	assert.JSONEq(t, string(manifest["snapshot"]), string(parsed["snapshot"]))
	assert.JSONEq(t, string(manifest["schema"]), string(parsed["schema"]))
	assert.JSONEq(t, string(manifest["warnings"]), string(parsed["warnings"]))
}

// errNoSpace stands in for a stdout write failing on a full disk.
var errNoSpace = errors.New("no space left on device")

// One assert.JSONEq against a full literal catches a wrong type, a missing
// key, or a leaked display-only field (source_id, closed, active) at once.
func Test_run_reports_the_store_result_alongside_the_manifest_as_json(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	walletPK := b.Account(v9fixture.AccountRow{Name: "Wallet", Type: "SAVINGS", Currency: "CAD", Active: true})
	landlordPK := b.Payee(v9fixture.PayeeRow{Name: "Landlord"})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)

	pairOutTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-100.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: pairOutTxn, Amount: "-100.00", QuickenID: 1001, Transfer: "2002"})
	pairInTxn := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "100.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: pairInTxn, Amount: "100.00", QuickenID: 2002, Transfer: "1001"})

	missingTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-1.00", PostedDate: &day})
	missingLeg := b.Entry(v9fixture.EntryRow{Parent: missingTxn, Amount: "-1.00", QuickenID: 3001, Transfer: "999"})
	namedTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-500.00", PostedDate: &day, Payee: landlordPK})
	namedLeg := b.Entry(v9fixture.EntryRow{Parent: namedTxn, Amount: "-500.00", QuickenID: 3002, Transfer: "Savings"})
	noMatchTxn := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "-1204.17", PostedDate: &day})
	noMatchLeg := b.Entry(v9fixture.EntryRow{Parent: noMatchTxn, Amount: "-1204.17", QuickenID: 3003, Transfer: "Old Visa"})

	walletTxn := b.Transaction(v9fixture.TransactionRow{Account: walletPK, Amount: "250.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: walletTxn, Amount: "250.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: walletPK, EndDate: &day, EndingBalance: "250.00"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())

	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	manifestBytes, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	assert.JSONEq(t, string(manifest["snapshot"]), string(parsed["snapshot"]))
	assert.JSONEq(t, string(manifest["schema"]), string(parsed["schema"]))

	assert.JSONEq(t, `[]`, string(parsed["warnings"]))

	wantStore := fmt.Sprintf(`{
		"path": %q,
		"built": true,
		"rows": {"accounts":3,"categories":0,"payees":1,"tags":0,"transactions":6,"splits":6,"split_tags":0,"transfers":4,"investment_transactions":0,"securities":0,"prices":0},
		"balances": {
			"checked": 1,
			"mismatched": [],
			"never_reconciled": [
				{"id":"acct-%d","name":"Chequing","currency":"CAD","closed":false,"active":true},
				{"id":"acct-%d","name":"Savings","currency":"CAD","closed":false,"active":true}
			],
			"investment_accounts": 0
		},
		"splits": {"checked": 6, "mismatched": []},
		"shares": {"checked": 0, "mismatched": []},
		"transfers": {
			"paired": 1,
			"cross_currency": 0,
			"one_sided": [
				{"id":"xfer-%d","date":"2026-03-01","account":"Chequing","currency":"CAD","payee":null,"amount":"-1.00","other_account":null,"other_account_id":null},
				{"id":"xfer-%d","date":"2026-03-01","account":"Chequing","currency":"CAD","payee":"Landlord","amount":"-500.00","other_account":"Savings","other_account_id":"acct-%d"},
				{"id":"xfer-%d","date":"2026-03-01","account":"Savings","currency":"CAD","payee":null,"amount":"-1204.17","other_account":"Old Visa","other_account_id":null}
			]
		},
		"findings": {"open": 4, "ignored": 0, "fixed": 0, "new": 4, "newly_fixed": 0},
		"rates": {"first": "2026-01-02", "last": "2026-01-02", "added": 1, "fetch_error": null}
	}`, storePathUnder(home), chequingPK, savingsPK, missingLeg, namedLeg, savingsPK, noMatchLeg)
	assert.JSONEq(t, wantStore, string(parsed["store"]))
}

// Once the build was reached, a failed validation still writes the --json document
// with "built": false, instead of returning before anything is written.
func Test_run_prints_the_unbuilt_store_as_json_when_validation_fails(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Closed: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)

	balancedTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: balancedTxn, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.01"})

	oneSidedTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: oneSidedTxn, Amount: "-5.00", QuickenID: 3001, Transfer: "Old Visa"})

	splitDay := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	mismatchedTxn := b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "-212.40", PostedDate: &splitDay})
	b.Entry(v9fixture.EntryRow{Parent: mismatchedTxn, Amount: "-202.40"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"})

	assert.Equal(t, 1, exitCode)

	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	storePath := storePathUnder(home)

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	assert.JSONEq(t, "[]", string(parsed["warnings"]))

	var store map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(parsed["store"], &store))
	var built bool
	require.NoError(t, json.Unmarshal(store["built"], &built))
	assert.False(t, built)
	assert.JSONEq(t, "null", string(store["findings"]))
	var path string
	require.NoError(t, json.Unmarshal(store["path"], &path))
	assert.Equal(t, storePath, path)

	var balances map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(store["balances"], &balances))
	var balanceMismatched []map[string]any
	require.NoError(t, json.Unmarshal(balances["mismatched"], &balanceMismatched))
	require.Len(t, balanceMismatched, 1)
	assert.Equal(t, "2026-03-01", balanceMismatched[0]["statement_date"])
	assert.Equal(t, "100.00", balanceMismatched[0]["quarry"])
	assert.Equal(t, "100.01", balanceMismatched[0]["quicken"])
	assert.Equal(t, "-0.01", balanceMismatched[0]["difference"])
	assert.Equal(t, true, balanceMismatched[0]["closed"])
	assert.Equal(t, false, balanceMismatched[0]["active"])

	var splits map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(store["splits"], &splits))
	var splitMismatched []map[string]any
	require.NoError(t, json.Unmarshal(splits["mismatched"], &splitMismatched))
	require.Len(t, splitMismatched, 1)
	assert.Equal(t, "-212.40", splitMismatched[0]["amount"])
	assert.Equal(t, "-202.40", splitMismatched[0]["splits_total"])

	var transfers map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(store["transfers"], &transfers))
	var oneSided []map[string]any
	require.NoError(t, json.Unmarshal(transfers["one_sided"], &oneSided))
	assert.Len(t, oneSided, 1)

	assert.Equal(t,
		"quarry: validation failed: 1 of 1 account does not match Quicken's last reconciled balance and "+
			"1 transaction does not equal the sum of its splits; "+abbreviated(t, storePath, home)+
			" was not changed; each difference is listed on stdout; fix them in Quicken and run quarry sync, "+
			"or run quarry sync --from "+snapshotID(snapshotPath)+" after updating quarry\n",
		stderr.String())
}

// Once the build was reached, a stdout write failure still points at
// --from --json under --json, the same as it does on the human path.
func Test_run_points_at_from_json_when_stdout_fails_writing_a_failed_validation_as_json(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &day, EndingBalance: "100.01"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	writeErr := errNoSpace
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, failingWriter{err: writeErr}, &stderr)

	assert.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	assert.Equal(t,
		"quarry: cannot write the result to stdout: "+writeErr.Error()+"; run quarry sync --from "+
			snapshotID(snapshotPath)+" --json to see it again\n",
		stderr.String())
}

// Two never-reconciled accounts with opposite closed/active flags, so a
// hardcoded closed:false or active:true in the renderer cannot pass.
func Test_run_lists_never_reconciled_accounts_in_json_and_succeeds(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	closedPK := b.Account(v9fixture.AccountRow{Name: "Zulu Card", Type: "CREDITCARD", Currency: "CAD", Closed: true})
	inactivePK := b.Account(v9fixture.AccountRow{Name: "Alpha Wallet", Type: "SAVINGS", Currency: "CAD"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	var store map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(parsed["store"], &store))
	var balances map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(store["balances"], &balances))

	wantNeverReconciled := fmt.Sprintf(
		`[{"id":"acct-%d","name":"Alpha Wallet","currency":"CAD","closed":false,"active":false},`+
			`{"id":"acct-%d","name":"Zulu Card","currency":"CAD","closed":true,"active":false}]`,
		inactivePK, closedPK)
	assert.JSONEq(t, wantNeverReconciled, string(balances["never_reconciled"]))
}

// The reference count line and rows never change with the schema outcome:
// only the Schema line and its rows differ.
func Test_run_reports_a_schema_mismatch(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)

	want := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 1 account\n%-10s%s\n%-10s%s\n"+
			"  - table   %s\n  - column  %s.%s\n  - column  %s.%s\n  + column  %s.%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "DIFFERS from reference hardkoded/quicken-skills@752107b+quarry.1: "+
			"1 table and 2 columns missing, 1 column not in reference",
		v9fixture.MissingSchemaDroppedTable,
		v9fixture.MissingSchemaDroppedColumnTable, v9fixture.MissingSchemaDroppedColumn1,
		v9fixture.MissingSchemaDroppedColumnTable, v9fixture.MissingSchemaDroppedColumn2,
		v9fixture.MissingSchemaAddedColumnTable, v9fixture.MissingSchemaAddedColumn,
	)
	assert.Equal(t, want, stdout.String())

	assert.Equal(t, "quarry: schema check failed: Home.quicken is missing 1 table and 2 columns "+
		"that the schema reference expects; the snapshot is kept at "+abbreviated(t, snapshotPath, home)+
		" and the diff is in its .json manifest; quarry cannot import this file until its schema reference is updated\n",
		stderr.String())

	rawManifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(rawManifest, &manifest))
	assert.Equal(t, []any{}, manifest["warnings"])
	schema, ok := manifest["schema"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, schema["verified"])

	_, err = os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func Test_run_reports_extra_schema_only_as_a_warning(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.ExtraSchemaBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode)
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
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 1 account\n%-10s%s\n%-10s%s\n"+
			"  + table   %s\n  + column  %s.%s\n  + column  %s.%s\n"+
			"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns), "+
			"plus 1 table and 2 columns not in it",
		v9fixture.ExtraSchemaAddedTable,
		v9fixture.ExtraSchemaAddedColumnTable1, v9fixture.ExtraSchemaAddedColumn1,
		v9fixture.ExtraSchemaAddedColumnTable2, v9fixture.ExtraSchemaAddedColumn2,
		"Store", abbreviated(t, storePath, home),
		"Rows", "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices",
		"Balances", "no accounts to check; 1 never reconciled",
		"Splits", "no transactions to check",
		"Shares", "no holdings to check",
		"Transfers", "none",
		"Findings", "none open",
		"Rates", fakeRatesText,
	)
	assert.Equal(t, want, stdout.String())

	wantWarning := "quarry: warning: Home.quicken has 1 table and 2 columns that are not in the schema reference; " +
		"quarry ignores them (listed in " + filepath.Base(manifestPath) + ")\n"
	assert.Equal(t, wantWarning, stderr.String())

	rawManifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(rawManifest, &manifest))
	assert.Equal(t, []any{strings.TrimSuffix(strings.TrimPrefix(wantWarning, "quarry: warning: "), "\n")},
		manifest["warnings"])
}

func Test_run_reports_a_schema_mismatch_as_json(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"})

	require.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	manifestBytes, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	assert.JSONEq(t, string(manifest["snapshot"]), string(parsed["snapshot"]))
	assert.JSONEq(t, string(manifest["schema"]), string(parsed["schema"]))
	assert.JSONEq(t, string(manifest["warnings"]), string(parsed["warnings"]))

	storeValue, present := parsed["store"]
	require.True(t, present, "the store key must be present even when the import was not attempted")
	assert.JSONEq(t, "null", string(storeValue))

	var schema map[string]any
	require.NoError(t, json.Unmarshal(parsed["schema"], &schema))
	assert.Equal(t, false, schema["verified"])
	assert.NotEmpty(t, schema["missing_tables"])
	assert.NotEmpty(t, schema["missing_columns"])
	assert.NotEmpty(t, schema["unexpected_columns"])

	require.NotEmpty(t, stderr.String())
	assert.True(t, strings.HasPrefix(stderr.String(), "quarry: schema check failed:"))
}

// --from re-checks the same snapshot file against the same embedded
// reference MissingSchemaBundle already mismatches, independent of Quicken.
func Test_run_reports_a_schema_mismatch_with_from(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
	var syncStdout, syncStderr bytes.Buffer
	require.Equal(t, 1, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncStdout, &syncStderr))
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--from", id})

	require.Equal(t, 1, exitCode)
	assert.Equal(t, syncStdout.String(), stdout.String())
	assert.Equal(t, "quarry: schema check failed: snapshot "+id+" of Home.quicken is missing 1 table and 2 columns "+
		"that the schema reference expects; quarry cannot import it until its schema reference is updated\n",
		stderr.String())
	_, err := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func Test_run_reports_a_schema_mismatch_with_from_as_json(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
	var syncStdout, syncStderr bytes.Buffer
	require.Equal(t, 1, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncStdout, &syncStderr))
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--from", id, "--json"})

	require.Equal(t, 1, exitCode)
	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
	storeValue, present := parsed["store"]
	require.True(t, present, "the store key must be present even when the import was not attempted")
	assert.JSONEq(t, "null", string(storeValue))

	var schema map[string]any
	require.NoError(t, json.Unmarshal(parsed["schema"], &schema))
	assert.Equal(t, false, schema["verified"])
	assert.NotEmpty(t, schema["missing_tables"])
	assert.NotEmpty(t, schema["missing_columns"])
	assert.NotEmpty(t, schema["unexpected_columns"])

	require.NotEmpty(t, stderr.String())
	assert.True(t, strings.HasPrefix(stderr.String(), "quarry: schema check failed: snapshot "+id))
}

// On the mismatch path no build is reached, so a stdout write failure still
// names the manifest already on disk, not --from --json.
func Test_run_keeps_the_snapshot_message_when_writing_stdout_fails_on_a_schema_mismatch(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.MissingSchemaBundle(t, filepath.Join(home, "Documents"))
	writeErr := errNoSpace
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, failingWriter{err: writeErr}, &stderr)

	assert.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	onlyFileWithSuffix(t, snapshotsDir, ".json")
	assert.Equal(t,
		"quarry: cannot write the result to stdout: "+writeErr.Error()+"; the snapshot is kept at "+
			abbreviated(t, snapshotPath, home)+" and its .json manifest holds the full result\n",
		stderr.String())
}

// Chequing's stale and deleted-newer statements, and its non-reconciled
// transaction, must all be ignored for its balance to match.
func Test_run_checks_balances_and_split_sums_before_swapping_the_store_in(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	closedPK := b.Account(v9fixture.AccountRow{Name: "Closed Card", Type: "CREDITCARD", Currency: "CAD", Closed: true})
	walletPK := b.Account(v9fixture.AccountRow{Name: "Old Wallet", Type: "SAVINGS", Currency: "CAD"})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})

	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)

	reconciledTxnPK := b.Transaction(v9fixture.TransactionRow{
		Account: chequingPK, Amount: "100.00", PostedDate: &day, Status: &reconciled,
	})
	b.Entry(v9fixture.EntryRow{Parent: reconciledTxnPK, Amount: "100.00"})
	unclearedTxnPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "50.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: unclearedTxnPK, Amount: "50.00"})

	jan := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &jan, EndingBalance: "999.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &mar, EndingBalance: "1.00", Deleted: true})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &feb, EndingBalance: "100.00"})

	closedTxnPK := b.Transaction(v9fixture.TransactionRow{
		Account: closedPK, Amount: "25.00", PostedDate: &day, Status: &reconciled,
	})
	b.Entry(v9fixture.EntryRow{Parent: closedTxnPK, Amount: "25.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: closedPK, EndDate: &day, EndingBalance: "25.00"})

	walletTxnPK := b.Transaction(v9fixture.TransactionRow{
		Account: walletPK, Amount: "10.00", PostedDate: &day, Status: &reconciled,
	})
	b.Entry(v9fixture.EntryRow{Parent: walletTxnPK, Amount: "10.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: walletPK, EndDate: &day, EndingBalance: "10.00"})

	_ = savingsPK
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	writeConfig(t, home, fmt.Sprintf("[accounts]\nnon-registered = [\"acct-%d\"]\n", brokeragePK))

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
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 5 accounts\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)",
		"Store", abbreviated(t, storePath, home),
		"Rows", "4 transactions, 4 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices",
		"Balances", "3 accounts match Quicken's last reconciled balance; 1 never reconciled and 1 investment account's cash not checked",
		"Splits", "all 4 transactions equal the sum of their splits",
		"Shares", "no holdings to check",
		"Transfers", "none",
		"Findings", "1 open; run quarry findings to list them",
		"Rates", fakeRatesText,
	)
	require.Equal(t, want, stdout.String())

	_, err = os.Stat(storePath)
	require.NoError(t, err)
}

// maxLen returns the length of the longest of ss.
func maxLen(ss ...string) int {
	n := 0
	for _, s := range ss {
		n = max(n, len(s))
	}
	return n
}

// mismatchRow renders one "!" row the same way the failed-validation block does.
func mismatchRow(labelWidth, quarryWidth, quickenWidth, diffWidth int, label, date, quarry, quicken, diff string) string {
	return fmt.Sprintf("  ! %-*s%s  quarry %*s  Quicken %*s  difference %*s",
		labelWidth, label, date, quarryWidth, quarry, quickenWidth, quicken, diffWidth, diff)
}

// A mismatch on a first run (no previous store) keeps the snapshot and
// leaves no store, with the full failed-validation block on stdout.
func Test_run_refuses_a_balance_mismatch_and_leaves_no_store(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	matchingPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &day, EndingBalance: "100.01"})
	matchingTxnPK := b.Transaction(v9fixture.TransactionRow{Account: matchingPK, Amount: "10.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: matchingTxnPK, Amount: "10.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: matchingPK, EndDate: &day, EndingBalance: "10.00"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	assert.Equal(t, 1, exitCode)

	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")

	label := "Chequing (CAD)"
	row := mismatchRow(len(label)+2, len("100.00"), len("100.01"), len("-0.01"), label, "2026-03-01", "100.00", "100.01", "-0.01")
	wantStdout := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 2 accounts\n%-10s%s\n%-10s%s\n"+
			"%-10s%s\n%-10s%s\n%-10s%s\n%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)",
		"Store", "NOT BUILT (no store at "+abbreviated(t, storePath, home)+" yet)",
		"Rows", "2 transactions, 2 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices",
		"Balances", "DIFFER for 1 of 2 accounts",
		row,
		"Splits", "all 2 transactions equal the sum of their splits",
		"Shares", "no holdings to check",
		"Transfers", "none",
	)
	assert.Equal(t, wantStdout, stdout.String())

	assert.Equal(t,
		"quarry: validation failed: 1 of 2 accounts does not match Quicken's last reconciled balance; "+
			abbreviated(t, storePath, home)+" was not changed; each difference is listed on stdout; "+
			"fix them in Quicken and run quarry sync, or run quarry sync --from "+
			snapshotID(snapshotPath)+" after updating quarry\n",
		stderr.String())

	_, err = os.Stat(storePath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// Once the build was reached, even though it failed, a stdout write
// failure points at --from --json, not at the snapshot's manifest.
func Test_run_points_at_from_json_when_stdout_fails_rendering_a_failed_validation(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &day, EndingBalance: "100.01"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	writeErr := errNoSpace
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, failingWriter{err: writeErr}, &stderr)

	assert.Equal(t, 1, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	assert.Equal(t,
		"quarry: cannot write the result to stdout: "+writeErr.Error()+"; run quarry sync --from "+
			snapshotID(snapshotPath)+" --json to see it again\n",
		stderr.String())
}

// A failing sync leaves an existing store byte-identical; the failed-validation block
// lists every mismatched balance in account-name then source-id order.
func Test_run_leaves_the_previous_store_byte_identical_after_a_failing_sync(t *testing.T) {
	home := newHome(t)

	storeDir := filepath.Join(home, "Library", "Application Support", "quarry")
	require.NoError(t, os.MkdirAll(storeDir, 0o700))
	storePath := filepath.Join(storeDir, "quarry.duckdb")
	sentinel := []byte("previous store bytes, untouched by a failing sync")
	require.NoError(t, os.WriteFile(storePath, sentinel, 0o600))

	b := v9fixture.NewBuilder()
	usPK := b.Account(v9fixture.AccountRow{Name: "US Chequing", Type: "CHECKING", Currency: "USD", Active: true})
	visaPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Closed: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD"})
	reconciled := int64(2)

	usDay := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	usTxnPK := b.Transaction(v9fixture.TransactionRow{Account: usPK, Amount: "8310.00", PostedDate: &usDay, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: usTxnPK, Amount: "8310.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: usPK, EndDate: &usDay, EndingBalance: "8300.00"})

	visaDay := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	visaTxnPK := b.Transaction(v9fixture.TransactionRow{Account: visaPK, Amount: "-1204.17", PostedDate: &visaDay, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: visaTxnPK, Amount: "-1204.17"})
	b.Reconcile(v9fixture.ReconcileRow{Account: visaPK, EndDate: &visaDay, EndingBalance: "-1184.17"})

	savingsDay := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	savingsTxnPK := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "50.00", PostedDate: &savingsDay, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: savingsTxnPK, Amount: "50.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: savingsPK, EndDate: &savingsDay, EndingBalance: "60.00"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	assert.Equal(t, 1, exitCode)

	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)

	labelWidth := maxLen("US Chequing (USD)", "Visa Infinite (CAD, closed)", "Savings (CAD, inactive)") + 2
	quarryWidth := maxLen("8,310.00", "-1,204.17", "50.00")
	quickenWidth := maxLen("8,300.00", "-1,184.17", "60.00")
	diffWidth := maxLen("10.00", "-20.00", "-10.00")

	// Sorted by account name (byte order): Savings, US Chequing, Visa Infinite.
	wantStdout := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 3 accounts\n%-10s%s\n%-10s%s\n"+
			"%-10s%s\n%-10s%s\n%-10s%s\n%s\n%s\n%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)",
		"Store", "NOT REBUILT ("+abbreviated(t, storePath, home)+" unchanged)",
		"Rows", "3 transactions, 3 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices",
		"Balances", "DIFFER for 3 of 3 accounts",
		mismatchRow(labelWidth, quarryWidth, quickenWidth, diffWidth,
			"Savings (CAD, inactive)", "2026-01-15", "50.00", "60.00", "-10.00"),
		mismatchRow(labelWidth, quarryWidth, quickenWidth, diffWidth,
			"US Chequing (USD)", "2026-08-31", "8,310.00", "8,300.00", "10.00"),
		mismatchRow(labelWidth, quarryWidth, quickenWidth, diffWidth,
			"Visa Infinite (CAD, closed)", "2026-07-15", "-1,204.17", "-1,184.17", "-20.00"),
		"Splits", "all 3 transactions equal the sum of their splits",
		"Shares", "no holdings to check",
		"Transfers", "none",
	)
	assert.Equal(t, wantStdout, stdout.String())

	assert.Equal(t,
		"quarry: validation failed: 3 of 3 accounts do not match Quicken's last reconciled balance; "+
			abbreviated(t, storePath, home)+" was not changed; each difference is listed on stdout; "+
			"fix them in Quicken and run quarry sync, or run quarry sync --from "+
			snapshotID(snapshotPath)+" after updating quarry\n",
		stderr.String())

	got, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.Equal(t, sentinel, got)
}

// A transaction whose splits don't sum to its amount lists in the
// failed-validation block; no payee falls back to "(no payee)".
func Test_run_lists_mismatched_splits_in_the_failed_validation_stdout_block(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	reconciled := int64(2)

	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	matchingTxnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: matchingTxnPK, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &day, EndingBalance: "100.00"})

	splitDay := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	mismatchedTxnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "-212.40", PostedDate: &splitDay})
	b.Entry(v9fixture.EntryRow{Parent: mismatchedTxnPK, Amount: "-202.40"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	assert.Equal(t, 1, exitCode)

	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")

	wantStdout := fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 1 account\n%-10s%s\n%-10s%s\n"+
			"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%s\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)",
		"Store", "NOT BUILT (no store at "+abbreviated(t, storePath, home)+" yet)",
		"Rows", "2 transactions, 2 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices",
		"Balances", "1 account matches Quicken's last reconciled balance",
		"Splits", "DIFFER for 1 of 2 transactions",
		"  ! 2024-03-02  Visa Infinite (CAD)  (no payee)  amount -212.40  splits -202.40",
		"Shares", "no holdings to check",
		"Transfers", "none",
	)
	assert.Equal(t, wantStdout, stdout.String())
	assert.Equal(t,
		"quarry: validation failed: 1 transaction does not equal the sum of its splits; "+
			abbreviated(t, storePath, home)+" was not changed; each difference is listed on stdout; "+
			"fix them in Quicken and run quarry sync, or run quarry sync --from "+
			snapshotID(snapshotPath)+" after updating quarry\n",
		stderr.String())

	_, err = os.Stat(storePath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// The bundle itself is valid; its data file is present but not a SQLite
// database at all, so this exercises srv.Sync's error path, not path resolution.
func Test_run_refuses_an_encrypted_bundle(t *testing.T) {
	home := newHome(t)
	bundleDir := filepath.Join(home, "Documents", "Home.quicken")
	require.NoError(t, os.MkdirAll(bundleDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), []byte("not a database"), 0o600))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundleDir})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: "+abbreviated(t, bundleDir, home)+
		" is encrypted, so Quicken does not have it open; open it in Quicken, then run quarry sync again\n",
		stderr.String())
	_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// A WAL-formatted bundle with no live -wal file must be refused before Sync
// opens it — that open alone would create -wal/-shm this test checks for.
func Test_run_refuses_a_bundle_that_is_not_open_in_quicken(t *testing.T) {
	home := newHome(t)
	bundle := v9fixture.ClosedWALBundle(t, filepath.Join(home, "Documents"))
	before, err := os.ReadDir(bundle.Dir)
	require.NoError(t, err)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: "+abbreviated(t, bundle.Dir, home)+
		" is not open in Quicken (its database has no write-ahead log); open it in Quicken, then run quarry sync again\n",
		stderr.String())
	after, err := os.ReadDir(bundle.Dir)
	require.NoError(t, err)
	assert.Equal(t, entryNames(before), entryNames(after))
	_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// entryNames returns entries' names in order.
func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// Prepare's MkdirAll is a no-op on an already-existing directory regardless
// of its permission bits, so the snapshots directory must exist before the
// chmod, or the failure this test wants would never surface.
func Test_run_refuses_a_snapshots_directory_that_is_not_writable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	home := newHome(t)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	require.NoError(t, os.MkdirAll(snapshotsDir, 0o700))
	t.Cleanup(func() { _ = os.Chmod(snapshotsDir, 0o700) })
	require.NoError(t, os.Chmod(snapshotsDir, 0o500))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot write to "+abbreviated(t, snapshotsDir, home)+
		": permission denied; make the directory writable by your user\n",
		stderr.String())
	entries, err := os.ReadDir(snapshotsDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// The snapshots directory is left behind, empty, in every case: Prepare
// already ran before the content check can fail.
func Test_run_refuses_a_bundle_whose_snapshot_content_is_rejected(t *testing.T) {
	cases := []struct {
		name        string
		buildBundle func(t *testing.T, home string) string
		wantLine    func(t *testing.T, bundleDir, home string) string
	}{
		{
			name: "damaged so only integrity_check fails",
			buildBundle: func(t *testing.T, home string) string {
				t.Helper()
				bundleDir := filepath.Join(home, "Documents", "Home.quicken")
				require.NoError(t, os.MkdirAll(bundleDir, 0o700))
				v9fixture.CorruptDataFile(t, filepath.Join(bundleDir, "data"))
				return bundleDir
			},
			wantLine: func(t *testing.T, bundleDir, home string) string {
				t.Helper()
				last := integrityCheckLastLine(t, filepath.Join(bundleDir, "data"))
				return "quarry: the snapshot of " + abbreviated(t, bundleDir, home) +
					" failed SQLite's integrity check (" + last +
					"); nothing was kept; quit and reopen the file in Quicken, then run quarry sync again"
			},
		},
		{
			name: "missing ZACCOUNT (including a 0-byte data file)",
			buildBundle: func(t *testing.T, home string) string {
				t.Helper()
				bundleDir := filepath.Join(home, "Documents", "Home.quicken")
				require.NoError(t, os.MkdirAll(bundleDir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "data"), nil, 0o600))
				return bundleDir
			},
			wantLine: func(t *testing.T, bundleDir, home string) string {
				t.Helper()
				return "quarry: " + abbreviated(t, bundleDir, home) +
					" is not a Quicken Classic for Mac database (no ZACCOUNT table); pass the right file with --quicken <path>"
			},
		},
		{
			name: "ZACCOUNT with no rows",
			buildBundle: func(t *testing.T, home string) string {
				t.Helper()
				bundle := v9fixture.EmptyAccountsBundle(t, filepath.Join(home, "Documents"))
				return bundle.Dir
			},
			wantLine: func(t *testing.T, bundleDir, home string) string {
				t.Helper()
				return "quarry: " + abbreviated(t, bundleDir, home) +
					" has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>"
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			bundleDir := c.buildBundle(t, home)

			exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundleDir})

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
			require.Len(t, lines, 1)
			assert.Equal(t, c.wantLine(t, bundleDir, home), lines[0])
			snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
			entries, err := os.ReadDir(snapshotsDir)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

// integrityCheckLastLine reads PRAGMA integrity_check's first row's last
// physical line through a connection independent of the code under test.
func integrityCheckLastLine(t *testing.T, path string) string {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var row string
	require.NoError(t, conn.QueryRowContext(t.Context(), "PRAGMA integrity_check").Scan(&row))
	lines := strings.Split(row, "\n")
	return lines[len(lines)-1]
}
