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

// importInto imports the snapshot at dataPath into fake.
func importInto(tb testing.TB, fake *fakeStore, dataPath string) (store.Result, error) {
	tb.Helper()
	return importer.NewServer(importer.WithStore(fake)).Import(tb.Context(), store.SnapshotRef{Path: dataPath})
}

// importBuilt writes b to a fresh bundle and imports it into fake.
func importBuilt(tb testing.TB, fake *fakeStore, b *v9fixture.Builder) (store.Result, error) {
	tb.Helper()
	return importInto(tb, fake, snapshotPath(tb, b))
}

// snapshotPath writes b to a fresh bundle and returns its data file's path.
func snapshotPath(tb testing.TB, b *v9fixture.Builder) string {
	tb.Helper()
	return b.WriteBundle(tb, tb.TempDir()).DataPath
}

// importOK imports b into a fresh store and requires the import to succeed.
func importOK(tb testing.TB, b *v9fixture.Builder) (*fakeStore, store.Result) {
	tb.Helper()
	return importOKFrom(tb, snapshotPath(tb, b))
}

// importOKFrom imports the snapshot at dataPath into a fresh store and requires the import to succeed.
func importOKFrom(tb testing.TB, dataPath string) (*fakeStore, store.Result) {
	tb.Helper()
	fake := &fakeStore{}
	result, err := importInto(tb, fake, dataPath)
	require.NoError(tb, err)
	return fake, result
}

// importRefused imports b into a fresh store and returns the reason the import was refused with.
func importRefused(tb testing.TB, b *v9fixture.Builder) (string, *fakeStore) {
	tb.Helper()
	return importRefusedFrom(tb, snapshotPath(tb, b))
}

// importRefusedFrom imports the snapshot at dataPath into a fresh store and returns the reason the import was
// refused with.
func importRefusedFrom(tb testing.TB, dataPath string) (string, *fakeStore) {
	tb.Helper()
	fake := &fakeStore{}
	_, err := importInto(tb, fake, dataPath)
	return importReason(tb, err), fake
}

// importFailingValidation imports b into a fresh store and requires the import to fail validation.
func importFailingValidation(tb testing.TB, b *v9fixture.Builder) (store.Result, *fakeStore) {
	tb.Helper()
	fake := &fakeStore{}
	result, err := importBuilt(tb, fake, b)
	require.ErrorIs(tb, err, store.ErrValidationFailed)
	return result, fake
}

// importReason returns the reason of the *importer.UnmappableError err must be.
func importReason(tb testing.TB, err error) string {
	tb.Helper()
	var unmappable *importer.UnmappableError
	require.ErrorAs(tb, err, &unmappable, "expected *importer.UnmappableError, got %v", err)
	return unmappable.Reason
}

var (
	investDay    = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	investLater  = time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	buyCode      = new(int64(3))
	dividendCode = new(int64(10))
	priceDay1    = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	priceDay2    = time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
)

// newChequing adds an active CAD chequing account named "Chequing".
func newChequing(b *v9fixture.Builder) int64 {
	return b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
}

// newVisa adds an active CAD credit card account named "Visa Infinite".
func newVisa(b *v9fixture.Builder) int64 {
	return b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
}

// newBrokerage adds the account every investment test imports into.
func newBrokerage(b *v9fixture.Builder) int64 {
	return b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
}

func newAcme(b *v9fixture.Builder) int64 {
	return b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
}

// transactionWithEntry adds row plus one split for its whole amount, so the snapshot passes split
// validation, and returns the transaction's Z_PK.
func transactionWithEntry(b *v9fixture.Builder, row v9fixture.TransactionRow) int64 {
	pk := b.Transaction(row)
	b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount})
	return pk
}

// investmentWithEntry adds an investment transaction with the one entry Quicken writes for it, in row's amount,
// and returns its Z_PK.
func investmentWithEntry(b *v9fixture.Builder, row v9fixture.TransactionRow) int64 {
	pk := b.InvestmentTransaction(row)
	b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount})
	return pk
}

func chequingWithOneReconciledTxn(b *v9fixture.Builder, cents string) int64 {
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: cents, PostedDate: &posted, Status: &reconciled})
	return acctPK
}

// transferLeg adds a one-entry transaction in account whose entry carries
// quickenID and the ZTRANSFER text link, returning the entry's Z_PK.
func transferLeg(b *v9fixture.Builder, account int64, amount string, quickenID int64, link string) int64 {
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &day})
	return b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, QuickenID: quickenID, Transfer: link})
}

// execOn runs query against the SQLite file at dataPath, for rows v9fixture.Builder cannot express.
func execOn(tb testing.TB, dataPath, query string, args ...any) {
	tb.Helper()
	db, err := sql.Open("sqlite3", dataPath)
	require.NoError(tb, err)
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(tb.Context(), query, args...)
	require.NoError(tb, err)
}

// setColumnBlob overwrites column on the row identified by pk with value as
// a real SQLite BLOB, for fixture shapes v9fixture.Builder has no field for.
func setColumnBlob(tb testing.TB, dataPath, table, column string, pk int64, value []byte) {
	tb.Helper()
	execOn(tb, dataPath, "UPDATE "+table+" SET "+column+" = ? WHERE Z_PK = ?", value, pk)
}

// setColumnEmptyText overwrites column with a zero-length SQLite TEXT
// value — distinct from setColumnBlob's zero-length BLOB.
func setColumnEmptyText(tb testing.TB, dataPath, table, column string, pk int64) {
	tb.Helper()
	execOn(tb, dataPath, "UPDATE "+table+" SET "+column+" = '' WHERE Z_PK = ?", pk)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func splitIDFor(pk int64) string { return fmt.Sprintf("split-%d", pk) }

func accountIDFor(pk int64) string { return fmt.Sprintf("acct-%d", pk) }

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

func splitByID(fake *fakeStore, id string) store.Split {
	for _, s := range fake.Rows.Splits {
		if s.ID == id {
			return s
		}
	}
	return store.Split{}
}
