package importer_test

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// execOn runs query against the SQLite file at dataPath, for rows v9fixture.Builder cannot express.
func execOn(t *testing.T, dataPath, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite3", dataPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
}

// investmentWithEntry adds an investment transaction with the one entry Quicken writes for it, in row's amount,
// and returns its Z_PK.
func investmentWithEntry(b *v9fixture.Builder, row v9fixture.TransactionRow) int64 {
	pk := b.InvestmentTransaction(row)
	b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount})
	return pk
}

func payeeIDs(fake *fakeStore) []string {
	ids := make([]string, len(fake.Rows.Payees))
	for i, p := range fake.Rows.Payees {
		ids[i] = p.ID
	}
	return ids
}

func tagIDs(fake *fakeStore) []string {
	ids := make([]string, len(fake.Rows.Tags))
	for i, tg := range fake.Rows.Tags {
		ids[i] = tg.ID
	}
	return ids
}

func transactionIDs(fake *fakeStore) []string {
	ids := make([]string, len(fake.Rows.Transactions))
	for i, txn := range fake.Rows.Transactions {
		ids[i] = txn.ID
	}
	return ids
}

// fakeStore captures the rows the last Replace call received and counts
// every call, standing in for a real Store so importer tests never link
// DuckDB.
type fakeStore struct {
	Rows          store.Rows
	Path          string
	historyFault  *store.OpenError
	findings      finding.Counts
	states        []finding.State
	carried       bool
	findingsFault *store.OpenError
	unreadable    bool
	rates         store.RatesSummary
	ratesFault    *store.OpenError
	nextErr       error
	replaceCalls  int
	shareCheck    store.ShareCheck
	shareErr      error
}

func (f *fakeStore) failNext(err error) { f.nextErr = err }

// CheckShares answers the configured shareCheck (default: nothing checked, no mismatch).
func (f *fakeStore) CheckShares(context.Context, store.Rows) (store.ShareCheck, error) {
	return f.shareCheck, f.shareErr
}

func (f *fakeStore) Replace(_ context.Context, rows store.Rows) (store.Replaced, error) {
	f.replaceCalls++
	if f.nextErr != nil {
		return store.Replaced{}, f.nextErr
	}
	f.Rows = rows
	return store.Replaced{
		Path: f.Path, HistoryFault: f.historyFault, Findings: f.findings, FindingStates: f.states, FindingsCarried: f.carried,
		FindingsFault: f.findingsFault, StoreUnreadable: f.unreadable, Rates: f.rates,
		RatesFault: f.ratesFault,
	}, nil
}

func importReason(t *testing.T, err error) string {
	t.Helper()
	var unmappable *importer.UnmappableError
	require.ErrorAs(t, err, &unmappable, "expected *importer.UnmappableError, got %v", err)
	return unmappable.Reason
}

var (
	investDay    = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	investLater  = time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	buyCode      = new(int64(3))
	dividendCode = new(int64(10))
)

// newBrokerage adds the account every investment test imports into.
func newBrokerage(b *v9fixture.Builder) int64 {
	return b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
}

func importInvestments(t *testing.T, b *v9fixture.Builder) (*fakeStore, store.Result) {
	t.Helper()
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	return fake, result
}

func importInvestmentsRefused(t *testing.T, b *v9fixture.Builder) (string, *fakeStore) {
	t.Helper()
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	return importReason(t, err), fake
}

var (
	priceDay1 = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	priceDay2 = time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
)

func newAcme(b *v9fixture.Builder) int64 {
	return b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
}

func importSecurities(t *testing.T, b *v9fixture.Builder) (*fakeStore, store.Result) {
	t.Helper()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	return fake, result
}

// setColumnBlob overwrites column on the row identified by pk with value as
// a real SQLite BLOB, for fixture shapes v9fixture.Builder has no field for.
func setColumnBlob(t *testing.T, dataPath, table, column string, pk int64, value []byte) {
	t.Helper()
	db, err := sql.Open("sqlite3", dataPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(t.Context(), "UPDATE "+table+" SET "+column+" = ? WHERE Z_PK = ?", value, pk) //nolint:gosec // table and column are test constants
	require.NoError(t, err)
}

// setColumnEmptyText overwrites column with a zero-length SQLite TEXT
// value — distinct from setColumnBlob's zero-length BLOB.
func setColumnEmptyText(t *testing.T, dataPath, table, column string, pk int64) {
	t.Helper()
	db, err := sql.Open("sqlite3", dataPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(t.Context(), "UPDATE "+table+" SET "+column+" = '' WHERE Z_PK = ?", pk) //nolint:gosec // table and column are test constants
	require.NoError(t, err)
}

func chequingWithOneReconciledTxn(b *v9fixture.Builder, cents string) int64 {
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: cents, PostedDate: &posted, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: cents})
	return acctPK
}

// transferLeg adds a one-entry transaction in account whose entry carries
// quickenID and the ZTRANSFER text link, returning the entry's Z_PK.
func transferLeg(b *v9fixture.Builder, account int64, amount string, quickenID int64, link string) int64 {
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &day})
	return b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, QuickenID: quickenID, Transfer: link})
}

func splitIDFor(pk int64) string { return fmt.Sprintf("split-%d", pk) }

func accountIDFor(pk int64) string { return fmt.Sprintf("acct-%d", pk) }

func splitByID(fake *fakeStore, id string) store.Split {
	for _, s := range fake.Rows.Splits {
		if s.ID == id {
			return s
		}
	}
	return store.Split{}
}
