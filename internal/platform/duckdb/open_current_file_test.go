package duckdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_open_read_only_reads_the_file_now_at_the_path_while_an_earlier_open_is_still_running(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeStoreHolding(t, filepath.Join(dir, "data.duckdb"), 1)
	replacement := writeStoreHolding(t, filepath.Join(dir, "replacement.duckdb"), 2)
	earlier, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = earlier.Close() })
	require.NoError(t, os.Rename(replacement, path))

	current, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = current.Close() })

	assert.Equal(t, int32(2), onlyValue(t, current))
}

// writeStoreHolding creates a DuckDB file at path whose table t holds the one value v.
func writeStoreHolding(t *testing.T, path string, v int32) string {
	t.Helper()
	db, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")
	require.NoError(t, err)
	require.NoError(t, db.AppendRows(t.Context(), "t", [][]any{{v}}))
	require.NoError(t, db.CheckpointClose(t.Context()))
	return path
}

// onlyValue is the one value of db's table t.
func onlyValue(t *testing.T, db *duckdb.DB) int32 {
	t.Helper()
	var v int32
	err := db.QueryRows(t.Context(), "SELECT v FROM t", nil, func(scan func(dest ...any) error) error { return scan(&v) })
	require.NoError(t, err)
	return v
}
