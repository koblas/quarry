package v9fixture_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Needs a real file copy, not the platform/sqlite package, to prove the
// bundle's WAL-only row is absent from data's own bytes.
func Test_a_byte_copy_of_data_omits_the_WAL_only_account(t *testing.T) {
	bundle := v9fixture.OpenBundle(t, t.TempDir())

	raw, err := os.ReadFile(bundle.DataPath)
	require.NoError(t, err)
	copyPath := filepath.Join(t.TempDir(), "copy")
	require.NoError(t, os.WriteFile(copyPath, raw, 0o600))

	copyDB, err := sql.Open("sqlite3", copyPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = copyDB.Close() })
	var copyCount int
	require.NoError(t, copyDB.QueryRow("SELECT count(*) FROM ZACCOUNT").Scan(&copyCount))

	liveDB, err := sql.Open("sqlite3", bundle.DataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = liveDB.Close() })
	var liveCount int
	require.NoError(t, liveDB.QueryRow("SELECT count(*) FROM ZACCOUNT").Scan(&liveCount))

	assert.Equal(t, 1, copyCount)
	assert.Equal(t, 2, liveCount)
}
