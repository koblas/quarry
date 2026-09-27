package v9fixture_test

import (
	"database/sql"
	"fmt"
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

func Test_MissingSchemaBundle_drops_a_table_two_columns_and_adds_one(t *testing.T) {
	bundle := v9fixture.MissingSchemaBundle(t, t.TempDir())
	conn, err := sql.Open("sqlite3", bundle.DataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	var tableCount int
	require.NoError(t, conn.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		v9fixture.MissingSchemaDroppedTable).Scan(&tableCount))
	assert.Zero(t, tableCount)

	droppedFrom := tableColumns(t, conn, v9fixture.MissingSchemaDroppedColumnTable)
	assert.NotContains(t, droppedFrom, v9fixture.MissingSchemaDroppedColumn1)
	assert.NotContains(t, droppedFrom, v9fixture.MissingSchemaDroppedColumn2)

	addedTo := tableColumns(t, conn, v9fixture.MissingSchemaAddedColumnTable)
	assert.Contains(t, addedTo, v9fixture.MissingSchemaAddedColumn)

	var accounts int
	require.NoError(t, conn.QueryRow("SELECT count(*) FROM ZACCOUNT").Scan(&accounts))
	assert.Equal(t, 1, accounts)
}

func Test_ExtraSchemaBundle_adds_a_table_and_two_columns(t *testing.T) {
	bundle := v9fixture.ExtraSchemaBundle(t, t.TempDir())
	conn, err := sql.Open("sqlite3", bundle.DataPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	var tableCount int
	require.NoError(t, conn.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		v9fixture.ExtraSchemaAddedTable).Scan(&tableCount))
	assert.Equal(t, 1, tableCount)

	cols1 := tableColumns(t, conn, v9fixture.ExtraSchemaAddedColumnTable1)
	assert.Contains(t, cols1, v9fixture.ExtraSchemaAddedColumn1)
	cols2 := tableColumns(t, conn, v9fixture.ExtraSchemaAddedColumnTable2)
	assert.Contains(t, cols2, v9fixture.ExtraSchemaAddedColumn2)

	var accounts int
	require.NoError(t, conn.QueryRow("SELECT count(*) FROM ZACCOUNT").Scan(&accounts))
	assert.Equal(t, 1, accounts)
}

// tableColumns queries table's columns directly, independent of any
// production schema-reading code under test.
func tableColumns(t *testing.T, conn *sql.DB, table string) []string {
	t.Helper()
	rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var cols []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		require.NoError(t, rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk))
		cols = append(cols, name)
	}
	require.NoError(t, rows.Err())
	return cols
}
