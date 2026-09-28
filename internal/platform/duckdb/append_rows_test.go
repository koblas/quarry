package duckdb_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTable(t *testing.T, ddl string) *duckdb.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	db, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(t.Context(), ddl)
	require.NoError(t, err)
	return db
}

func Test_append_rows_reports_a_duplicate_primary_key(t *testing.T) {
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY, v VARCHAR)")

	err := db.AppendRows(t.Context(), "t", [][]any{
		{int32(1), "a"},
		{int32(1), "b"},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "constraint")
}

func Test_append_rows_reports_a_wrong_column_count(t *testing.T) {
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY, v VARCHAR)")

	err := db.AppendRows(t.Context(), "t", [][]any{
		{int32(1)},
	})

	require.Error(t, err)
}

// Two rows exist; the context is cancelled before the first is appended, so
// a row count of zero (not just an error) proves the loop stopped rather
// than merely reporting a late failure after both were sent.
func Test_append_rows_stops_when_the_context_is_cancelled(t *testing.T) {
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY)")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := db.AppendRows(ctx, "t", [][]any{
		{int32(1)},
		{int32(2)},
	})
	require.ErrorIs(t, err, context.Canceled)

	var count int64
	require.NoError(t, db.QueryRows(t.Context(), "SELECT count(*) FROM t", nil,
		func(scan func(dest ...any) error) error { return scan(&count) }))
	assert.Zero(t, count)
}
