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
	"time"

	"github.com/koblas/quarry/internal/platform/atomicfile"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
)

// FileName is quarry's store filename inside a Store's directory.
const FileName = "quarry.duckdb"

// FormatVersion is the store format this build of quarry writes and reads.
const FormatVersion = 2

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

// DB is the connection a Store builds one partial file through.
// *duckdb.DB is the production implementation.
type DB interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	AppendRows(ctx context.Context, table string, rows [][]any) error
	CheckpointClose(ctx context.Context) error
	Close() error
}

var _ DB = (*duckdb.DB)(nil)

// ReadDB is the read-only connection a Store answers one read through.
// *duckdb.DB is the production implementation.
type ReadDB interface {
	QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error
	Close() error
}

var _ ReadDB = (*duckdb.DB)(nil)

// Store builds quarry's DuckDB file inside one directory and answers reads
// from it.
type Store struct {
	dir           string
	create        func(ctx context.Context, path string) (DB, error)
	openReadOnly  func(ctx context.Context, path string) (ReadDB, error)
	quarryVersion string
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
	s := &Store{dir: dir, create: createDuckDB, openReadOnly: openDuckDBReadOnly, quarryVersion: develVersion}
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

// openDuckDBReadOnly is New's default read opener; it returns a nil
// interface, never a typed nil, when duckdb.OpenReadOnly fails.
func openDuckDBReadOnly(ctx context.Context, path string) (ReadDB, error) {
	db, err := duckdb.OpenReadOnly(ctx, path)
	if err != nil {
		return nil, err //nolint:wrapcheck // OpenReadOnly already names the path; callers add the operation
	}
	return db, nil
}

// openRead opens the store file read-only. Every read method goes through
// it, and it never creates the file: a missing store is the open's error.
func (s *Store) openRead(ctx context.Context) (ReadDB, error) {
	return s.openReadOnly(ctx, s.Path())
}

// Replace creates the store directory if needed, sweeps aged build leftovers
// and swaps rows into quarry.duckdb, removing any stale quarry.duckdb.wal
// first. On failure this run's own build file is removed and the existing
// store is untouched; a permission fault matches store.ErrStoreNotWritable, disk-full store.ErrDiskFull.
func (s *Store) Replace(ctx context.Context, rows store.Rows) (string, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", buildError(err)
	}
	s.sweepLeftovers()

	finalPath := s.Path()
	partialPath := filepath.Join(s.dir, partialName(time.Now()))

	db, err := s.create(ctx, partialPath)
	if err != nil {
		removePartial(partialPath)
		return "", buildError(err)
	}

	builtAt := time.Now().UTC()
	if err := build(ctx, db, rows, s.quarryVersion, builtAt); err != nil {
		_ = db.Close()
		removePartial(partialPath)
		return "", buildError(err)
	}

	if err := db.CheckpointClose(ctx); err != nil {
		_ = db.Close()
		removePartial(partialPath)
		return "", buildError(err)
	}

	// The last point an interrupt can still keep the previous store.
	if err := ctx.Err(); err != nil {
		removePartial(partialPath)
		return "", buildError(err)
	}

	if err := removeStaleWAL(finalPath); err != nil {
		removePartial(partialPath)
		return "", buildError(err)
	}

	if err := os.Rename(partialPath, finalPath); err != nil {
		removePartial(partialPath)
		return "", buildError(err)
	}
	atomicfile.SyncDir(s.dir)

	return finalPath, nil
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

// build creates quarry's schema and views in db and bulk-loads every table
// in rows, then store_info last: a store carrying it is complete.
func build(ctx context.Context, db DB, rows store.Rows, quarryVersion string, builtAt time.Time) error {
	if _, err := db.Exec(ctx, schemaDDL+accountBalancesViewDDL()); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

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
	if err := appendTable(ctx, db, "import_runs", importRunRows(rows.ImportRuns)); err != nil {
		return err
	}
	return appendTable(ctx, db, "store_info", [][]any{{int32(FormatVersion), quarryVersion, builtAt}})
}

// appendTable bulk-loads rows into table, naming the table on failure.
func appendTable(ctx context.Context, db DB, table string, rows [][]any) error {
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

func nullableNonEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func accountRows(accounts []store.Account) [][]any {
	out := make([][]any, len(accounts))
	for i, a := range accounts {
		out[i] = []any{a.ID, a.SourceID, a.Name, a.Type, a.Currency, nullableStr(a.Institution), a.Closed, a.Active}
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
			amount, t.Currency, t.Status, nullableStr(t.ChequeNumber),
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
		out[i] = []any{tr.ID, tr.FromSplitID, nullableStr(tr.ToSplitID), tr.CrossCurrency}
	}
	return out
}

func importRunRows(runs []store.ImportRun) [][]any {
	out := make([][]any, len(runs))
	for i, r := range runs {
		c := r.Counts
		out[i] = []any{
			r.ID, r.StartedAt, r.FinishedAt, r.Snapshot.Path, r.Snapshot.SHA256, r.Snapshot.SchemaFingerprint,
			int64(c.Accounts), int64(c.Categories), int64(c.Payees), int64(c.Tags),
			int64(c.Transactions), int64(c.Splits), int64(c.SplitTags), int64(c.Transfers),
			int64(r.BalancesChecked), int64(r.BalancesMismatched), int64(r.SplitsMismatched),
			int64(r.TransfersOneSided), int64(r.InvestmentTransactionsNotImported),
			nullableTime(r.Snapshot.TakenAt), nullableNonEmpty(r.Snapshot.Source),
			int64(r.BalancesNeverReconciled), int64(r.InvestmentAccounts),
			int64(r.TransfersPaired), int64(r.TransfersCrossCurrency),
		}
	}
	return out
}
