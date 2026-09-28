package duckdb_test

import (
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
	db, _ := newOpenDatabase(t)

	_, err := db.Exec(t.Context(), "NOT VALID SQL")

	require.Error(t, err)
}

func Test_query_rows_fails_on_a_query_error(t *testing.T) {
	db, _ := newOpenDatabase(t)

	err := db.QueryRows(t.Context(), "SELECT * FROM missing_table", nil,
		func(scan func(dest ...any) error) error { return nil })

	require.Error(t, err)
}

func Test_query_rows_fails_on_a_scan_type_error(t *testing.T) {
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
	path := filepath.Join(t.TempDir(), "missing.duckdb")

	_, err := duckdb.OpenReadOnly(t.Context(), path)

	require.Error(t, err)
}

func Test_open_read_only_refuses_writes(t *testing.T) {
	writer, path := newOpenDatabase(t)
	require.NoError(t, writer.Close())

	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")

	require.Error(t, err)
}
