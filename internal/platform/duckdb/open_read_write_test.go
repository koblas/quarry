package duckdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_open_read_write_edits_an_existing_database(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	created, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)
	_, err = created.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")
	require.NoError(t, err)
	require.NoError(t, created.CheckpointClose(t.Context()))
	editor, err := duckdb.OpenReadWrite(t.Context(), path)
	require.NoError(t, err)
	_, err = editor.Exec(t.Context(), "INSERT INTO t VALUES (7)")
	require.NoError(t, err)
	require.NoError(t, editor.CheckpointClose(t.Context()))

	reader, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	var got int
	err = reader.QueryRows(t.Context(), "SELECT v FROM t", nil, func(scan func(dest ...any) error) error { return scan(&got) })
	require.NoError(t, err)
	assert.Equal(t, 7, got)
}

func Test_open_read_write_on_a_non_duckdb_file_fails_naming_the_path(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	require.NoError(t, os.WriteFile(path, []byte("not a database\n"), 0o600))

	_, err := duckdb.OpenReadWrite(t.Context(), path)

	require.Error(t, err)
	assert.True(t, duckdb.IsNotDatabase(err), err.Error())
	assert.ErrorContains(t, err, "open "+path+" read-write: ")
}
