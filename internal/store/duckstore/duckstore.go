package duckstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/atomicfile"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
)

// FileName is quarry's store filename inside a Store's directory.
const FileName = "quarry.duckdb"

// FormatVersion is the store format this build of quarry writes and reads.
const FormatVersion = 7

// develVersion is the quarry_version recorded when no build version is known.
const develVersion = "(devel)"

// leftoverMaxAge is how old a store partial must be before Replace's sweep removes it.
const leftoverMaxAge = time.Hour

// Build file name parts; partialName and buildFilePattern are the only places the shape is spelled.
const (
	partialPrefix     = ".quarry-"
	partialSuffix     = ".duckdb.partial"
	partialNameLayout = "20060102T150405Z"
)

// partialName is this run's own build file name: its UTC start and process
// id, so no two concurrent runs share one.
func partialName(start time.Time) string {
	return fmt.Sprintf("%s%s-%d%s", partialPrefix, start.UTC().Format(partialNameLayout), os.Getpid(), partialSuffix)
}

// buildFilePattern matches any run's build file or its .wal, including the
// seconds-only name older releases left behind.
var buildFilePattern = regexp.MustCompile(`^` + regexp.QuoteMeta(partialPrefix) + `\d{8}T\d{6}Z(-\d+)?` +
	regexp.QuoteMeta(partialSuffix) + `(\.wal)?$`)

// moneyWidth and moneyScale match schemaDDL's DECIMAL(18,2) money columns.
const moneyWidth, moneyScale = 18, 2

// priceWidth and priceScale match schemaDDL's DECIMAL(18,6) price column.
const priceWidth, priceScale = 18, 6

// sharesWidth and sharesScale match schemaDDL's DECIMAL(18,6) shares and split columns.
const sharesWidth, sharesScale = 18, 6

// commissionWidth and commissionScale match schemaDDL's DECIMAL(18,4) commission column.
const commissionWidth, commissionScale = 18, 4

// DB is the connection a Store builds one partial file through.
// *duckdb.DB is the production implementation.
type DB interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	AppendRows(ctx context.Context, table string, rows [][]any) error
	// QueryRows runs query against the build file, calling row once per result row.
	QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error
	CheckpointClose(ctx context.Context) error
	Close() error
}

var _ DB = (*duckdb.DB)(nil)

// ScratchDB is the in-memory connection CheckShares loads transactions into and walks.
// *duckdb.DB is the production implementation.
type ScratchDB interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	AppendRows(ctx context.Context, table string, rows [][]any) error
	QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error
	Close() error
}

var _ ScratchDB = (*duckdb.DB)(nil)

// ReadDB is the read-only connection a Store answers one read through.
// *duckdb.DB is the production implementation.
type ReadDB interface {
	QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error
	QueryTable(ctx context.Context, query string, maxRows int) (duckdb.Table, error)
	Close() error
}

var _ ReadDB = (*duckdb.DB)(nil)

// Store builds quarry's DuckDB file inside one directory and answers reads
// from it.
type Store struct {
	dir           string
	create        func(ctx context.Context, path string) (DB, error)
	openReadOnly  func(ctx context.Context, path string) (ReadDB, error)
	scratch       func(ctx context.Context) (ScratchDB, error)
	quarryVersion string
	rates         RatesSource
}

// RatesSource supplies the exchange rates a build stores. Refresh returns a
// non-nil error only when ctx ended; any other failure comes back as
// RatesRefresh.FetchError.
type RatesSource interface {
	Refresh(ctx context.Context, req store.RatesRequest) (store.RatesRefresh, error)
}

// Option configures a Store.
type Option func(*Store)

// WithCreate replaces how a Store creates its partial build file, which by
// default is duckdb.Create. create must refuse an existing path.
func WithCreate(create func(ctx context.Context, path string) (DB, error)) Option {
	return func(s *Store) { s.create = create }
}

// WithOpenReadOnly replaces how a Store opens the store file to read it,
// which by default is duckdb.OpenReadOnly. open must not write to path.
func WithOpenReadOnly(open func(ctx context.Context, path string) (ReadDB, error)) Option {
	return func(s *Store) { s.openReadOnly = open }
}

// WithScratch replaces how CheckShares opens its in-memory scratch database, which by
// default is duckdb.CreateInMemory. scratch must return a database of its own each call.
func WithScratch(scratch func(ctx context.Context) (ScratchDB, error)) Option {
	return func(s *Store) { s.scratch = scratch }
}

// WithRates sets where Replace gets exchange rates from; without it the
// store's fx_rates table is left empty.
func WithRates(src RatesSource) Option {
	return func(s *Store) { s.rates = src }
}

// WithQuarryVersion sets the quarry_version Replace records; an empty v keeps
// the default "(devel)", so the column is never stored empty.
func WithQuarryVersion(v string) Option {
	return func(s *Store) {
		if v != "" {
			s.quarryVersion = v
		}
	}
}

// New returns a Store that builds quarry.duckdb inside dir.
func New(dir string, opts ...Option) *Store {
	s := &Store{dir: dir, create: createDuckDB, openReadOnly: openDuckDBReadOnly, scratch: createScratch, quarryVersion: develVersion}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Path returns the store file's path inside the Store's directory.
func (s *Store) Path() string {
	return filepath.Join(s.dir, FileName)
}

// Exists reports whether a store file is already at Path; a stat fault
// other than not-found counts as one existing.
func (s *Store) Exists() bool {
	_, err := os.Stat(s.Path())
	return !errors.Is(err, fs.ErrNotExist)
}

// createDuckDB is New's default creator; it returns a nil interface, never
// a typed nil, when duckdb.Create fails.
func createDuckDB(ctx context.Context, path string) (DB, error) {
	db, err := duckdb.Create(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("create store file: %w", err)
	}
	return db, nil
}

// createScratch is New's default scratch opener; it returns a nil interface, never a typed nil, when
// duckdb.CreateInMemory fails.
func createScratch(ctx context.Context) (ScratchDB, error) {
	db, err := duckdb.CreateInMemory(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // CreateInMemory already names the operation
	}
	return db, nil
}

// openDuckDBReadOnly is New's default read opener; it returns a nil
// interface, never a typed nil, when duckdb.OpenReadOnly fails.
func openDuckDBReadOnly(ctx context.Context, path string) (ReadDB, error) {
	db, err := duckdb.OpenReadOnly(ctx, path)
	if err != nil {
		return nil, err //nolint:wrapcheck // OpenReadOnly already names the path; callers add the operation
	}
	return db, nil
}

// openRead opens the store file read-only for one read and checks its
// format, refusing with *store.OpenError; it never creates the file.
func (s *Store) openRead(ctx context.Context) (ReadDB, error) {
	path := s.Path()
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, &store.OpenError{Fault: store.OpenFaultMissing, Path: path, Err: err}
	}
	db, err := s.openReadOnly(ctx, path)
	if err != nil {
		return nil, openFault(path, err)
	}
	if err := checkFormat(ctx, db, path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// errFormatMismatch is the fault behind a store whose store_info is not this build's format.
var errFormatMismatch = errors.New("store format is not this build's")

// Format-check queries, run by openRead before any read's own query.
const (
	columnExistsQuery = `SELECT count(*) FROM duckdb_columns()
WHERE database_name = current_database() AND schema_name = 'main' AND table_name = ? AND column_name = ?`
	formatVersionQuery = `SELECT format_version FROM store_info`
	snapshotPathQuery  = `SELECT snapshot_path FROM import_runs ORDER BY id DESC LIMIT 1`
)

// openFault classifies err, a failed open or format check of the store at path.
func openFault(path string, err error) *store.OpenError {
	fault := &store.OpenError{Path: path, Err: err}
	// The file decides whether the store vanished after the stat, never the driver's wording.
	if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
		fault.Fault = store.OpenFaultMissing
		return fault
	}
	switch {
	case duckdb.IsNotDatabase(err):
		fault.Fault = store.OpenFaultNotDuckDB
	case duckdb.IsPermission(err):
		fault.Fault = store.OpenFaultPermission
	case duckdb.IsLocked(err):
		fault.Fault = store.OpenFaultLocked
	default:
		fault.Fault = store.OpenFaultOther
		fault.Reason = faultReason(path, err)
	}
	return fault
}

// faultReason is err's first line, naming the store by path however the
// fault spelled it, or "unknown error" when that leaves nothing.
func faultReason(path string, err error) string {
	reason := duckdb.ErrorLine(err)
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		reason = pathErr.Err.Error()
	}
	// The driver names the resolved path, which can contain path (/private/var and /var): replace it first.
	if resolved, evalErr := filepath.EvalSymlinks(path); evalErr == nil && resolved != path {
		reason = strings.ReplaceAll(reason, resolved, path)
	}
	if reason == "" {
		return "unknown error"
	}
	return reason
}

// checkFormat refuses, as store.OpenFaultOtherFormat, a store whose
// store_info does not hold exactly one row of FormatVersion.
func checkFormat(ctx context.Context, db ReadDB, path string) error {
	// The catalog first: selecting from a missing store_info is a query error.
	hasInfo, err := hasColumn(ctx, db, "store_info", "format_version")
	if err != nil {
		return openFault(path, err)
	}
	if hasInfo {
		var version sql.NullInt64
		rows := 0
		err := db.QueryRows(ctx, formatVersionQuery, nil, func(scan func(dest ...any) error) error {
			rows++
			return scan(&version)
		})
		if err != nil {
			return openFault(path, err)
		}
		if rows == 1 && version.Int64 == FormatVersion { // NULL scans as 0
			return nil
		}
	}
	return &store.OpenError{
		Fault: store.OpenFaultOtherFormat, Path: path, SnapshotPath: snapshotPath(ctx, db), Err: errFormatMismatch,
	}
}

// snapshotPath is the highest-id import run's snapshot path, or "" when
// import_runs has no run to name.
func snapshotPath(ctx context.Context, db ReadDB) string {
	hasPath, _ := hasColumn(ctx, db, "import_runs", "snapshot_path")
	if !hasPath { // false on a catalog fault too
		return ""
	}
	var path sql.NullString
	err := db.QueryRows(ctx, snapshotPathQuery, nil, func(scan func(dest ...any) error) error {
		return scan(&path)
	})
	if err != nil {
		return ""
	}
	return path.String
}

// hasColumn reports whether the store's main schema has table.column.
func hasColumn(ctx context.Context, db ReadDB, table, column string) (bool, error) {
	var n int64
	err := db.QueryRows(ctx, columnExistsQuery, []any{table, column}, func(scan func(dest ...any) error) error {
		return scan(&n)
	})
	if err != nil {
		return false, fmt.Errorf("read store catalog: %w", err)
	}
	return n > 0, nil
}

// Replace swaps rows into quarry.duckdb through a build file, carrying the previous store's import_runs, findings and
// exchange rates. An unreadable table restarts (Replaced.HistoryFault, FindingsFault, RatesFault). On failure the existing
// store is untouched; a permission fault matches store.ErrStoreNotWritable, disk-full store.ErrDiskFull.
func (s *Store) Replace(ctx context.Context, rows store.Rows) (store.Replaced, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return store.Replaced{}, buildError(err)
	}
	s.sweepLeftovers()

	carried, historyFault := s.readHistory(ctx)

	finalPath := s.Path()
	partialPath := filepath.Join(s.dir, partialName(time.Now()))

	db, err := s.create(ctx, partialPath)
	if err != nil {
		removePartial(partialPath)
		return store.Replaced{}, buildError(err)
	}

	builtAt := time.Now().UTC()
	states, err := build(ctx, db, rows, carried, builtAt)
	if err != nil {
		_ = db.Close()
		removePartial(partialPath)
		return store.Replaced{}, buildError(err)
	}

	ratesSummary, err := s.finishBuild(ctx, db, rows, carried, builtAt)
	if err != nil {
		_ = db.Close()
		removePartial(partialPath)
		return store.Replaced{}, buildError(err)
	}

	if err := db.CheckpointClose(ctx); err != nil {
		_ = db.Close()
		removePartial(partialPath)
		return store.Replaced{}, buildError(err)
	}

	// The last point an interrupt can still keep the previous store.
	if err := ctx.Err(); err != nil {
		removePartial(partialPath)
		return store.Replaced{}, buildError(err)
	}

	if err := removeStaleWAL(finalPath); err != nil {
		removePartial(partialPath)
		return store.Replaced{}, buildError(err)
	}

	if err := os.Rename(partialPath, finalPath); err != nil {
		removePartial(partialPath)
		return store.Replaced{}, buildError(err)
	}
	atomicfile.SyncDir(s.dir)

	return store.Replaced{
		Path: finalPath, HistoryFault: historyFault, Findings: finding.Classify(states, nil).Counts, FindingStates: states,
		FindingsCarried: carried.findingsCarried, FindingsFault: carried.findingsFault, StoreUnreadable: carried.unreadable,
		Rates: ratesSummary, RatesFault: carried.ratesFault,
	}, nil
}

// sweepLeftovers best-effort removes buildFilePattern matches in
// s.dir older than leftoverMaxAge; a ReadDir or Remove failure is silent.
func (s *Store) sweepLeftovers() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-leftoverMaxAge)
	for _, entry := range entries {
		if entry.IsDir() || !buildFilePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(s.dir, entry.Name()))
	}
}

// removeStaleWAL removes finalPath+".wal", left by the store being
// replaced, ignoring the file already being absent.
func removeStaleWAL(finalPath string) error {
	if err := os.Remove(finalPath + ".wal"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove stale wal: %w", err)
	}
	return nil
}

// buildFailureError is a failed build: Error and Unwrap reach cause alone, while Is
// also matches the store sentinel the fault was classified as.
type buildFailureError struct {
	sentinel error
	cause    error
}

func (e buildFailureError) Error() string        { return "build store: " + e.cause.Error() }
func (e buildFailureError) Unwrap() error        { return e.cause }
func (e buildFailureError) Is(target error) bool { return target == e.sentinel }

// buildError wraps a failed build's cause, tagging permission and disk-full faults.
func buildError(cause error) error {
	var sentinel error
	switch {
	case duckdb.IsPermission(cause):
		sentinel = store.ErrStoreNotWritable
	case duckdb.IsDiskFull(cause):
		sentinel = store.ErrDiskFull
	}
	return buildFailureError{sentinel: sentinel, cause: cause}
}

// removePartial removes path and path+".wal", ignoring either being
// already absent.
func removePartial(path string) {
	_ = os.Remove(path)
	_ = os.Remove(path + ".wal")
}

// build loads schema, views, rows, then findings merged with carried. It returns the state of each
// finding it recorded. finishBuild completes the file; until it appends store_info the file is not a store.
func build(ctx context.Context, db DB, rows store.Rows, carried history, builtAt time.Time) ([]finding.State, error) {
	if _, err := db.Exec(ctx, schemaDDL+accountBalancesViewDDL()+cashFlowViewDDL()+spendingViewDDL); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if err := loadRows(ctx, db, rows, carried); err != nil {
		return nil, err
	}
	if err := loadHoldingShares(ctx, db); err != nil {
		return nil, err
	}
	states, err := loadFindings(ctx, db, carried.findings, rows.ReferencedCategoryIDs, builtAt)
	if err != nil {
		return nil, err
	}
	return states, nil
}

// loadRows bulk-loads every table of rows, and the carried import_runs ahead of the new run.
func loadRows(ctx context.Context, db DB, rows store.Rows, carried history) error {
	if err := appendTable(ctx, db, "accounts", accountRows(rows.Accounts)); err != nil {
		return err
	}
	if err := appendTable(ctx, db, "categories", categoryRows(rows.Categories)); err != nil {
		return err
	}
	if err := appendTable(ctx, db, "payees", payeeRows(rows.Payees)); err != nil {
		return err
	}
	if err := appendTable(ctx, db, "tags", tagRows(rows.Tags)); err != nil {
		return err
	}
	txnRows, err := transactionRows(rows.Transactions)
	if err != nil {
		return err
	}
	if err := appendTable(ctx, db, "transactions", txnRows); err != nil {
		return err
	}
	splitRows, err := splitRows(rows.Splits)
	if err != nil {
		return err
	}
	if err := appendTable(ctx, db, "splits", splitRows); err != nil {
		return err
	}
	if err := appendTable(ctx, db, "split_tags", splitTagRows(rows.SplitTags)); err != nil {
		return err
	}
	if err := appendTable(ctx, db, "transfers", transferRows(rows.Transfers)); err != nil {
		return err
	}
	if err := appendTable(ctx, db, "securities", securityRows(rows.Securities)); err != nil {
		return err
	}
	priceRows, err := priceRows(rows.Prices)
	if err != nil {
		return err
	}
	if err := appendTable(ctx, db, "prices", priceRows); err != nil {
		return err
	}
	invRows, err := investmentTransactionRows(rows.InvestmentTransactions)
	if err != nil {
		return err
	}
	if err := appendTable(ctx, db, "investment_transactions", invRows); err != nil {
		return err
	}
	return appendTable(ctx, db, "import_runs", importRunRows(carried, rows.ImportRuns))
}

// rowAppender is the bulk-load half of DB and ScratchDB.
type rowAppender interface {
	AppendRows(ctx context.Context, table string, rows [][]any) error
}

// appendTable bulk-loads rows into table, naming the table on failure.
func appendTable(ctx context.Context, db rowAppender, table string, rows [][]any) error {
	if err := db.AppendRows(ctx, table, rows); err != nil {
		return fmt.Errorf("load %s: %w", table, err)
	}
	return nil
}

func nullableStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullablePtrTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func nullableNonEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func accountRows(accounts []store.Account) [][]any {
	out := make([][]any, len(accounts))
	for i, a := range accounts {
		out[i] = []any{a.ID, a.SourceID, a.Name, a.Type, a.Currency, nullableStr(a.Institution), a.Closed, a.Active, !a.NotInReports, a.LinkedTracking}
	}
	return out
}

func categoryRows(categories []store.Category) [][]any {
	out := make([][]any, len(categories))
	for i, c := range categories {
		out[i] = []any{c.ID, c.SourceID, nullableStr(c.ParentID), c.Name, c.FullPath, c.Kind, c.Hidden}
	}
	return out
}

func payeeRows(payees []store.Payee) [][]any {
	out := make([][]any, len(payees))
	for i, p := range payees {
		out[i] = []any{p.ID, p.SourceID, p.Name}
	}
	return out
}

func tagRows(tags []store.Tag) [][]any {
	out := make([][]any, len(tags))
	for i, tg := range tags {
		out[i] = []any{tg.ID, tg.SourceID, tg.Name}
	}
	return out
}

func transactionRows(transactions []store.Transaction) ([][]any, error) {
	out := make([][]any, len(transactions))
	for i, t := range transactions {
		amount, err := duckdb.Decimal(t.Amount, moneyWidth, moneyScale)
		if err != nil {
			return nil, fmt.Errorf("transaction %s: %w", t.ID, err)
		}
		out[i] = []any{
			t.ID, t.SourceID, t.AccountID, t.Date, nullableStr(t.PayeeID), nullableStr(t.Memo),
			amount, t.Currency, t.Status, nullableStr(t.ChequeNumber), t.ExcludedFromReports, nullablePtrTime(t.PostedDate),
		}
	}
	return out, nil
}

func splitRows(splits []store.Split) ([][]any, error) {
	out := make([][]any, len(splits))
	for i, s := range splits {
		amount, err := duckdb.Decimal(s.Amount, moneyWidth, moneyScale)
		if err != nil {
			return nil, fmt.Errorf("split %s: %w", s.ID, err)
		}
		out[i] = []any{
			s.ID, s.SourceID, s.TransactionID, nullableStr(s.CategoryID), amount, nullableStr(s.Memo), nullableStr(s.TransferAccountID),
		}
	}
	return out, nil
}

func splitTagRows(splitTags []store.SplitTag) [][]any {
	out := make([][]any, len(splitTags))
	for i, st := range splitTags {
		out[i] = []any{st.SplitID, st.TagID}
	}
	return out
}

func transferRows(transfers []store.Transfer) [][]any {
	out := make([][]any, len(transfers))
	for i, tr := range transfers {
		out[i] = []any{tr.ID, tr.FromSplitID, nullableStr(tr.ToSplitID), tr.CrossCurrency, nullableStr(tr.OtherAccount)}
	}
	return out
}

func securityRows(securities []store.Security) [][]any {
	out := make([][]any, len(securities))
	for i, sec := range securities {
		out[i] = []any{sec.ID, sec.SourceID, sec.Name, nullableStr(sec.Ticker), nullableStr(sec.Currency)}
	}
	return out
}

func priceRows(prices []store.Price) ([][]any, error) {
	out := make([][]any, len(prices))
	for i, p := range prices {
		price, err := duckdb.Decimal(p.Price, priceWidth, priceScale)
		if err != nil {
			return nil, fmt.Errorf("price of %s on %s: %w", p.SecurityID, p.Date.Format(time.DateOnly), err)
		}
		out[i] = []any{p.SecurityID, p.SourceID, p.Date, price}
	}
	return out, nil
}

func investmentTransactionRows(txns []store.InvestmentTransaction) ([][]any, error) {
	out := make([][]any, len(txns))
	for i, t := range txns {
		shares, err1 := decimalCell("shares", t.Shares, sharesWidth, sharesScale)
		amount, err2 := decimalCell("amount", &t.Amount, moneyWidth, moneyScale)
		commission, err3 := decimalCell("commission", t.Commission, commissionWidth, commissionScale)
		splitNew, err4 := decimalCell("split_new_shares", t.SplitNewShares, sharesWidth, sharesScale)
		splitOld, err5 := decimalCell("split_old_shares", t.SplitOldShares, sharesWidth, sharesScale)
		if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
			return nil, fmt.Errorf("investment transaction %s: %w", t.ID, err)
		}
		out[i] = []any{
			t.ID, t.SourceID, t.AccountID, nullableStr(t.SecurityID), t.Date, t.Action, shares, amount, commission,
			t.Currency, nullableStr(t.Memo), splitNew, splitOld,
		}
	}
	return out, nil
}

// decimalCell is the append cell for v as DECIMAL(width, scale): NULL for nil, else v unscaled;
// it names column when v is out of range.
func decimalCell(column string, v *int64, width, scale uint8) (any, error) {
	if v == nil {
		return nil, nil //nolint:nilnil // a NULL cell has no error
	}
	cell, err := duckdb.Decimal(*v, width, scale)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", column, err)
	}
	return cell, nil
}

func importRunRows(carried history, runs []store.ImportRun) [][]any {
	out := append([][]any(nil), carried.rows...)
	for i, r := range runs {
		c := r.Counts
		out = append(out, []any{
			carried.maxID + int64(i) + 1, r.StartedAt, r.FinishedAt, r.Snapshot.Path, r.Snapshot.SHA256, r.Snapshot.SchemaFingerprint,
			int64(c.Accounts), int64(c.Categories), int64(c.Payees), int64(c.Tags),
			int64(c.Transactions), int64(c.Splits), int64(c.SplitTags), int64(c.Transfers),
			int64(r.BalancesChecked), int64(r.BalancesMismatched), int64(r.SplitsMismatched),
			int64(r.TransfersOneSided),
			nullableTime(r.Snapshot.TakenAt), nullableNonEmpty(r.Snapshot.Source),
			int64(r.BalancesNeverReconciled), int64(r.InvestmentAccounts),
			int64(r.TransfersPaired), int64(r.TransfersCrossCurrency),
			nil, nil, nil,
			int64(c.Securities), int64(c.Prices), int64(c.InvestmentTransactions), int64(r.SharesChecked),
		})
	}
	return out
}
