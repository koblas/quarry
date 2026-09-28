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
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Chequing's stale and deleted-newer statements, and its non-reconciled
// transaction, must all be ignored for its balance to match.
func Test_run_checks_balances_and_split_sums_before_swapping_the_store_in(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	closedPK := b.Account(v9fixture.AccountRow{Name: "Closed Card", Type: "CREDITCARD", Currency: "CAD", Closed: true})
	walletPK := b.Account(v9fixture.AccountRow{Name: "Old Wallet", Type: "SAVINGS", Currency: "CAD"})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})

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
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s, 5 accounts\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Snapshot", abbreviated(t, snapshotPath, home),
		"Manifest", abbreviated(t, manifestPath, home),
		"Source", abbreviated(t, bundle.Dir, home),
		"Size", megabytes(info.Size()),
		"SHA-256", hex.EncodeToString(sum[:]),
		"Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)",
		"Store", abbreviated(t, storePath, home),
		"Rows", "4 transactions, 4 splits, 0 transfers, 0 payees, 0 categories, 0 tags",
		"Balances", "3 accounts match Quicken's last reconciled balance; 1 never reconciled and 1 investment account not checked",
		"Splits", "all 4 transactions equal the sum of their splits",
	)
	require.Equal(t, want, stdout.String())

	_, err = os.Stat(storePath)
	require.NoError(t, err)
}

// Interim V1 (no stdout block yet): a mismatch on a first run keeps the
// snapshot but never creates quarry.duckdb.
func Test_run_refuses_a_balance_mismatch_and_leaves_no_store(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(),
		"quarry: validation failed: 1 of 2 accounts does not match Quicken's last reconciled balance")
	assert.Contains(t, stderr.String(), "was not changed; fix the account in Quicken and run quarry sync")

	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	onlyFileWithSuffix(t, snapshotsDir, ".json")

	storePath := filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")
	_, err := os.Stat(storePath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
