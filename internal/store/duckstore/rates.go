package duckstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
)

// rateWidth and rateScale match schemaDDL's fx_rates.usd_cad DECIMAL(10,6).
const rateWidth, rateScale = 10, 6

// storedRatesQuery reads the first and last date in fx_rates; both are NULL when it is empty.
const storedRatesQuery = `SELECT min(date), max(date) FROM fx_rates`

// recordRatesQuery stamps the run this build added, the highest id, with how far its rates reach.
const recordRatesQuery = `UPDATE import_runs SET rates_checked_from = CAST(? AS DATE), rates_last = CAST(? AS DATE)
WHERE id = (SELECT max(id) FROM import_runs)`

// finishBuild appends the fetched rates, records them on the new import run, then store_info last. It fails only
// when ctx ended or a row cannot be written; a fetch that fell short keeps whatever rates it returned.
func (s *Store) finishBuild(ctx context.Context, db DB, rows store.Rows, builtAt time.Time) (store.RatesSummary, error) {
	need, refresh, err := s.refreshRates(ctx, rows.Transactions)
	if err != nil {
		return store.RatesSummary{}, err
	}
	fxRows, err := rateRows(refresh.Rates)
	if err != nil {
		return store.RatesSummary{}, err
	}
	if err := appendTable(ctx, db, "fx_rates", fxRows); err != nil {
		return store.RatesSummary{}, err
	}
	summary, err := storedRates(ctx, db)
	if err != nil {
		return store.RatesSummary{}, err
	}
	summary.Added, summary.FetchError = refresh.Added, refresh.FetchError
	if err := recordRates(ctx, db, askedFrom(need, refresh), summary.Last); err != nil {
		return store.RatesSummary{}, err
	}
	// A file carrying store_info is complete, so it goes last.
	return summary, appendTable(ctx, db, "store_info", [][]any{{int32(FormatVersion), s.quarryVersion, builtAt}})
}

// refreshRates asks the rates source for every date from the earliest transaction to today. It returns the
// span it asked for, empty when there is no source, and an empty refresh.
func (s *Store) refreshRates(ctx context.Context, transactions []store.Transaction) (store.DateSpan, store.RatesRefresh, error) {
	if s.rates == nil {
		return store.DateSpan{}, store.RatesRefresh{}, nil
	}
	need := needSpan(transactions, time.Now())
	refresh, err := s.rates.Refresh(ctx, store.RatesRequest{Need: need})
	if err != nil {
		return store.DateSpan{}, store.RatesRefresh{}, fmt.Errorf("fetch exchange rates: %w", err)
	}
	return need, refresh, nil
}

// askedFrom is the first date of the span the source answered, or the zero time when it asked nothing
// or failed: an answer, even an empty one, means the dates before it have no rate and need no new request.
func askedFrom(need store.DateSpan, refresh store.RatesRefresh) time.Time {
	if refresh.FetchError != "" {
		return time.Time{}
	}
	return need.First
}

// storedRates reads the first and last date in fx_rates, zero when it is empty.
func storedRates(ctx context.Context, db DB) (store.RatesSummary, error) {
	var first, last sql.NullTime
	err := db.QueryRows(ctx, storedRatesQuery, nil, func(scan func(dest ...any) error) error {
		return scan(&first, &last)
	})
	if err != nil {
		return store.RatesSummary{}, fmt.Errorf("read stored exchange rates: %w", err)
	}
	return store.RatesSummary{First: first.Time, Last: last.Time}, nil
}

// recordRates stamps the build's import run with the dates it asked from and its last stored rate; a zero time is NULL.
func recordRates(ctx context.Context, db DB, checkedFrom, last time.Time) error {
	if _, err := db.Exec(ctx, recordRatesQuery, nullIfZero(checkedFrom), nullIfZero(last)); err != nil {
		return fmt.Errorf("record exchange rates: %w", err)
	}
	return nil
}

// nullIfZero is t, or nil for the zero time.
func nullIfZero(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// needSpan is the dates from the earliest transaction to now's local calendar
// date (now carries the local zone), or empty when there are no transactions.
func needSpan(transactions []store.Transaction, now time.Time) store.DateSpan {
	if len(transactions) == 0 {
		return store.DateSpan{}
	}
	first := transactions[0].Date
	for _, t := range transactions[1:] {
		if t.Date.Before(first) {
			first = t.Date
		}
	}
	year, month, day := now.Date()
	return store.DateSpan{First: first, Last: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

func rateRows(rates []store.Rate) ([][]any, error) {
	out := make([][]any, len(rates))
	for i, r := range rates {
		rate, err := duckdb.Decimal(int64(r.USDCAD), rateWidth, rateScale)
		if err != nil {
			return nil, fmt.Errorf("rate for %s: %w", r.Date.Format(time.DateOnly), err)
		}
		out[i] = []any{r.Date, rate, r.Series}
	}
	return out, nil
}
