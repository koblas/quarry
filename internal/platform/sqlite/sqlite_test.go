package sqlite_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/platform/sqlschema"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	require.NoError(t, err)
	_, err = conn.Exec("INSERT INTO t (v) VALUES ('a')")
	require.NoError(t, err)
	return path
}

func Test_open_read_only_refuses_writes(t *testing.T) {
	path := newTestDatabase(t)

	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(t.Context(), "INSERT INTO t (v) VALUES ('b')")

	require.Error(t, err)
}

func Test_open_read_only_fails_when_the_file_is_not_a_sqlite_database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))

	_, err := sqlite.OpenReadOnly(t.Context(), path)

	require.Error(t, err)
}

func Test_schema_reads_table_and_column_names(t *testing.T) {
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	got, err := db.Schema(t.Context())

	require.NoError(t, err)
	assert.Equal(t, sqlschema.Schema{"t": {"id", "v"}}, got)
}

func Test_schema_reads_quoted_table_and_column_names_verbatim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Exec(`CREATE TABLE "order" ("select" TEXT)`)
	require.NoError(t, err)

	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	got, err := db.Schema(t.Context())

	require.NoError(t, err)
	assert.Equal(t, sqlschema.Schema{"order": {"select"}}, got)
}

func Test_integrity_check_fails_on_a_corrupted_database(t *testing.T) {
	path := newMultiPageTestDatabase(t)
	corruptLastPage(t, path)

	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	err = db.IntegrityCheck(t.Context())

	require.Error(t, err)
}

// newMultiPageTestDatabase writes enough rows to spill past the first
// (4096-byte) page, so a later page can be corrupted without breaking the
// schema page's own readability.
func newMultiPageTestDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	require.NoError(t, err)
	for i := 0; i < 500; i++ {
		_, err = conn.Exec("INSERT INTO t (v) VALUES (?)", strings.Repeat("x", 100))
		require.NoError(t, err)
	}
	return path
}

// corruptLastPage flips bytes in the file's final page, which by
// construction holds only row data, not the schema page.
func corruptLastPage(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Greater(t, len(raw), 8192)
	for i := len(raw) - 200; i < len(raw)-100; i++ {
		raw[i] ^= 0xFF
	}
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

func Test_schema_fails_when_the_connection_is_closed(t *testing.T) {
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = db.Schema(t.Context())

	require.Error(t, err)
}

func Test_backup_fails_when_the_source_connection_is_closed(t *testing.T) {
	srcPath := newTestDatabase(t)
	src, err := sqlite.OpenReadOnly(t.Context(), srcPath)
	require.NoError(t, err)
	require.NoError(t, src.Close())
	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, nil, 0o600))

	err = sqlite.Backup(t.Context(), src, destPath)

	require.Error(t, err)
}

func Test_backup_fails_when_the_destination_is_not_a_database(t *testing.T) {
	srcPath := newTestDatabase(t)
	src, err := sqlite.OpenReadOnly(t.Context(), srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, []byte("not a database"), 0o600))

	err = sqlite.Backup(t.Context(), src, destPath)

	require.Error(t, err)
}

func Test_query_int_fails_on_a_query_error(t *testing.T) {
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.QueryInt(t.Context(), "SELECT count(*) FROM missing_table")

	require.Error(t, err)
}

func Test_integrity_check_fails_when_the_connection_is_closed(t *testing.T) {
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	err = db.IntegrityCheck(t.Context())

	require.Error(t, err)
}

func Test_backup_fails_when_the_destination_file_is_read_only(t *testing.T) {
	srcPath := newTestDatabase(t)
	src, err := sqlite.OpenReadOnly(t.Context(), srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, nil, 0o400))
	t.Cleanup(func() { _ = os.Chmod(destPath, 0o600) })

	err = sqlite.Backup(t.Context(), src, destPath)

	require.Error(t, err)
}

func Test_integrity_check_passes_on_a_healthy_database(t *testing.T) {
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	err = db.IntegrityCheck(t.Context())

	assert.NoError(t, err)
}

func Test_query_int_returns_the_selected_value(t *testing.T) {
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	got, err := db.QueryInt(t.Context(), "SELECT count(*) FROM t")

	require.NoError(t, err)
	assert.Equal(t, 1, got)
}

func Test_backup_copies_rows_into_the_destination(t *testing.T) {
	srcPath := newTestDatabase(t)
	src, err := sqlite.OpenReadOnly(t.Context(), srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, nil, 0o600))

	err = sqlite.Backup(t.Context(), src, destPath)
	require.NoError(t, err)

	dest, err := sqlite.OpenReadOnly(t.Context(), destPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dest.Close() })
	got, err := dest.QueryInt(t.Context(), "SELECT count(*) FROM t")
	require.NoError(t, err)
	assert.Equal(t, 1, got)
}

func Test_backup_sets_journal_mode_delete_on_the_destination(t *testing.T) {
	srcPath := newWALTestDatabase(t)
	src, err := sqlite.OpenReadOnly(t.Context(), srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, nil, 0o600))

	err = sqlite.Backup(t.Context(), src, destPath)
	require.NoError(t, err)

	// Offsets 18-19 of the SQLite header are the file-format write/read
	// version: 2 for WAL, 1 for a rollback journal. The backup API copies
	// the source's WAL header verbatim, so this is 2 unless Backup resets
	// the destination's journal mode itself.
	header, err := os.ReadFile(destPath)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(header), 20)
	assert.Equal(t, []byte{0x01, 0x01}, header[18:20])

	_, err = os.Stat(destPath + "-wal")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// newWALTestDatabase creates a database already switched into WAL mode, so
// its header bytes 18-19 read 2 (see Test_backup_sets_journal_mode_delete_on_the_destination).
func newWALTestDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)
	_, err = conn.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	require.NoError(t, err)
	_, err = conn.Exec("INSERT INTO t (v) VALUES ('a')")
	require.NoError(t, err)
	return path
}

func Test_backup_fails_when_the_destination_directory_is_missing(t *testing.T) {
	srcPath := newTestDatabase(t)
	src, err := sqlite.OpenReadOnly(t.Context(), srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	destPath := filepath.Join(t.TempDir(), "missing-dir", "dest")

	err = sqlite.Backup(t.Context(), src, destPath)

	require.Error(t, err)
}
