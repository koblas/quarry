// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errNoSpace stands in for a stdout write failing on a full disk.
var errNoSpace = errors.New("no space left on device")

// One assert.JSONEq against a full literal catches a wrong type, a missing
// key, or a leaked display-only field (source_id, closed, active) at once.
func Test_run_reports_the_store_result_alongside_the_manifest_as_json(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &stdout, &stderr)

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
		"rows": {"accounts":3,"categories":0,"payees":1,"tags":0,"transactions":6,"splits":6,"split_tags":0,"transfers":4},
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
		"not_imported": {"investment_transactions": 0}
	}`, storePathUnder(home), chequingPK, savingsPK, missingLeg, namedLeg, savingsPK, noMatchLeg)
	assert.JSONEq(t, wantStore, string(parsed["store"]))
}

// Once the build was reached, a failed validation still writes the --json document
// with "built": false, instead of returning before anything is written.
func Test_run_prints_the_unbuilt_store_as_json_when_validation_fails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &stdout, &stderr)

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
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	closedPK := b.Account(v9fixture.AccountRow{Name: "Zulu Card", Type: "CREDITCARD", Currency: "CAD", Closed: true})
	inactivePK := b.Account(v9fixture.AccountRow{Name: "Alpha Wallet", Type: "SAVINGS", Currency: "CAD"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir, "--json"}, &stdout, &stderr)

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
