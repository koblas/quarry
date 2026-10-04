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
		[2]string{"Findings", "none open"},
		[2]string{"Rates", fakeRatesText},
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
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	salaryPK := b.Category(v9fixture.TagRow{Name: "Salary", Type: new(int64(2))})
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: foodPK})
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: salaryPK})
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
		[2]string{"Findings", "none open"},
		[2]string{"Rates", fakeRatesText},
	), stdout.String())
}

func Test_run_counts_investment_transactions_without_importing_them(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	buyTxn := b.InvestmentTransaction(v9fixture.TransactionRow{
		Type: new(int64(3)), Account: brokeragePK, Amount: "-400.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: buyTxn, Amount: "-400.00"})
	dividendTxn := b.InvestmentTransaction(v9fixture.TransactionRow{
		Type: new(int64(10)), Account: brokeragePK, Amount: "12.00", PostedDate: &day,
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
		[2]string{"Findings", "none open"},
		[2]string{"Rates", fakeRatesText},
	), stdout.String())

	db, err := duckdb.OpenReadOnly(t.Context(), storePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{
		fmt.Sprintf("txn-%d", contributionTxn): fmt.Sprintf("acct-%d", chequingPK),
		fmt.Sprintf("txn-%d", depositTxn):      fmt.Sprintf("acct-%d", brokeragePK),
	}, stringMap(t, db, "SELECT id, account_id FROM transactions"))
}

func Test_run_lists_one_sided_transfers_only_as_findings_on_a_successful_sync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	landlordPK := b.Payee(v9fixture.PayeeRow{Name: "Landlord"})
	day1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)

	outTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-100.00", PostedDate: &day1})
	b.Entry(v9fixture.EntryRow{Parent: outTxn, Amount: "-100.00", QuickenID: 1001, Transfer: "2002"})
	inTxn := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "100.00", PostedDate: &day1})
	b.Entry(v9fixture.EntryRow{Parent: inTxn, Amount: "100.00", QuickenID: 2002, Transfer: "1001"})
	noMatchTxn := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "-1204.17", PostedDate: &day3})
	noMatchLeg := b.Entry(v9fixture.EntryRow{Parent: noMatchTxn, Amount: "-1204.17", QuickenID: 3003, Transfer: "Old Visa"})
	namedTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-500.00", PostedDate: &day2, Payee: landlordPK})
	namedLeg := b.Entry(v9fixture.EntryRow{Parent: namedTxn, Amount: "-500.00", QuickenID: 3002, Transfer: "Savings"})
	missingTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-1.00", PostedDate: &day1})
	missingLeg := b.Entry(v9fixture.EntryRow{Parent: missingTxn, Amount: "-1.00", QuickenID: 3001, Transfer: "999"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	storePath := storePathUnder(home)
	assert.Equal(t, syncBlock(t, home, bundle.Dir, 2,
		[2]string{"Store", abbreviated(t, storePath, home)},
		[2]string{"Rows", "5 transactions, 5 splits, 4 transfers, 1 payee, 0 categories, 0 tags"},
		[2]string{"Balances", "no accounts to check; 2 never reconciled"},
		[2]string{"Splits", "all 5 transactions equal the sum of their splits"},
		[2]string{"Transfers", "1 paired, 3 one-sided"},
		[2]string{"Findings", "3 open; run quarry findings to list them"},
		[2]string{"Rates", fakeRatesText},
	), stdout.String())
	assert.Empty(t, stderr.String())

	db, err := duckdb.OpenReadOnly(t.Context(), storePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{
		fmt.Sprintf("xfer-%d", missingLeg): fmt.Sprintf("split-%d", missingLeg),
		fmt.Sprintf("xfer-%d", namedLeg):   fmt.Sprintf("split-%d", namedLeg),
		fmt.Sprintf("xfer-%d", noMatchLeg): fmt.Sprintf("split-%d", noMatchLeg),
	}, stringMap(t, db, "SELECT id, from_split_id FROM transfers WHERE to_split_id IS NULL"))
}

func Test_run_lists_one_sided_transfers_without_warning_when_validation_fails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	mismatchedTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-10.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: mismatchedTxn, Amount: "-9.00"})
	legTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: legTxn, Amount: "-5.00", QuickenID: 3001, Transfer: "Old Visa"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 1, exitCode)
	storePath := storePathUnder(home)
	assert.Equal(t, syncBlock(t, home, bundle.Dir, 2,
		[2]string{"Store", "NOT BUILT (no store at " + abbreviated(t, storePath, home) + " yet)"},
		[2]string{"Rows", "2 transactions, 2 splits, 1 transfer, 0 payees, 0 categories, 0 tags"},
		[2]string{"Balances", "no accounts to check; 2 never reconciled"},
		[2]string{"Splits", "DIFFER for 1 of 2 transactions"},
	)+
		"  ! 2026-03-01  Chequing (CAD)  (no payee)  amount -10.00  splits -9.00\n"+
		"Transfers 0 paired, 1 one-sided\n"+
		"  ? 2026-03-01  Chequing (CAD)  (no payee)  -5.00  other account: Old Visa (not in this file)\n",
		stdout.String())
	assert.Equal(t, "quarry: validation failed: 1 transaction does not equal the sum of its splits; "+
		abbreviated(t, storePath, home)+" was not changed; each difference is listed on stdout; "+
		"fix them in Quicken and run quarry sync, or run quarry sync --from "+
		snapshotID(onlyFileWithSuffix(t, filepath.Join(filepath.Dir(storePath), "snapshots"), ".sqlite"))+" after updating quarry\n",
		stderr.String())
}
