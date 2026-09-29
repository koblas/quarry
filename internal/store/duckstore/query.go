package duckstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
)

// Query runs query verbatim against the store, opened read-only, and
// returns at most maxRows rows (every row when maxRows is 0 or less). It
// refuses with store.ErrQueryInterrupted once ctx is done, store.ErrReadOnlyQuery
// for a write, store.ErrExternalAccess for another file, database or extension,
// *store.UnprintableValueError for a value it cannot print, and *store.QueryError
// for any other query error; other open faults are wrapped "run query: ...".
func (s *Store) Query(ctx context.Context, query string, maxRows int) (store.QueryResult, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return store.QueryResult{}, fmt.Errorf("%w: %w", store.ErrQueryInterrupted, err)
		}
		return store.QueryResult{}, fmt.Errorf("run query: %w", err)
	}
	defer func() { _ = db.Close() }()

	table, err := db.QueryTable(ctx, query, maxRows)
	if err != nil {
		return store.QueryResult{}, queryRefusal(ctx, err)
	}
	return queryResult(table), nil
}

// queryRefusal classifies a failed query. A cancelled ctx is checked first:
// the driver's interrupt error reads "context canceled" and would otherwise
// be reported as a query error.
func queryRefusal(ctx context.Context, err error) error {
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
