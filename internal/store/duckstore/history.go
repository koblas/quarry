package duckstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// history is the import_runs rows of the store Replace is about to replace,
// in importRunRows's column order, and the highest id among them.
type history struct {
	rows  [][]any
	maxID int64
}

// requiredRunColumns are the import_runs columns every store format has, in importRunRows's order.
var requiredRunColumns = []string{
	"id", "started_at", "finished_at", "snapshot_path", "snapshot_sha256", "schema_fingerprint",
	"accounts_rows", "categories_rows", "payees_rows", "tags_rows", "transactions_rows", "splits_rows", "split_tags_rows", "transfers_rows",
	"balances_checked", "balances_mismatched", "splits_mismatched", "transfers_one_sided", "investment_transactions_not_imported",
}

// optionalRunColumns are the columns an older store format lacks; history carries NULL for each it does not have.
var optionalRunColumns = []string{
	"snapshot_taken_at", "source_path", "balances_never_reconciled", "investment_accounts", "transfers_paired", "transfers_cross_currency",
}

// runColumnsQuery lists the columns of the store's import_runs table, none when it has no such table.
const runColumnsQuery = `SELECT column_name FROM duckdb_columns()
WHERE database_name = current_database() AND schema_name = 'main' AND table_name = 'import_runs'`

// The phrases a sync prints for a history fault found after the store opened, each naming the previous store as "it".
const (
	reasonRunsRepeatID   = "its import_runs table repeats an id"
	reasonRunsIDTooLarge = "its import_runs table has an id too large to follow"
	reasonRunsMissing    = "it has no import_runs table"
	reasonRunsIncomplete = "its import_runs table is incomplete"
)

var (
	errRunsMissing  = errors.New("import_runs table is absent")
	errRunsRepeatID = errors.New("import_runs holds an id twice")
	errRunsIDLimit  = errors.New("import_runs holds the largest id, which no run can follow")
)

// readHistory reads the import_runs of the store at s.Path() and closes it before returning.
// An absent store has no history and no fault; an unreadable one has no history and its fault.
func (s *Store) readHistory(ctx context.Context) (history, *store.OpenError) {
	path := s.Path()
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return history{}, nil
	}
	db, err := s.openReadOnly(ctx, path)
	if err != nil {
		fault := openFault(path, err)
		if fault.Fault == store.OpenFaultMissing {
			return history{}, nil
		}
		return history{}, fault
	}
	defer func() { _ = db.Close() }()

	carried, err := readRuns(ctx, db)
	if err != nil {
		return history{}, historyFault(path, err)
	}
	return carried, nil
}

// historyFault is the fault of a history read that failed after the store opened: an
// OpenFaultOther whose Reason is the phrase UnreadableReason gives, never driver text.
func historyFault(path string, err error) *store.OpenError {
	reason := reasonRunsIncomplete
	switch {
	case errors.Is(err, errRunsMissing):
		reason = reasonRunsMissing
	case errors.Is(err, errRunsRepeatID):
		reason = reasonRunsRepeatID
	case errors.Is(err, errRunsIDLimit):
		reason = reasonRunsIDTooLarge
	}
	return &store.OpenError{Fault: store.OpenFaultOther, Path: path, Reason: reason, Err: err}
}

// readRuns reads every import_runs row through db. It fails with errRunsMissing (no table), errRunsRepeatID
// (an id twice), errRunsIDLimit (an id no run can follow), else with the read's own fault.
func readRuns(ctx context.Context, db ReadDB) (history, error) {
	present := map[string]bool{}
	err := db.QueryRows(ctx, runColumnsQuery, nil, func(scan func(dest ...any) error) error {
		var column string
		if err := scan(&column); err != nil {
			return err
		}
		present[column] = true
		return nil
	})
	if err != nil {
		return history{}, fmt.Errorf("read import_runs columns: %w", err)
	}
	if len(present) == 0 {
		return history{}, errRunsMissing
	}

	columns := slices.Clone(requiredRunColumns)
	for _, column := range optionalRunColumns {
		if !present[column] {
			column = "NULL"
		}
		columns = append(columns, column)
	}

	var carried history
	seen := map[int64]bool{}
	err = db.QueryRows(ctx, "SELECT "+strings.Join(columns, ", ")+" FROM import_runs ORDER BY id", nil,
		func(scan func(dest ...any) error) error {
			var r carriedRun
			if err := scan(r.targets()...); err != nil {
				return err
			}
			if seen[r.id] {
				return errRunsRepeatID
			}
			seen[r.id] = true
			carried.rows = append(carried.rows, r.values())
			carried.maxID = max(carried.maxID, r.id)
			return nil
		})
	if err != nil {
		return history{}, fmt.Errorf("read import_runs: %w", err)
	}
	if carried.maxID == math.MaxInt64 {
		return history{}, errRunsIDLimit
	}
	return carried, nil
}

// carriedRun is one import_runs row as read: the required columns typed, the
// optional ones nullable, so a NULL survives the round trip.
type carriedRun struct {
	id                                                           int64
	startedAt, finishedAt                                        time.Time
	snapshotPath, snapshotSHA256, schemaFingerprint              string
	counts                                                       [13]int64
	takenAt                                                      sql.NullTime
	source                                                       sql.NullString
	neverReconciled, investmentAccounts, paired, crossCurrencies sql.NullInt64
}

// targets are the scan destinations, one per column, in SELECT order.
func (r *carriedRun) targets() []any {
	out := []any{&r.id, &r.startedAt, &r.finishedAt, &r.snapshotPath, &r.snapshotSHA256, &r.schemaFingerprint}
	for i := range r.counts {
		out = append(out, &r.counts[i])
	}
	return append(out, &r.takenAt, &r.source, &r.neverReconciled, &r.investmentAccounts, &r.paired, &r.crossCurrencies)
}

// values are the row's cells as importRunRows appends them.
func (r *carriedRun) values() []any {
	out := []any{r.id, r.startedAt, r.finishedAt, r.snapshotPath, r.snapshotSHA256, r.schemaFingerprint}
	for _, count := range r.counts {
		out = append(out, count)
	}
	return append(out, nullValue(r.takenAt.Valid, r.takenAt.Time), nullValue(r.source.Valid, r.source.String),
		nullValue(r.neverReconciled.Valid, r.neverReconciled.Int64), nullValue(r.investmentAccounts.Valid, r.investmentAccounts.Int64),
		nullValue(r.paired.Valid, r.paired.Int64), nullValue(r.crossCurrencies.Valid, r.crossCurrencies.Int64))
}

// nullValue is v when valid, else nil.
func nullValue[T any](valid bool, v T) any {
	if !valid {
		return nil
	}
	return v
}
