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
	path := filepath.Join(t.TempDir(), "data.duckdb")
	require.NoError(t, os.WriteFile(path+".wal", nil, 0o600))

	err := checkNoWAL(path)

	require.ErrorIs(t, err, ErrWALRemains)
}

func Test_no_wal_check_passes_when_no_wal_file_exists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.duckdb")

	err := checkNoWAL(path)

	assert.NoError(t, err)
}
