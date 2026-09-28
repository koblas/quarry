package duckdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End to end: AppendRows writes negative/zero/boundary DECIMAL(18,2) values,
// CheckpointClose removes the .wal, and a fresh read-only connection reads every value back exactly.
func Test_bulk_inserted_decimals_read_back_exactly_after_checkpoint_close(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.duckdb")
	db, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)

	_, err = db.Exec(t.Context(), "CREATE TABLE t (id INTEGER, a DECIMAL(18,2))")
	require.NoError(t, err)

	negative, err := duckdb.Decimal(-120417, 18, 2)
	require.NoError(t, err)
	zero, err := duckdb.Decimal(0, 18, 2)
	require.NoError(t, err)
	boundary, err := duckdb.Decimal(999999999999999999, 18, 2)
	require.NoError(t, err)

	err = db.AppendRows(t.Context(), "t", [][]any{
		{int32(1), negative},
		{int32(2), zero},
		{int32(3), boundary},
	})
	require.NoError(t, err)

	_, err = os.Stat(path + ".wal")
	require.NoError(t, err, "a .wal file must exist before CheckpointClose")

	require.NoError(t, db.CheckpointClose(t.Context()))

	_, err = os.Stat(path + ".wal")
	assert.ErrorIs(t, err, os.ErrNotExist)

	reader, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	got := map[int32]string{}
	err = reader.QueryRows(t.Context(), "SELECT id, CAST(a AS VARCHAR) FROM t ORDER BY id", nil,
		func(scan func(dest ...any) error) error {
			var id int32
			var value string
			if err := scan(&id, &value); err != nil {
				return err
			}
			got[id] = value
			return nil
		})
	require.NoError(t, err)

	assert.Equal(t, map[int32]string{
		1: "-1204.17",
		2: "0.00",
		3: "9999999999999999.99",
	}, got)
}
