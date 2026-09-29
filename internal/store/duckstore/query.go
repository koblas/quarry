package duckstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
)

// Query runs query verbatim against the store, opened read-only, and
// returns at most maxRows rows (every row when maxRows is 0 or less). Open
// and query faults are wrapped once as "run query: ..." around the driver's
// own error; a value quarry cannot print is a *store.UnprintableValueError.
func (s *Store) Query(ctx context.Context, query string, maxRows int) (store.QueryResult, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.QueryResult{}, fmt.Errorf("run query: %w", err)
	}
	defer func() { _ = db.Close() }()

	table, err := db.QueryTable(ctx, query, maxRows)
	if unprintable, ok := errors.AsType[*duckdb.UnprintableValueError](err); ok {
		return store.QueryResult{}, &store.UnprintableValueError{Column: unprintable.Column, Type: unprintable.Type}
	}
	if err != nil {
		return store.QueryResult{}, fmt.Errorf("run query: %w", err)
	}
	return queryResult(table), nil
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
