package sqlite_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestDatabase(tb testing.TB) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(tb.Context(), "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	require.NoError(tb, err)
	_, err = conn.ExecContext(tb.Context(), "INSERT INTO t (v) VALUES ('a')")
	require.NoError(tb, err)
	return path
}

// rawIntegrityCheckRow reads PRAGMA integrity_check's first row through a
// connection independent of the code under test.
func rawIntegrityCheckRow(tb testing.TB, path string) string {
	tb.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })
	var row string
	require.NoError(tb, conn.QueryRowContext(tb.Context(), "PRAGMA integrity_check").Scan(&row))
	return row
}

// newMultiPageTestDatabase writes enough rows to spill past the first
// (4096-byte) page, so a later page can be corrupted without breaking the
// schema page's own readability.
func newMultiPageTestDatabase(tb testing.TB) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(tb.Context(), "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	require.NoError(tb, err)
	for range 500 {
		_, err = conn.ExecContext(tb.Context(), "INSERT INTO t (v) VALUES (?)", strings.Repeat("x", 100))
		require.NoError(tb, err)
	}
	return path
}

// corruptLastPage flips bytes in the file's final page, which by
// construction holds only row data, not the schema page.
func corruptLastPage(tb testing.TB, path string) {
	tb.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(tb, err)
	require.Greater(tb, len(raw), 8192)
	for i := len(raw) - 200; i < len(raw)-100; i++ {
		raw[i] ^= 0xFF
	}
	require.NoError(tb, os.WriteFile(path, raw, 0o600)) //nolint:gosec // path is under the test's own temp dir
}

// newWALTestDatabase creates a database already switched into WAL mode, so
// its header bytes 18-19 read 2 (see Test_backup_sets_journal_mode_delete_on_the_destination).
func newWALTestDatabase(tb testing.TB) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(tb.Context(), "PRAGMA journal_mode=WAL")
	require.NoError(tb, err)
	_, err = conn.ExecContext(tb.Context(), "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	require.NoError(tb, err)
	_, err = conn.ExecContext(tb.Context(), "INSERT INTO t (v) VALUES ('a')")
	require.NoError(tb, err)
	return path
}

var errNotSQLite3 = errors.New("boom")

// lockDatabaseExclusively opens a second connection to path and holds an
// uncommitted BEGIN EXCLUSIVE for the rest of the test.
func lockDatabaseExclusively(tb testing.TB, path string) {
	tb.Helper()
	locker, err := sql.Open("sqlite3", path)
	require.NoError(tb, err)
	locker.SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = locker.Close() })
	conn, err := locker.Conn(tb.Context())
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(tb.Context(), "BEGIN EXCLUSIVE")
	require.NoError(tb, err)
	_, err = conn.ExecContext(tb.Context(), "INSERT INTO t (v) VALUES ('locked')")
	require.NoError(tb, err)
}

func newMultiRowTestDatabase(tb testing.TB) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(tb.Context(), "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	require.NoError(tb, err)
	_, err = conn.ExecContext(tb.Context(), "INSERT INTO t (id, v) VALUES (1, 'a'), (2, 'b'), (3, 'c')")
	require.NoError(tb, err)
	return path
}

var errQueryRowsCallback = errors.New("boom")

// scriptedContext is a context that never fires Done, so the driver's own interrupt
// cannot be what stops a read; its Err is whatever err returns.
type scriptedContext struct {
	err func() error
}

func (scriptedContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (scriptedContext) Done() <-chan struct{}       { return nil }
func (scriptedContext) Value(any) any               { return nil }
func (c scriptedContext) Err() error                { return c.err() }
