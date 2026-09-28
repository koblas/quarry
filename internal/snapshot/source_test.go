package snapshot_test

import (
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_sqliteSource_backup_wraps_a_failure_from_the_underlying_backup(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	src := snapshot.NewSQLiteSource(0)
	require.NoError(t, src.Open(t.Context(), bundle.DataPath))
	t.Cleanup(func() { _ = src.Close() })
	// The backup API needs the destination to already exist.
	destPath := filepath.Join(t.TempDir(), "missing-dir", "dest")

	err := src.Backup(t.Context(), destPath)

	require.Error(t, err)
}

func Test_sqliteSource_close_is_a_no_op_before_open(t *testing.T) {
	src := snapshot.NewSQLiteSource(0)

	err := src.Close()

	assert.NoError(t, err)
}

func Test_sqliteSource_open_fails_on_a_missing_file(t *testing.T) {
	src := snapshot.NewSQLiteSource(0)

	err := src.Open(t.Context(), filepath.Join(t.TempDir(), "missing", "data"))

	require.Error(t, err)
}
