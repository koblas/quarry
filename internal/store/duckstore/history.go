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

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// history is what Replace carries from the store it replaces: its import_runs rows and highest id, its findings,
// its exchange rates in date order, and the date those rates were last asked from.
type history struct {
	rows            [][]any
	maxID           int64
	findings        []carriedFinding
	findingsCarried bool
	findingsFault   *store.OpenError // why a findings table that exists could not be read
	rates           []store.Rate
	ratesFault      *store.OpenError // why an fx_rates table that exists could not be read
	ratesFloor      time.Time        // the newest run's rates_checked_from; zero when it is NULL, or the rates were lost
	unreadable      bool             // true iff the store could not be opened
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
	"rates_checked_from", "rates_last", "rates_fetch_error",
}

// runColumnsQuery lists the columns of the store's import_runs table, none when it has no such table.
const runColumnsQuery = `SELECT column_name FROM duckdb_columns()
WHERE database_name = current_database() AND schema_name = 'main' AND table_name = 'import_runs'`

// findingColumnsQuery lists the columns of the store's findings table, none when it has no such table.
const findingColumnsQuery = `SELECT column_name FROM duckdb_columns()
WHERE database_name = current_database() AND schema_name = 'main' AND table_name = 'findings'`

// rateColumnsQuery lists the columns of the store's fx_rates table, none when it has no such table.
const rateColumnsQuery = `SELECT column_name FROM duckdb_columns()
WHERE database_name = current_database() AND schema_name = 'main' AND table_name = 'fx_rates'`

// ratesQuery reads fx_rates with each rate as exact millionths, never a float.
const ratesQuery = `SELECT date, CAST(usd_cad * 1000000 AS BIGINT), series FROM fx_rates ORDER BY date`

// The phrases a sync prints for a history fault found after the store opened, each naming the previous store as "it".
const (
	reasonRunsRepeatID   = "its import_runs table repeats an id"
	reasonRunsIDTooLarge = "its import_runs table has an id too large to follow"
	reasonRunsMissing    = "it has no import_runs table"
	reasonRunsIncomplete = "its import_runs table is incomplete"

	reasonFindingsRepeatID   = "its findings table repeats an id"
	reasonFindingsIncomplete = "its findings table is incomplete"

	reasonRatesRepeatDate = "its fx_rates table repeats a date"
	reasonRatesImpossible = "its fx_rates table holds an impossible rate"
	reasonRatesUnknown    = "its fx_rates table names an unknown series"
	reasonRatesIncomplete = "its fx_rates table is incomplete"
)

var (
	errRunsMissing  = errors.New("import_runs table is absent")
	errRunsRepeatID = errors.New("import_runs holds an id twice")
	errRunsIDLimit  = errors.New("import_runs holds the largest id, which no run can follow")

	errFindingsRepeatID = errors.New("findings holds an id twice")

	errRatesRepeatDate = errors.New("fx_rates holds a date twice")
	errRatesImpossible = errors.New("fx_rates holds a rate the column cannot hold")
	errRatesUnknown    = errors.New("fx_rates holds a series quarry does not read")
)

// readHistory reads the import_runs and findings of the store at s.Path() and closes it before returning.
// An absent store has no history; an unreadable one has none and a fault; a findings fault rides on the history.
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
		return history{unreadable: true}, fault
	}
	defer func() { _ = db.Close() }()

	var fault *store.OpenError
	carried, err := readRuns(ctx, db)
	if err != nil {
		carried, fault = history{}, historyFault(path, err)
	}
	findings, present, err := readFindings(ctx, db)
	switch {
	case err != nil:
		carried.findingsFault = findingsFault(path, err)
	case present:
		carried.findings, carried.findingsCarried = findings, true
	}
	rates, present, err := readRates(ctx, db)
	switch {
	case err != nil:
		carried.ratesFault, carried.ratesFloor = ratesFault(path, err), time.Time{}
	case present:
		carried.rates = rates
	default:
		carried.ratesFloor = time.Time{} // a floor claims dates whose rates are gone
	}
	return carried, fault
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

// findingsFault is the fault of a findings read that failed after the store opened: an
// OpenFaultOther whose Reason is a fixed phrase, never driver text.
func findingsFault(path string, err error) *store.OpenError {
	reason := reasonFindingsIncomplete
	if errors.Is(err, errFindingsRepeatID) {
		reason = reasonFindingsRepeatID
	}
	return &store.OpenError{Fault: store.OpenFaultOther, Path: path, Reason: reason, Err: err}
}

// ratesFault is the fault of an fx_rates read that failed after the store opened: an
// OpenFaultOther whose Reason is a fixed phrase, never driver text.
func ratesFault(path string, err error) *store.OpenError {
	reason := reasonRatesIncomplete
	switch {
	case errors.Is(err, errRatesRepeatDate):
		reason = reasonRatesRepeatDate
	case errors.Is(err, errRatesImpossible):
		reason = reasonRatesImpossible
	case errors.Is(err, errRatesUnknown):
		reason = reasonRatesUnknown
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
			carried.ratesFloor = r.ratesCheckedFrom.Time // rows arrive by id, so the last is the newest run's; NULL is the zero time
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

// carriedFinding is one findings row as read, its timestamps kept as stored so a carried finding keeps them.
type carriedFinding struct {
	id, typ      string
	firstFoundAt time.Time
	fixedAt      sql.NullTime
}

// readFindings reads every findings row through db; present is false, without a fault, when the store has no findings table.
// It fails with errFindingsRepeatID (an id twice), else with the read's own fault.
func readFindings(ctx context.Context, db ReadDB) ([]carriedFinding, bool, error) {
	present := false
	err := db.QueryRows(ctx, findingColumnsQuery, nil, func(scan func(dest ...any) error) error {
		var column string
		present = true
		return scan(&column)
	})
	if err != nil {
		return nil, false, fmt.Errorf("read findings columns: %w", err)
	}
	if !present {
		return nil, false, nil
	}
	var found []carriedFinding
	seen := map[string]bool{}
	err = db.QueryRows(ctx, "SELECT id, type, first_found_at, fixed_at FROM findings ORDER BY id", nil,
		func(scan func(dest ...any) error) error {
			var f carriedFinding
			if err := scan(&f.id, &f.typ, &f.firstFoundAt, &f.fixedAt); err != nil {
				return err
			}
			if seen[f.id] {
				return errFindingsRepeatID
			}
			seen[f.id] = true
			found = append(found, f)
			return nil
		})
	if err != nil {
		return nil, false, fmt.Errorf("read findings: %w", err)
	}
	return found, true, nil
}

// readRates reads every fx_rates row through db in date order; present is false, without a fault, when there is no table.
// It fails with errRatesRepeatDate, errRatesImpossible or errRatesUnknown for a row the new table would refuse, else with the read's own fault.
func readRates(ctx context.Context, db ReadDB) ([]store.Rate, bool, error) {
	present := false
	err := db.QueryRows(ctx, rateColumnsQuery, nil, func(scan func(dest ...any) error) error {
		var column string
		present = true
		return scan(&column)
	})
	if err != nil {
		return nil, false, fmt.Errorf("read fx_rates columns: %w", err)
	}
	if !present {
		return nil, false, nil
	}
	var rates []store.Rate
	seen := map[time.Time]bool{}
	err = db.QueryRows(ctx, ratesQuery, nil, func(scan func(dest ...any) error) error {
		var r store.Rate
		var millionths int64
		if err := scan(&r.Date, &millionths, &r.Series); err != nil {
			return err
		}
		r.USDCAD = money.Rate(millionths)
		switch {
		case seen[r.Date]:
			return errRatesRepeatDate
		case r.USDCAD <= 0 || r.USDCAD > maxStoredRate:
			return errRatesImpossible
		case r.Series != store.SeriesCurrent && r.Series != store.SeriesLegacy:
			return errRatesUnknown
		}
		seen[r.Date] = true
		rates = append(rates, r)
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("read fx_rates: %w", err)
	}
	return rates, true, nil
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
	ratesCheckedFrom, ratesLast                                  sql.NullTime
	ratesFetchError                                              sql.NullString
}

// targets are the scan destinations, one per column, in SELECT order.
func (r *carriedRun) targets() []any {
	out := []any{&r.id, &r.startedAt, &r.finishedAt, &r.snapshotPath, &r.snapshotSHA256, &r.schemaFingerprint}
	for i := range r.counts {
		out = append(out, &r.counts[i])
	}
	return append(out, &r.takenAt, &r.source, &r.neverReconciled, &r.investmentAccounts, &r.paired, &r.crossCurrencies,
		&r.ratesCheckedFrom, &r.ratesLast, &r.ratesFetchError)
}

// values are the row's cells as importRunRows appends them.
func (r *carriedRun) values() []any {
	out := []any{r.id, r.startedAt, r.finishedAt, r.snapshotPath, r.snapshotSHA256, r.schemaFingerprint}
	for _, count := range r.counts {
		out = append(out, count)
	}
	return append(out, nullValue(r.takenAt.Valid, r.takenAt.Time), nullValue(r.source.Valid, r.source.String),
		nullValue(r.neverReconciled.Valid, r.neverReconciled.Int64), nullValue(r.investmentAccounts.Valid, r.investmentAccounts.Int64),
		nullValue(r.paired.Valid, r.paired.Int64), nullValue(r.crossCurrencies.Valid, r.crossCurrencies.Int64),
		nullValue(r.ratesCheckedFrom.Valid, r.ratesCheckedFrom.Time), nullValue(r.ratesLast.Valid, r.ratesLast.Time),
		nullValue(r.ratesFetchError.Valid, r.ratesFetchError.String))
}

// nullValue is v when valid, else nil.
func nullValue[T any](valid bool, v T) any {
	if !valid {
		return nil
	}
	return v
}
