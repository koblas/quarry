package duckstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
)

// Query runs query verbatim against the store, opened read-only, returning at
// most maxRows rows (all when maxRows <= 0). It refuses with store.ErrQueryInterrupted,
// store.ErrReadOnlyQuery, store.ErrExternalAccess, *store.UnprintableValueError,
// *store.QueryError, or, for a store it cannot open, *store.OpenError.
func (s *Store) Query(ctx context.Context, query string, maxRows int) (store.QueryResult, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return store.QueryResult{}, fmt.Errorf("%w: %w", store.ErrQueryInterrupted, err)
		}
		return store.QueryResult{}, err
	}
	defer func() { _ = db.Close() }()

	table, err := db.QueryTable(ctx, query, maxRows)
	if err != nil {
		return store.QueryResult{}, queryRefusal(ctx, err)
	}
	return queryResult(table), nil
}

// queryRefusal classifies a failed query as one of Query's refusals.
func queryRefusal(ctx context.Context, err error) error {
	// First: the driver's interrupt error reads "context canceled", which would pass as a query error.
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", store.ErrQueryInterrupted, err)
	}
	if unprintable, ok := errors.AsType[*duckdb.UnprintableValueError](err); ok {
		return &store.UnprintableValueError{Column: unprintable.Column, Type: unprintable.Type}
	}
	if duckdb.IsReadOnlyViolation(err) {
		return fmt.Errorf("%w: %w", store.ErrReadOnlyQuery, err)
	}
	if duckdb.IsAccessDisabled(err) {
		return fmt.Errorf("%w: %w", store.ErrExternalAccess, err)
	}
	reason, _, _ := strings.Cut(err.Error(), "\n")
	return &store.QueryError{Reason: reason}
}

// queryResult converts a driver-package table into the store's own result shape.
func queryResult(table duckdb.Table) store.QueryResult {
	result := store.QueryResult{
		Columns: make([]store.QueryColumn, len(table.Columns)),
		Rows:    make([][]store.QueryValue, len(table.Rows)),
	}
	for i, c := range table.Columns {
		result.Columns[i] = store.QueryColumn{Name: c.Name, Type: c.Type}
	}
	for i, row := range table.Rows {
		values := make([]store.QueryValue, len(row))
		for j, v := range row {
			values[j] = store.QueryValue{Null: v.Null, Text: v.Text, Native: v.Native}
		}
		result.Rows[i] = values
	}
	return result
}
