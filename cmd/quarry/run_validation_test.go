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
