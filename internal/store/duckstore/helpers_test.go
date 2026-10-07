package duckstore_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"os"
	"slices"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBuiltStore builds a store of minimalRows and one exchange rate in a fresh directory and returns a Store over it built with opts.
func newBuiltStore(t *testing.T, opts ...duckstore.Option) *duckstore.Store {
	t.Helper()
	dir := t.TempDir()
	src := &fakeRates{refresh: store.RatesRefresh{Rates: []store.Rate{ratesOn(13, 1_250_000, "IEXE0101")}}}
	rows := minimalRows()
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, buy(acctOne, secAcme, 30, day(2026, time.March, 18), oneShare))
	_, err := duckstore.New(dir, duckstore.WithRates(src)).Replace(t.Context(), rows)
	require.NoError(t, err)
	return duckstore.New(dir, opts...)
}

var errQueryFailed = errors.New("query failed")

// spyReadDB counts Close calls on a real read connection and injects faults into its reads.
type spyReadDB struct {
	duckstore.ReadDB

	queryFault  error
	scanFault   error
	checkFaults map[string]error // fails one of the open's format checks, keyed by its query
	onQuery     func()           // runs before every read query
	closes      int
	passQueries int // the first passQueries read queries run for real before queryFault or scanFault apply
	queries     int
}

func (s *spyReadDB) QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error {
	if slices.Contains(duckstore.FormatCheckQueries, query) {
		if fault, ok := s.checkFaults[query]; ok {
			return fault
		}
		return s.ReadDB.QueryRows(ctx, query, args, row)
	}
	s.startQuery()
	s.queries++
	if s.queries <= s.passQueries {
		return s.ReadDB.QueryRows(ctx, query, args, row)
	}
	if s.queryFault != nil {
		return s.queryFault
	}
	if s.scanFault != nil {
		return row(func(...any) error { return s.scanFault })
	}
	return s.ReadDB.QueryRows(ctx, query, args, row)
}

func (s *spyReadDB) QueryTable(ctx context.Context, query string, maxRows int) (duckdb.Table, error) {
	s.startQuery()
	if s.queryFault != nil {
		return duckdb.Table{}, s.queryFault
	}
	return s.ReadDB.QueryTable(ctx, query, maxRows)
}

func (s *spyReadDB) Close() error {
	s.closes++
	return s.ReadDB.Close()
}

func (s *spyReadDB) startQuery() {
	if s.onQuery != nil {
		s.onQuery()
	}
}

// spyOpener opens the store read-only for real and hands spy the connection to wrap.
func spyOpener(spy *spyReadDB) duckstore.Option {
	return duckstore.WithOpenReadOnly(func(ctx context.Context, path string) (duckstore.ReadDB, error) {
		db, err := duckdb.OpenReadOnly(ctx, path)
		if err != nil {
			return nil, err
		}
		spy.ReadDB = db
		return spy, nil
	})
}

// failingOpener is an opener that fails with fault.
func failingOpener(fault error) duckstore.Option {
	return duckstore.WithOpenReadOnly(func(context.Context, string) (duckstore.ReadDB, error) {
		return nil, fault
	})
}

// ioFault is the error chain duckdb.DB returns for a failed read: the
// adapter's wrap around the driver's own error type.
func ioFault(op string) error {
	return fmt.Errorf("%s: %w", op, &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO, Msg: "IO Error: disk read failed"})
}

// skipAsRoot skips t under root, whom file modes do not stop.
func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
}

// day is the civil day year-month-dayOfMonth at UTC midnight.
func day(year int, month time.Month, dayOfMonth int) time.Time {
	return time.Date(year, month, dayOfMonth, 0, 0, 0, 0, time.UTC)
}

// openReadOnly opens the store file at path read-only, closing it when t ends.
func openReadOnly(t *testing.T, path string) *duckdb.DB {
	t.Helper()
	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// errScanFailed has the shape of database/sql's Scan conversion error.
var errScanFailed = errors.New(`sql: Scan error on column index 1, name "id": converting NULL to string is unsupported`)

// localToday is today's local calendar date at UTC midnight, the shape a DATE column reads back as.
func localToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func account(id string, sourceID int64, name, accountType, currency string) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: name, Type: accountType, Currency: currency, Active: true}
}

func transaction(id, accountID string, date time.Time, cents int64) store.Transaction {
	return store.Transaction{ID: id, SourceID: 1, AccountID: accountID, Date: date, Amount: cents, Currency: "CAD", Status: "uncleared"}
}

const (
	acctChequing   = "acct-chq"
	acctRetirement = "acct-ret"
	acctEUR        = "acct-eur"
	secControl     = "sec-control"
)

// balanceRows is holdingRows with brokerage accounts acct-1 (CAD) and acct-2 (USD), a CAD chequing account, a CAD
// retirement account and a EUR brokerage account; Control Corp (CAD) joins its securities.
func balanceRows(txns ...store.InvestmentTransaction) store.Rows {
	rows := holdingRows(txns...)
	rows.Accounts[0].Type = store.AccountTypeBrokerage
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctChequing, SourceID: 3, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
		store.Account{ID: acctRetirement, SourceID: 4, Name: "RRSP", Type: store.AccountTypeRetirement, Currency: "CAD", Active: true},
		store.Account{ID: acctEUR, SourceID: 5, Name: "Euro Brokerage", Type: store.AccountTypeBrokerage, Currency: "EUR", Active: true})
	rows.Securities = append(rows.Securities,
		store.Security{ID: secControl, SourceID: 5, Name: "Control Corp", Ticker: new("CTRL"), Currency: new("CAD")})
	return rows
}

// cashRows is a store whose only account is a CAD chequing account with txns.
func cashRows(txns ...store.Transaction) store.Rows {
	rows := noTransactionRows()
	rows.Transactions = txns
	return rows
}

// minimalRows fills every table (transfers: one paired, one one-sided) so a
// round trip covers each table and each nullable column set and NULL.
func minimalRows() store.Rows {
	return store.Rows{
		Accounts: []store.Account{{
			ID: "acct-1", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD",
			Institution: new("Big Bank"), Closed: false, Active: true,
		}},
		Categories: []store.Category{{
			ID: "cat-1", SourceID: 1, Name: "Groceries", FullPath: "Groceries", Kind: "expense", Hidden: false,
		}},
		Payees: []store.Payee{{ID: "payee-1", SourceID: 1, Name: "Coffee Shop"}},
		Tags:   []store.Tag{{ID: "tag-1", SourceID: 1, Name: "Reimbursable"}},
		Transactions: []store.Transaction{{
			ID: "txn-1", SourceID: 1, AccountID: "acct-1",
			Date: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), PayeeID: new("payee-1"), Memo: new("Beans"),
			Amount: 1234, Currency: "CAD", Status: "uncleared", ChequeNumber: new("101"),
		}},
		Splits: []store.Split{{
			ID: "split-1", SourceID: 1, TransactionID: "txn-1", CategoryID: new("cat-1"),
			Amount: 1234, Memo: new("split memo"),
		}, {
			ID: "split-4", SourceID: 4, TransactionID: "txn-1", CategoryID: new("cat-1"), Amount: 1234,
		}},
		SplitTags: []store.SplitTag{{SplitID: "split-1", TagID: "tag-1"}},
		Transfers: []store.Transfer{
			{ID: "xfer-1", FromSplitID: "split-1", ToSplitID: new("split-2"), CrossCurrency: true},
			{ID: "xfer-3", FromSplitID: "split-3", OtherAccount: new("Savings")},
		},
		Securities: []store.Security{
			{ID: "sec-1", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
			{ID: "sec-2", SourceID: 2, Name: "Plain Fund"},
		},
		Prices: []store.Price{{
			SecurityID: "sec-1", SourceID: 7, Date: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), Price: 12_345_678,
		}},
		InvestmentTransactions: []store.InvestmentTransaction{{
			ID: "inv-1", SourceID: 21, AccountID: "acct-1", SecurityID: new("sec-1"),
			Date: time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC), Action: "split", Shares: new(int64(1_500_000)),
			Amount: 12_345, Commission: new(int64(84_998)), CostBasis: new(int64(100_050)), Currency: "CAD", Memo: new("note"),
			SplitNewShares: new(int64(12_000_000)), SplitOldShares: new(int64(1_000_000)),
		}, {
			ID: "inv-2", SourceID: 22, AccountID: "acct-1",
			Date: time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC), Action: "dividend", Amount: 500, Currency: "CAD",
		}},
		ImportRuns: []store.ImportRun{{
			ID: 1, StartedAt: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), FinishedAt: time.Date(2026, 9, 27, 14, 30, 7, 0, time.UTC),
			Snapshot: store.SnapshotRef{
				Path: "/snapshots/20260927T143005Z.sqlite", SHA256: "9f86", SchemaFingerprint: "sha256:abc",
				TakenAt: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), Source: "/Users/alex/Documents/Home.quicken",
			},
			Counts: store.Counts{
				Accounts: 1, Categories: 2, Payees: 3, Tags: 4, Transactions: 5, Splits: 6, SplitTags: 7, Transfers: 8,
				Securities: 18, Prices: 19, InvestmentTransactions: 20,
			},
			BalancesChecked: 9, BalancesMismatched: 10, SplitsMismatched: 11, TransfersOneSided: 12,
			BalancesNeverReconciled: 14, InvestmentAccounts: 15, TransfersPaired: 16, TransfersCrossCurrency: 17, SharesChecked: 21,
		}},
	}
}

// faultDB wraps the real partial-file connection and injects at most one
// fault; with no fault configured it passes every call through.
type faultDB struct {
	duckstore.DB

	path             string
	checkpointFault  error
	afterCheckpoint  func()
	walOnClose       bool
	appendFaultTable string
	appendFault      error
	queryFaultOn     string // the build query that fails with queryFault, or whose first row's scan fails with scanFault
	queryFault       error
	scanFault        error
	execFaultOn      string // the statement Exec fails with execFault
	execFault        error
	appended         []string // every table AppendRows was asked to load, in order
	checkpoints      int      // CheckpointClose calls
	closes           int      // Close calls
}

// QueryRows fails with queryFault for the query queryFaultOn, hands its row callback a scan that
// fails with scanFault, else queries for real.
func (f *faultDB) QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error {
	if f.queryFault != nil && query == f.queryFaultOn {
		return f.queryFault
	}
	if f.scanFault != nil && query == f.queryFaultOn {
		return row(func(...any) error { return f.scanFault })
	}
	return f.DB.QueryRows(ctx, query, args, row)
}

// Exec fails with execFault for the statement execFaultOn, else runs it for real.
func (f *faultDB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if f.execFault != nil && query == f.execFaultOn {
		return nil, f.execFault
	}
	return f.DB.Exec(ctx, query, args...)
}

// AppendRows fails with appendFault for appendFaultTable, else appends for real.
func (f *faultDB) AppendRows(ctx context.Context, table string, rows [][]any) error {
	f.appended = append(f.appended, table)
	if f.appendFault != nil && table == f.appendFaultTable {
		return fmt.Errorf("append %s: %w", table, f.appendFault)
	}
	return f.DB.AppendRows(ctx, table, rows)
}

// CheckpointClose returns checkpointFault wrapped as duckdb.CheckpointClose
// wraps a driver error, or runs the real one and then afterCheckpoint.
func (f *faultDB) CheckpointClose(ctx context.Context) error {
	f.checkpoints++
	if f.checkpointFault != nil {
		return fmt.Errorf("checkpoint %s: %w", f.path, f.checkpointFault)
	}
	err := f.DB.CheckpointClose(ctx)
	if f.afterCheckpoint != nil {
		f.afterCheckpoint()
	}
	return err
}

// Close closes the real connection, then leaves a .wal beside the partial
// when walOnClose is set, as a crash mid-checkpoint would.
func (f *faultDB) Close() error {
	f.closes++
	err := f.DB.Close()
	if f.walOnClose {
		_ = os.WriteFile(f.path+".wal", []byte("wal"), 0o600)
	}
	return err
}

// newFaultStore returns a Store over dir that builds its partial file
// through f over a real DuckDB connection, configured further by opts.
func newFaultStore(dir string, f *faultDB, opts ...duckstore.Option) *duckstore.Store {
	create := duckstore.WithCreate(func(ctx context.Context, path string) (duckstore.DB, error) {
		db, err := duckdb.Create(ctx, path)
		if err != nil {
			return nil, err
		}
		f.DB, f.path = db, path
		return f, nil
	})
	return duckstore.New(dir, append([]duckstore.Option{create}, opts...)...)
}

func direntNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

func assertScalar(t *testing.T, db *duckdb.DB, query, want string) {
	t.Helper()
	var got string
	err := db.QueryRows(t.Context(), query, nil, func(scan func(dest ...any) error) error {
		return scan(&got)
	})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

const (
	tenUnits         = 10_000_000
	maxDecimal18x6   = 999_999_999_999_999_999
	maxHoldingValue  = "999999999999999998000000.00"
	maxHoldingInUSD  = "799999999999999998400000.00"
	maxHoldingInCAD  = "1249999999999999997500000.00"
	placeholderPrice = "1899-12-29"
)

// quote is a price of millionths for security on date.
func quote(security string, source int64, date time.Time, millionths int64) store.Price {
	return store.Price{SecurityID: security, SourceID: source, Date: date, Price: millionths}
}

const (
	secUSD        = "sec-usd"
	secEUR        = "sec-eur"
	secNoCurrency = "sec-none"
	secGhost      = "sec-ghost"
)

// holdingRows is a store whose only investment transactions are txns, with a CAD account and a USD one; its securities are
// Acme (CAD), Globex (USD), Euro Fund (EUR) and Plain Fund (no ticker, no currency), and it has no prices.
func holdingRows(txns ...store.InvestmentTransaction) store.Rows {
	rows := noTransactionRows()
	rows.Accounts = append(rows.Accounts, store.Account{
		ID: acctTwo, SourceID: 2, Name: "Brokerage USD", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true,
	})
	rows.Securities = []store.Security{
		{ID: secAcme, SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: secUSD, SourceID: 2, Name: "Globex Inc", Ticker: new("GLBX"), Currency: new("USD")},
		{ID: secEUR, SourceID: 3, Name: "Euro Fund", Ticker: new("EURF"), Currency: new("EUR")},
		{ID: secNoCurrency, SourceID: 4, Name: "Plain Fund"},
	}
	rows.Prices = nil
	rows.InvestmentTransactions = txns
	return rows
}

func cents(text string) *big.Int {
	n, _ := new(big.Int).SetString(text, 10)
	return n
}

// noTransactionRows is minimalRows without any cash or investment transaction.
func noTransactionRows() store.Rows {
	rows := minimalRows()
	rows.Transactions, rows.Splits, rows.SplitTags, rows.Transfers, rows.InvestmentTransactions = nil, nil, nil, nil, nil
	return rows
}

// fakeRates is a RatesSource that records each request and plays back one refresh.
type fakeRates struct {
	requests []store.RatesRequest
	refresh  store.RatesRefresh
	err      error
	during   func(ctx context.Context) // runs inside Refresh, before it returns
}

func (f *fakeRates) Refresh(ctx context.Context, req store.RatesRequest) (store.RatesRefresh, error) {
	f.requests = append(f.requests, req)
	if f.during != nil {
		f.during(ctx)
	}
	return f.refresh, f.err
}

func ratesOn(day int, usdCAD money.Rate, series string) store.Rate {
	return store.Rate{Date: time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC), USDCAD: usdCAD, Series: series}
}

const (
	acctOne  = "acct-1"
	acctTwo  = "acct-2"
	secAcme  = "sec-1"
	secBeta  = "sec-2"
	oneShare = 1_000_000
)

func buy(account, security string, source int64, day time.Time, millionths int64) store.InvestmentTransaction {
	return store.InvestmentTransaction{
		ID: fmt.Sprintf("itxn-%d", source), SourceID: source, AccountID: account, SecurityID: &security,
		Date: day, Action: "buy", Shares: &millionths, Amount: 100, Currency: "CAD",
	}
}

func splitOf(account, security string, source int64, day time.Time, newShares, oldShares int64) store.InvestmentTransaction {
	txn := buy(account, security, source, day, 0)
	txn.Action, txn.Shares = "split", nil
	txn.SplitNewShares, txn.SplitOldShares = &newShares, &oldShares
	return txn
}

// assertOtherFault requires err to be the *store.OpenError of an unclassified fault, its Reason the one line reason.
func assertOtherFault(t *testing.T, err error, reason string) {
	t.Helper()
	openErr, ok := errors.AsType[*store.OpenError](err)
	require.True(t, ok, "want *store.OpenError, got %v", err)
	assert.Equal(t, store.OpenFaultOther, openErr.Fault)
	assert.Equal(t, reason, openErr.Reason)
}

// march is a date in March 2026: the 13th is a Friday, the 14th and 15th the weekend after it.
func march(day int) time.Time { return time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC) }

func newStoreWithRates(t *testing.T, rows store.Rows, rates ...store.Rate) *duckstore.Store {
	t.Helper()
	dir := t.TempDir()
	src := &fakeRates{refresh: store.RatesRefresh{Rates: rates}}
	_, err := duckstore.New(dir, duckstore.WithRates(src)).Replace(t.Context(), rows)
	require.NoError(t, err)
	return duckstore.New(dir)
}

func fridayAndMonday() []store.Rate {
	return []store.Rate{ratesOn(13, fridayRate, "FXUSDCAD"), ratesOn(16, mondayRate, "FXUSDCAD")}
}

const (
	acctInReports  = "acct-in"
	acctNotReports = "acct-out"
	acctLinked     = "acct-linked"
	acctUSD        = "acct-usd"
	catExpense     = "cat-expense"
	catIncome      = "cat-income"
	catSystem      = "cat-system"
	keepSplit      = "keep"
)

// reportRows is a store whose only view-visible split is `keep`, an expense.
// Its unmatched transfer leg has a NULL to_split_id, as real stores do.
func reportRows() store.Rows {
	rows := store.Rows{
		Accounts: []store.Account{
			{ID: acctInReports, SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
			{ID: acctNotReports, SourceID: 2, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
			{ID: acctLinked, SourceID: 6, Name: "Netskope 401(k)", Type: "retirement", Currency: "USD", Active: true, LinkedTracking: true},
		},
		Categories: []store.Category{
			{ID: catExpense, SourceID: 1, Name: "Groceries", FullPath: "Groceries", Kind: "expense"},
			{ID: catIncome, SourceID: 2, Name: "Salary", FullPath: "Salary", Kind: "income"},
			{ID: catSystem, SourceID: 3, Name: "Adjustment", FullPath: "Adjustment", Kind: "system"},
		},
		Transfers: []store.Transfer{{ID: "xfer-orphan", FromSplitID: "orphan-leg"}},
	}
	addSplit(&rows, splitSpec{id: keepSplit, category: new(catExpense), amount: -1000})
	return rows
}

// splitSpec is one split and the transaction carrying it; account defaults to
// acctInReports, currency to CAD and date to 2026-03-15; tags are tag ids.
type splitSpec struct {
	id       string
	account  string
	currency string
	category *string
	amount   int64
	excluded bool
	payee    *string
	date     time.Time
	tags     []string
}

func addSplit(rows *store.Rows, spec splitSpec) {
	account := spec.account
	if account == "" {
		account = acctInReports
	}
	currency := spec.currency
	if currency == "" {
		currency = "CAD"
	}
	date := spec.date
	if date.IsZero() {
		date = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	}
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-" + spec.id, SourceID: int64(len(rows.Transactions) + 1), AccountID: account,
		Date: date, PayeeID: spec.payee,
		Amount: spec.amount, Currency: currency, Status: "uncleared", ExcludedFromReports: spec.excluded,
	})
	rows.Splits = append(rows.Splits, store.Split{
		ID: spec.id, SourceID: int64(len(rows.Splits) + 1), TransactionID: "txn-" + spec.id,
		CategoryID: spec.category, Amount: spec.amount,
	})
	for _, tag := range spec.tags {
		rows.SplitTags = append(rows.SplitTags, store.SplitTag{SplitID: spec.id, TagID: tag})
	}
}

func newStoreWith(t *testing.T, rows store.Rows) *duckstore.Store {
	t.Helper()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), rows)
	require.NoError(t, err)
	return duckstore.New(dir)
}

// queryTexts runs query and returns every cell as DuckDB's text for it.
func queryTexts(t *testing.T, st *duckstore.Store, query string) [][]string {
	t.Helper()
	got, err := st.Query(t.Context(), query, 0)
	require.NoError(t, err)
	texts := make([][]string, len(got.Rows))
	for i, row := range got.Rows {
		texts[i] = make([]string, len(row))
		for j, cell := range row {
			texts[i][j] = cell.Text
		}
	}
	return texts
}

// netWorthRead reads st's net worth on dates.
func netWorthRead(t *testing.T, st *duckstore.Store, dates ...time.Time) store.NetWorth {
	t.Helper()
	got, err := st.NetWorth(t.Context(), store.NetWorthParams{Dates: dates})
	require.NoError(t, err)
	return got
}
