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
	t.Parallel()
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY, v VARCHAR)")

	err := db.AppendRows(t.Context(), "t", [][]any{
		{int32(1), "a"},
		{int32(1), "b"},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "constraint")
}

// The pool's own Conn(ctx) acquisition checks ctx before this package's own
// per-row check ever runs, for a context already cancelled beforehand.
func Test_append_rows_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY)")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := db.AppendRows(ctx, "t", [][]any{{int32(1)}})

	require.ErrorIs(t, err, context.Canceled)
}

func Test_append_rows_reports_a_wrong_column_count(t *testing.T) {
	t.Parallel()
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY, v VARCHAR)")

	err := db.AppendRows(t.Context(), "t", [][]any{
		{int32(1)},
	})

	require.Error(t, err)
}

// cancelAfterNErrCalls reports Err() as nil for its first n calls, then as
// context.Canceled — lands AppendRows' per-row check on a chosen row deterministically.
type cancelAfterNErrCalls struct {
	context.Context
	n     int
	calls int
}

func (c *cancelAfterNErrCalls) Err() error {
	c.calls++
	if c.calls > c.n {
		return context.Canceled
	}
	return nil
}

// Three rows exist; Err() cancels after the first, so a row count of
// exactly 1 (not just an error) proves the loop stopped there.
func Test_append_rows_stops_when_the_context_is_cancelled(t *testing.T) {
	t.Parallel()
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY)")
	ctx := &cancelAfterNErrCalls{Context: t.Context(), n: 1}

	err := db.AppendRows(ctx, "t", [][]any{
		{int32(1)},
		{int32(2)},
		{int32(3)},
	})
	require.ErrorIs(t, err, context.Canceled)

	var count int64
	require.NoError(t, db.QueryRows(t.Context(), "SELECT count(*) FROM t", nil,
		func(scan func(dest ...any) error) error { return scan(&count) }))
	assert.Equal(t, int64(1), count)
}
