package duckdb_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newOpenDatabase(t *testing.T) (*duckdb.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	db, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func Test_exec_fails_on_a_syntax_error(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	_, err := db.Exec(t.Context(), "NOT VALID SQL")

	require.Error(t, err)
}

func Test_query_rows_fails_on_a_query_error(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	err := db.QueryRows(t.Context(), "SELECT * FROM missing_table", nil,
		func(scan func(dest ...any) error) error { return nil })

	require.Error(t, err)
}

func Test_query_rows_fails_on_a_scan_type_error(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	_, err := db.Exec(t.Context(), "CREATE TABLE t (v VARCHAR)")
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), "INSERT INTO t VALUES ('not a number')")
	require.NoError(t, err)

	err = db.QueryRows(t.Context(), "SELECT v FROM t", nil,
		func(scan func(dest ...any) error) error {
			var n int
			return scan(&n)
		})

	require.Error(t, err)
}

func Test_query_rows_stops_once_the_callback_errors(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	_, err := db.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")
	require.NoError(t, err)
	require.NoError(t, db.AppendRows(t.Context(), "t", [][]any{{int32(1)}, {int32(2)}}))
	boom := assert.AnError

	calls := 0
	err = db.QueryRows(t.Context(), "SELECT v FROM t ORDER BY v", nil,
		func(scan func(dest ...any) error) error {
			calls++
			return boom
		})

	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, calls)
}

func Test_open_read_only_fails_on_a_missing_path(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.duckdb")

	_, err := duckdb.OpenReadOnly(t.Context(), path)

	require.Error(t, err)
}

func Test_open_read_only_refuses_writes(t *testing.T) {
	t.Parallel()
	writer, path := newOpenDatabase(t)
	require.NoError(t, writer.Close())

	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")

	require.Error(t, err)
}

// sql.Open's own eager open of the existing file already succeeded, so a
// context cancelled before OpenReadOnly runs lands specifically on
// PingContext, not on sql.Open.
func Test_open_read_only_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	writer, path := newOpenDatabase(t)
	require.NoError(t, writer.Close())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := duckdb.OpenReadOnly(ctx, path)

	require.ErrorIs(t, err, context.Canceled)
}

// Enough rows to span several result chunks; the callback cancels ctx and returns nil,
// so the cancellation surfaces through rows.Err(), not the callback's own error.
// Not parallel: cancellation must win the race against iteration finishing
// on its own, and CPU contention from sibling parallel tests changes which
// side wins.
func Test_query_rows_fails_when_the_context_is_cancelled_mid_iteration(t *testing.T) {
	db, _ := newOpenDatabase(t)
	_, err := db.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")
	require.NoError(t, err)
	rows := make([][]any, 20000)
	for i := range rows {
		rows[i] = []any{int32(i)}
	}
	require.NoError(t, db.AppendRows(t.Context(), "t", rows))
	ctx, cancel := context.WithCancel(t.Context())

	calls := 0
	err = db.QueryRows(ctx, "SELECT v FROM t ORDER BY v", nil,
		func(scan func(dest ...any) error) error {
			calls++
			cancel()
			return nil
		})

	require.Error(t, err)
	assert.Less(t, calls, len(rows), "cancellation should have stopped iteration before the last row")
}

// The Appender itself refuses a table that does not exist.
func Test_append_rows_fails_when_the_table_does_not_exist(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	err := db.AppendRows(t.Context(), "does_not_exist", [][]any{{int32(1)}})

	require.Error(t, err)
}

// A cancelled context makes CHECKPOINT itself fail, distinct from the
// connection-close or no-WAL steps that follow it.
func Test_checkpoint_close_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := db.CheckpointClose(ctx)

	require.ErrorIs(t, err, context.Canceled)
}
