package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/platform/sqlschema"
	sqlite3 "github.com/mattn/go-sqlite3"
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

// A catalog row for a virtual table whose module is absent makes reading
// its columns fail without ever hitting a Scan or a query-open fault.
func Test_schema_wraps_the_error_when_a_table_column_read_fails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	_, err = conn.Exec("PRAGMA writable_schema = ON")
	require.NoError(t, err)
	_, err = conn.Exec("INSERT INTO sqlite_master (type, name, tbl_name, rootpage, sql) VALUES " +
		"('table', 'ZFOO', 'ZFOO', 0, 'CREATE VIRTUAL TABLE ZFOO USING nonexistent_module')")
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Schema(t.Context())

	require.Error(t, err)
	assert.ErrorContains(t, err, "columns of ZFOO")
	assert.ErrorContains(t, err, "no such module")
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
	row := rawIntegrityCheckRow(t, path)
	rowLines := strings.Split(row, "\n")
	require.True(t, strings.HasPrefix(row, "*** in database"))

	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	err = db.IntegrityCheck(t.Context())

	var integrityErr sqlite.IntegrityError
	require.ErrorAs(t, err, &integrityErr)
	assert.Equal(t, rowLines[len(rowLines)-1], integrityErr.Result)
	assert.NotContains(t, integrityErr.Result, "*** in database")
}

// rawIntegrityCheckRow reads PRAGMA integrity_check's first row through a
// connection independent of the code under test.
func rawIntegrityCheckRow(t *testing.T, path string) string {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var row string
	require.NoError(t, conn.QueryRow("PRAGMA integrity_check").Scan(&row))
	return row
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

	err = sqlite.Backup(t.Context(), src, destPath, 0)

	require.Error(t, err)
}

func Test_backup_fails_when_the_destination_is_not_a_database(t *testing.T) {
	srcPath := newTestDatabase(t)
	src, err := sqlite.OpenReadOnly(t.Context(), srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, []byte("not a database"), 0o600))

	err = sqlite.Backup(t.Context(), src, destPath, 0)

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

	err = sqlite.Backup(t.Context(), src, destPath, 0)

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

	err = sqlite.Backup(t.Context(), src, destPath, 0)
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

	err = sqlite.Backup(t.Context(), src, destPath, 0)
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

	err = sqlite.Backup(t.Context(), src, destPath, 0)

	require.Error(t, err)
}

func Test_IsNotADB_and_IsBusy_classify_sqlite3_error_codes(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantNotADB bool
		wantBusy   bool
	}{
		{name: "not a database", err: sqlite3.Error{Code: sqlite3.ErrNotADB}, wantNotADB: true},
		{name: "busy", err: sqlite3.Error{Code: sqlite3.ErrBusy}, wantBusy: true},
		{name: "locked", err: sqlite3.Error{Code: sqlite3.ErrLocked}, wantBusy: true},
		{name: "busy extended code (recovery)", err: sqlite3.Error{Code: sqlite3.ErrNo(sqlite3.ErrBusyRecovery)}, wantBusy: true},
		{name: "locked extended code (shared cache)", err: sqlite3.Error{Code: sqlite3.ErrNo(sqlite3.ErrLockedSharedCache)}, wantBusy: true},
		{name: "unrelated sqlite3 error", err: sqlite3.Error{Code: sqlite3.ErrCorrupt}},
		{name: "non-sqlite3 error", err: errors.New("boom")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.wantNotADB, sqlite.IsNotADB(c.err))
			assert.Equal(t, c.wantBusy, sqlite.IsBusy(c.err))
		})
	}
}

// lockDatabaseExclusively opens a second connection to path and holds an
// uncommitted BEGIN EXCLUSIVE for the rest of the test.
func lockDatabaseExclusively(t *testing.T, path string) {
	t.Helper()
	locker, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	locker.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = locker.Close() })
	conn, err := locker.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(t.Context(), "BEGIN EXCLUSIVE")
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "INSERT INTO t (v) VALUES ('locked')")
	require.NoError(t, err)
}

// busy_timeout=0 disables SQLite's own retry, so any wait is runBackup's.
// Bounded by 2s so a still-broken loop times out the test, not the suite.
func Test_backup_fails_when_the_source_is_exclusively_locked_past_the_deadline(t *testing.T) {
	path := newTestDatabase(t)
	src, err := sqlite.OpenReadOnlyBusy(t.Context(), path, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	lockDatabaseExclusively(t, path)
	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, nil, 0o600))

	result := make(chan error, 1)
	go func() {
		result <- sqlite.Backup(t.Context(), src, destPath, 50*time.Millisecond)
	}()

	select {
	case err := <-result:
		require.Error(t, err)
		assert.True(t, sqlite.IsBusy(err), "expected a busy-classified error, got %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Backup did not return within 2s of the source being locked past its deadline")
	}
}

// busyTimeout is an hour, so only runBackup's own ctx check can return this
// quickly. Cancel fires after the goroutine is inside the retry loop.
func Test_backup_stops_when_the_context_is_cancelled_while_waiting_on_a_busy_source(t *testing.T) {
	path := newTestDatabase(t)
	src, err := sqlite.OpenReadOnlyBusy(t.Context(), path, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	lockDatabaseExclusively(t, path)
	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, nil, 0o600))
	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		result <- sqlite.Backup(ctx, src, destPath, time.Hour)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Backup did not stop within 2s of ctx being cancelled while waiting on a busy source")
	}
}

// Proves the retry loop converges on success under real lock contention,
// not just the two boundary outcomes the tests above cover.
func Test_backup_succeeds_once_the_source_lock_releases_within_the_busy_budget(t *testing.T) {
	path := newTestDatabase(t)
	src, err := sqlite.OpenReadOnlyBusy(t.Context(), path, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	locker, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	locker.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = locker.Close() })
	conn, err := locker.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(t.Context(), "BEGIN EXCLUSIVE")
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "INSERT INTO t (v) VALUES ('before-commit')")
	require.NoError(t, err)

	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, nil, 0o600))

	result := make(chan error, 1)
	go func() {
		result <- sqlite.Backup(t.Context(), src, destPath, 2*time.Second)
	}()
	go func() {
		time.Sleep(180 * time.Millisecond)
		_, _ = conn.ExecContext(context.Background(), "COMMIT")
	}()

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Backup did not return within 3s of the source lock being released")
	}

	dest, err := sqlite.OpenReadOnly(t.Context(), destPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dest.Close() })
	got, err := dest.QueryInt(t.Context(), "SELECT count(*) FROM t WHERE v = 'before-commit'")
	require.NoError(t, err)
	assert.Equal(t, 1, got)
}

// The writer releases well before busyTimeout elapses.
func Test_open_read_only_busy_succeeds_once_the_writer_releases_the_lock_before_the_timeout(t *testing.T) {
	path := newTestDatabase(t)
	locker, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	locker.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = locker.Close() })
	conn, err := locker.Conn(t.Context())
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "BEGIN EXCLUSIVE")
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "INSERT INTO t (v) VALUES ('locked')")
	require.NoError(t, err)
	go func() {
		time.Sleep(30 * time.Millisecond)
		_, _ = conn.ExecContext(context.Background(), "COMMIT")
	}()

	db, err := sqlite.OpenReadOnlyBusy(t.Context(), path, 2*time.Second)

	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
}
