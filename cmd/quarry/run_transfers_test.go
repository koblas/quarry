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
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBlock renders the expected sync stdout: the Phase 0 block for the one
// snapshot under home, then each label/value pair as its own line.
func syncBlock(t *testing.T, home, bundleDir string, accounts int, lines ...[2]string) string {
	t.Helper()
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)

	var b strings.Builder
	fmt.Fprintf(&b, "%-10s%s\n", "Snapshot", abbreviated(t, snapshotPath, home))
	fmt.Fprintf(&b, "%-10s%s\n", "Manifest", abbreviated(t, manifestPath, home))
	fmt.Fprintf(&b, "%-10s%s\n", "Source", abbreviated(t, bundleDir, home))
	fmt.Fprintf(&b, "%-10s%s, %d accounts\n", "Size", megabytes(info.Size()), accounts)
	fmt.Fprintf(&b, "%-10s%s\n", "SHA-256", hex.EncodeToString(sum[:]))
	fmt.Fprintf(&b, "%-10s%s\n", "Schema", "matches reference hardkoded/quicken-skills@752107b+quarry.1 (82 tables, 1,838 columns)")
	for _, line := range lines {
		fmt.Fprintf(&b, "%-10s%s\n", line[0], line[1])
	}
	return b.String()
}

func storePathUnder(home string) string {
	return filepath.Join(home, "Library", "Application Support", "quarry", "quarry.duckdb")
}

func Test_run_pairs_transfers_between_the_users_accounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	cardPK := b.Account(v9fixture.AccountRow{Name: "Visa", Type: "CREDITCARD", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	toSavingsTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-100.00", PostedDate: &day})
	toSavingsLeg := b.Entry(v9fixture.EntryRow{Parent: toSavingsTxn, Amount: "-100.00", QuickenID: 1001, Transfer: "2002"})
	fromChequingTxn := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "100.00", PostedDate: &day})
	fromChequingLeg := b.Entry(v9fixture.EntryRow{Parent: fromChequingTxn, Amount: "100.00", QuickenID: 2002, Transfer: "1001"})
	toCardTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-50.00", PostedDate: &day})
	toCardLeg := b.Entry(v9fixture.EntryRow{Parent: toCardTxn, Amount: "-50.00", QuickenID: 3003, Transfer: "4004"})
	paymentTxn := b.Transaction(v9fixture.TransactionRow{Account: cardPK, Amount: "50.00", PostedDate: &day})
	paymentLeg := b.Entry(v9fixture.EntryRow{Parent: paymentTxn, Amount: "50.00", QuickenID: 4004, Transfer: "3003"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	storePath := storePathUnder(home)
	require.Equal(t, syncBlock(t, home, bundle.Dir, 3,
		[2]string{"Store", abbreviated(t, storePath, home)},
		[2]string{"Rows", "4 transactions, 4 splits, 2 transfers, 0 payees, 0 categories, 0 tags"},
		[2]string{"Balances", "no accounts to check; 3 never reconciled"},
		[2]string{"Splits", "all 4 transactions equal the sum of their splits"},
		[2]string{"Transfers", "2 paired"},
	), stdout.String())

	db, err := duckdb.OpenReadOnly(t.Context(), storePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	splitID := func(pk int64) string { return fmt.Sprintf("split-%d", pk) }
	xferID := func(pk int64) string { return fmt.Sprintf("xfer-%d", pk) }
	acctID := func(pk int64) string { return fmt.Sprintf("acct-%d", pk) }

	assert.Equal(t, map[string]string{
		xferID(toSavingsLeg): splitID(toSavingsLeg), xferID(toCardLeg): splitID(toCardLeg),
	}, stringMap(t, db, "SELECT id, from_split_id FROM transfers"))
	assert.Equal(t, map[string]string{
		xferID(toSavingsLeg): splitID(fromChequingLeg), xferID(toCardLeg): splitID(paymentLeg),
	}, stringMap(t, db, "SELECT id, COALESCE(to_split_id, 'NULL') FROM transfers"))
	assert.Equal(t, map[string]string{
		xferID(toSavingsLeg): "false", xferID(toCardLeg): "false",
	}, stringMap(t, db, "SELECT id, CAST(cross_currency AS VARCHAR) FROM transfers"))
	assert.Equal(t, map[string]string{
		splitID(toSavingsLeg): acctID(savingsPK), splitID(fromChequingLeg): acctID(chequingPK),
		splitID(toCardLeg): acctID(cardPK), splitID(paymentLeg): acctID(chequingPK),
	}, stringMap(t, db, "SELECT id, COALESCE(transfer_account_id, 'NULL') FROM splits"))
}

func Test_run_reports_no_transfers_for_a_file_with_no_transactions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	b.Category(v9fixture.TagRow{Name: "Food", Type: v9fixture.Int64Ptr(1)})
	b.Category(v9fixture.TagRow{Name: "Salary", Type: v9fixture.Int64Ptr(2)})
	b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	assert.Equal(t, syncBlock(t, home, bundle.Dir, 2,
		[2]string{"Store", abbreviated(t, storePathUnder(home), home)},
		[2]string{"Rows", "0 transactions, 0 splits, 0 transfers, 1 payee, 2 categories, 1 tag"},
		[2]string{"Balances", "no accounts to check; 2 never reconciled"},
		[2]string{"Splits", "no transactions to check"},
		[2]string{"Transfers", "none"},
	), stdout.String())
}

func Test_run_counts_investment_transactions_without_importing_them(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	buyTxn := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntInvestmentTransaction, Account: brokeragePK, Amount: "-400.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: buyTxn, Amount: "-400.00"})
	dividendTxn := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntInvestmentTransaction, Account: brokeragePK, Amount: "12.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: dividendTxn, Amount: "12.00"})
	contributionTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-1000.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: contributionTxn, Amount: "-1000.00", QuickenID: 5005, Transfer: "6006"})
	depositTxn := b.Transaction(v9fixture.TransactionRow{Account: brokeragePK, Amount: "1000.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: depositTxn, Amount: "1000.00", QuickenID: 6006, Transfer: "5005"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	storePath := storePathUnder(home)
	require.Equal(t, syncBlock(t, home, bundle.Dir, 2,
		[2]string{"Store", abbreviated(t, storePath, home)},
		[2]string{"Rows", "2 transactions, 2 splits, 1 transfer, 0 payees, 0 categories, 0 tags; 2 investment transactions not imported"},
		[2]string{"Balances", "no accounts to check; 1 never reconciled and 1 investment account not checked"},
		[2]string{"Splits", "all 2 transactions equal the sum of their splits"},
		[2]string{"Transfers", "1 paired"},
	), stdout.String())

	db, err := duckdb.OpenReadOnly(t.Context(), storePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", contributionTxn): fmt.Sprintf("acct-%d", chequingPK),
		fmt.Sprintf("txn-%d", depositTxn):      fmt.Sprintf("acct-%d", brokeragePK),
	}, stringMap(t, db, "SELECT id, account_id FROM transactions"))
}
