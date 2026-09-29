// White-box: checkNoWAL is unexported. A real CHECKPOINT always removes the
// .wal file itself, so the only way to exercise "a .wal remains" is to
// plant one beside an already-closed database and call the check directly.
package duckdb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_no_wal_check_fails_when_a_wal_file_remains(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	require.NoError(t, os.WriteFile(path+".wal", nil, 0o600))

	err := checkNoWAL(path)

	require.ErrorIs(t, err, ErrWALRemains)
}

func Test_no_wal_check_passes_when_no_wal_file_exists(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")

	err := checkNoWAL(path)

	assert.NoError(t, err)
}

// An unreadable parent directory makes os.Stat fail with EACCES rather than
// ErrNotExist — that error must pass through, not be swallowed as "no WAL".
func Test_no_wal_check_fails_when_the_stat_itself_errors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	path := filepath.Join(dir, "data.duckdb")

	err := checkNoWAL(path)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrWALRemains)
}
