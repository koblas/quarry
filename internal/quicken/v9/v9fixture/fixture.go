// Package v9fixture builds synthetic Quicken Classic for Mac v9 bundles for
// tests: an open WAL-mode database held by a second connection with
// uncheckpointed writes (the state Quicken leaves its file in while it has
// it open), and closed, non-WAL bundles for content-rejection cases.
package v9fixture

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// Bundle is a fixture .quicken bundle: a live database held open with
// uncheckpointed WAL writes.
type Bundle struct {
	Dir                 string // the .quicken bundle directory
	DataPath            string // Dir/data
	CheckpointedAccount string // account name present in DataPath's bytes
	WALOnlyAccount      string // account name present only in the WAL
}

// OpenBundle creates a Home.quicken bundle under dir with ReferenceDDL's
// schema, one account checkpointed into data and a second account written
// only to the WAL. The holder connection that keeps the WAL open stays open
// until tb.Cleanup, so DataPath's -wal and -shm files are present the whole
// test.
func OpenBundle(tb testing.TB, dir string) Bundle {
	tb.Helper()
	ctx := context.Background()

	bundleDir := filepath.Join(dir, "Home.quicken")
	require.NoError(tb, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")

	holder, err := sql.Open("sqlite3", dataPath)
	require.NoError(tb, err)
	holder.SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = holder.Close() })

	requireExec(tb, ctx, holder, "PRAGMA journal_mode=WAL")
	requireExec(tb, ctx, holder, "PRAGMA wal_autocheckpoint=0")
	requireExec(tb, ctx, holder, v9.ReferenceDDL)

	const checkpointed = "Checking"
	const walOnly = "WAL Only Savings"
	requireExec(tb, ctx, holder, "INSERT INTO ZACCOUNT (ZNAME) VALUES (?)", checkpointed)
	requireExec(tb, ctx, holder, "PRAGMA wal_checkpoint(PASSIVE)")
	requireExec(tb, ctx, holder, "INSERT INTO ZACCOUNT (ZNAME) VALUES (?)", walOnly)

	return Bundle{
		Dir:                 bundleDir,
		DataPath:            dataPath,
		CheckpointedAccount: checkpointed,
		WALOnlyAccount:      walOnly,
	}
}

func requireExec(tb testing.TB, ctx context.Context, db *sql.DB, query string, args ...any) {
	tb.Helper()
	_, err := db.ExecContext(ctx, query, args...)
	require.NoError(tb, err)
}

// EmptyAccountsBundle creates a closed, rollback-journal (non-WAL)
// Home.quicken bundle under dir with ReferenceDDL's schema and zero
// ZACCOUNT rows.
func EmptyAccountsBundle(tb testing.TB, dir string) Bundle {
	tb.Helper()
	ctx := context.Background()

	bundleDir := filepath.Join(dir, "Home.quicken")
	require.NoError(tb, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")

	conn, err := sql.Open("sqlite3", dataPath)
	require.NoError(tb, err)
	requireExec(tb, ctx, conn, v9.ReferenceDDL)
	require.NoError(tb, conn.Close())

	return Bundle{Dir: bundleDir, DataPath: dataPath}
}

// ClosedWALBundle creates a Home.quicken bundle under dir with
// ReferenceDDL's schema and one account, written through a WAL-mode
// connection that is then closed: the header stays WAL-formatted (SQLite
// never reverts it on close) but the checkpoint-on-close removes -wal/-shm,
// leaving the bundle Quicken would leave after quitting without ever
// reverting journal_mode.
func ClosedWALBundle(tb testing.TB, dir string) Bundle {
	tb.Helper()
	ctx := context.Background()

	bundleDir := filepath.Join(dir, "Home.quicken")
	require.NoError(tb, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")

	conn, err := sql.Open("sqlite3", dataPath)
	require.NoError(tb, err)
	conn.SetMaxOpenConns(1)
	requireExec(tb, ctx, conn, "PRAGMA journal_mode=WAL")
	requireExec(tb, ctx, conn, v9.ReferenceDDL)
	requireExec(tb, ctx, conn, "INSERT INTO ZACCOUNT (ZNAME) VALUES ('Checking')")
	require.NoError(tb, conn.Close())

	return Bundle{Dir: bundleDir, DataPath: dataPath}
}

// Identifiers MissingSchemaBundle mutates; none references an index, PK, UNIQUE or FK.
const (
	MissingSchemaDroppedTable       = "ZALERT"
	MissingSchemaDroppedColumnTable = "ZCLOUDSYNCDATA"
	MissingSchemaDroppedColumn1     = "ZMINTWATERMARK"
	MissingSchemaDroppedColumn2     = "ZRESOURCETYPE"
	MissingSchemaAddedColumnTable   = "ZDOCUMENTPROPERTY"
	MissingSchemaAddedColumn        = "ZQUARRYEXTRACOL"
)

// MissingSchemaBundle creates a closed, non-WAL Home.quicken bundle under
// dir with one ZACCOUNT row and a mutated schema: MissingSchemaDroppedTable
// removed entirely, MissingSchemaDroppedColumn1/2 dropped from
// MissingSchemaDroppedColumnTable, and MissingSchemaAddedColumn added to
// MissingSchemaAddedColumnTable. The result is missing one table and two
// columns the reference names, plus one column the reference does not.
func MissingSchemaBundle(tb testing.TB, dir string) Bundle {
	tb.Helper()
	ctx := context.Background()

	bundleDir := filepath.Join(dir, "Home.quicken")
	require.NoError(tb, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")

	conn, err := sql.Open("sqlite3", dataPath)
	require.NoError(tb, err)
	requireExec(tb, ctx, conn, v9.ReferenceDDL)
	requireExec(tb, ctx, conn, "INSERT INTO ZACCOUNT (ZNAME) VALUES ('Checking')")
	requireExec(tb, ctx, conn, fmt.Sprintf("DROP TABLE %s", MissingSchemaDroppedTable))
	requireExec(tb, ctx, conn, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s",
		MissingSchemaDroppedColumnTable, MissingSchemaDroppedColumn1))
	requireExec(tb, ctx, conn, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s",
		MissingSchemaDroppedColumnTable, MissingSchemaDroppedColumn2))
	requireExec(tb, ctx, conn, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s VARCHAR",
		MissingSchemaAddedColumnTable, MissingSchemaAddedColumn))
	require.NoError(tb, conn.Close())

	return Bundle{Dir: bundleDir, DataPath: dataPath}
}

// Identifiers ExtraSchemaBundle mutates: the table it adds, and two columns on two existing tables.
const (
	ExtraSchemaAddedTable        = "ZQUARRYEXTRATABLE"
	ExtraSchemaAddedColumnTable1 = "ZDOWNLOADSESSION"
	ExtraSchemaAddedColumn1      = "ZQUARRYEXTRACOL1"
	ExtraSchemaAddedColumnTable2 = "ZFOREXQUOTE"
	ExtraSchemaAddedColumn2      = "ZQUARRYEXTRACOL2"
)

// ExtraSchemaBundle creates a closed, non-WAL Home.quicken bundle under dir
// with one ZACCOUNT row and a mutated schema that adds ExtraSchemaAddedTable
// and ExtraSchemaAddedColumn1/2 on ExtraSchemaAddedColumnTable1/2, without
// removing or altering anything the reference already names.
func ExtraSchemaBundle(tb testing.TB, dir string) Bundle {
	tb.Helper()
	ctx := context.Background()

	bundleDir := filepath.Join(dir, "Home.quicken")
	require.NoError(tb, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")

	conn, err := sql.Open("sqlite3", dataPath)
	require.NoError(tb, err)
	requireExec(tb, ctx, conn, v9.ReferenceDDL)
	requireExec(tb, ctx, conn, "INSERT INTO ZACCOUNT (ZNAME) VALUES ('Checking')")
	requireExec(tb, ctx, conn, fmt.Sprintf("CREATE TABLE %s (Z_PK INTEGER PRIMARY KEY, ZVALUE VARCHAR)", ExtraSchemaAddedTable))
	requireExec(tb, ctx, conn, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s VARCHAR",
		ExtraSchemaAddedColumnTable1, ExtraSchemaAddedColumn1))
	requireExec(tb, ctx, conn, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s VARCHAR",
		ExtraSchemaAddedColumnTable2, ExtraSchemaAddedColumn2))
	require.NoError(tb, conn.Close())

	return Bundle{Dir: bundleDir, DataPath: dataPath}
}

// CorruptDataFile builds a fresh, closed, non-WAL SQLite database at path
// with a valid, non-empty ZACCOUNT table plus a heavily indexed filler
// table, then flips bytes in the file's last page. ZACCOUNT's pages sit
// near the front of the file, so the corruption fails PRAGMA
// integrity_check without touching ZACCOUNT's row, sqlite_master, or any
// pragma_table_info column read.
func CorruptDataFile(tb testing.TB, path string) {
	tb.Helper()
	ctx := context.Background()

	conn, err := sql.Open("sqlite3", path)
	require.NoError(tb, err)
	requireExec(tb, ctx, conn, "CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY, ZNAME TEXT)")
	requireExec(tb, ctx, conn, "INSERT INTO ZACCOUNT (ZNAME) VALUES ('Checking')")
	requireExec(tb, ctx, conn, "CREATE TABLE ZFILLER (id INTEGER PRIMARY KEY, v TEXT)")
	requireExec(tb, ctx, conn, "CREATE INDEX ZFILLER_V ON ZFILLER(v)")
	for i := 0; i < 1000; i++ {
		requireExec(tb, ctx, conn, "INSERT INTO ZFILLER (v) VALUES (?)", strings.Repeat("x", 100))
	}
	require.NoError(tb, conn.Close())

	raw, err := os.ReadFile(path)
	require.NoError(tb, err)
	require.Greater(tb, len(raw), 8192)
	for i := len(raw) - 200; i < len(raw)-100; i++ {
		raw[i] ^= 0xFF
	}
	require.NoError(tb, os.WriteFile(path, raw, 0o600))

	requireIntegrityCheckFailsOnly(tb, path)
}

// requireIntegrityCheckFailsOnly confirms path is damaged in exactly the
// way CorruptDataFile promises: PRAGMA integrity_check's first row starts
// with SQLite's boilerplate header, while ZACCOUNT's row and sqlite_master
// itself still read cleanly, through a connection independent of any
// production code under test.
func requireIntegrityCheckFailsOnly(tb testing.TB, path string) {
	tb.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })

	var result string
	require.NoError(tb, conn.QueryRow("PRAGMA integrity_check").Scan(&result))
	require.True(tb, strings.HasPrefix(result, "*** in database"))

	var count int
	require.NoError(tb, conn.QueryRow("SELECT count(*) FROM ZACCOUNT").Scan(&count))
	require.Equal(tb, 1, count)

	var tables int
	require.NoError(tb, conn.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&tables))
	require.Greater(tb, tables, 0)
}
