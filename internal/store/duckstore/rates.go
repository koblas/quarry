package duckstore

import (
	"context"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
)

// rateWidth and rateScale match schemaDDL's fx_rates.usd_cad DECIMAL(10,6).
const rateWidth, rateScale = 10, 6

// finishBuild fetches the rates the store needs, appends them to fx_rates,
// then appends store_info last: a file carrying it is complete. It fails only
// when ctx ended or a row cannot be written; a fetch that fell short is not a
// failure and keeps whatever rates it returned.
func (s *Store) finishBuild(ctx context.Context, db DB, rows store.Rows, builtAt time.Time) error {
	refresh, err := s.refreshRates(ctx, rows.Transactions)
	if err != nil {
		return err
	}
	fxRows, err := rateRows(refresh.Rates)
	if err != nil {
		return err
	}
	if err := appendTable(ctx, db, "fx_rates", fxRows); err != nil {
		return err
	}
	return appendTable(ctx, db, "store_info", [][]any{{int32(FormatVersion), s.quarryVersion, builtAt}})
}

// refreshRates asks the rates source for every date from the earliest
// transaction to today; it returns an empty refresh when there is no source.
func (s *Store) refreshRates(ctx context.Context, transactions []store.Transaction) (store.RatesRefresh, error) {
	if s.rates == nil {
		return store.RatesRefresh{}, nil
	}
	refresh, err := s.rates.Refresh(ctx, store.RatesRequest{Need: needSpan(transactions, time.Now())})
	if err != nil {
		return store.RatesRefresh{}, fmt.Errorf("fetch exchange rates: %w", err)
	}
	return refresh, nil
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
