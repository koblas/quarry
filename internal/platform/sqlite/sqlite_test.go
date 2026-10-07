package sqlite_test

import (
	"context"
	"database/sql"
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

func Test_open_read_only_refuses_writes(t *testing.T) {
	t.Parallel()
	path := newTestDatabase(t)

	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(t.Context(), "INSERT INTO t (v) VALUES ('b')")

	require.Error(t, err)
}

func Test_open_read_only_fails_when_the_file_is_not_a_sqlite_database(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))

	_, err := sqlite.OpenReadOnly(t.Context(), path)

	require.Error(t, err)
}

func Test_schema_reads_table_and_column_names(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "PRAGMA writable_schema = ON")
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "INSERT INTO sqlite_master (type, name, tbl_name, rootpage, sql) VALUES "+
		"('table', 'ZFOO', 'ZFOO', 0, 'CREATE VIRTUAL TABLE ZFOO USING nonexistent_module')")
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Schema(t.Context())

	require.Error(t, err)
	require.ErrorContains(t, err, "columns of ZFOO")
	assert.ErrorContains(t, err, "no such module")
}

func Test_schema_reads_quoted_table_and_column_names_verbatim(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(t.Context(), `CREATE TABLE "order" ("select" TEXT)`)
	require.NoError(t, err)

	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	got, err := db.Schema(t.Context())

	require.NoError(t, err)
	assert.Equal(t, sqlschema.Schema{"order": {"select"}}, got)
}

func Test_integrity_check_fails_on_a_corrupted_database(t *testing.T) {
	t.Parallel()
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

func Test_schema_fails_when_the_connection_is_closed(t *testing.T) {
	t.Parallel()
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = db.Schema(t.Context())

	require.Error(t, err)
}

func Test_backup_fails_when_the_source_connection_is_closed(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.QueryInt(t.Context(), "SELECT count(*) FROM missing_table")

	require.Error(t, err)
}

func Test_integrity_check_fails_when_the_connection_is_closed(t *testing.T) {
	t.Parallel()
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	err = db.IntegrityCheck(t.Context())

	require.Error(t, err)
}

func Test_backup_fails_when_the_destination_file_is_read_only(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	err = db.IntegrityCheck(t.Context())

	assert.NoError(t, err)
}

func Test_query_int_returns_the_selected_value(t *testing.T) {
	t.Parallel()
	path := newTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	got, err := db.QueryInt(t.Context(), "SELECT count(*) FROM t")

	require.NoError(t, err)
	assert.Equal(t, 1, got)
}

func Test_backup_copies_rows_into_the_destination(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	srcPath := newWALTestDatabase(t)
	src, err := sqlite.OpenReadOnly(t.Context(), srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	destPath := filepath.Join(t.TempDir(), "dest")
	require.NoError(t, os.WriteFile(destPath, nil, 0o600))

	err = sqlite.Backup(t.Context(), src, destPath, 0)
	require.NoError(t, err)

	// Offsets 18-19 are the file-format version (2=WAL, 1=rollback); Backup resets it.
	header, err := os.ReadFile(destPath)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(header), 20)
	assert.Equal(t, []byte{0x01, 0x01}, header[18:20])

	_, err = os.Stat(destPath + "-wal")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func Test_backup_fails_when_the_destination_directory_is_missing(t *testing.T) {
	t.Parallel()
	srcPath := newTestDatabase(t)
	src, err := sqlite.OpenReadOnly(t.Context(), srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	destPath := filepath.Join(t.TempDir(), "missing-dir", "dest")

	err = sqlite.Backup(t.Context(), src, destPath, 0)

	require.Error(t, err)
}

func Test_IsNotADB_and_IsBusy_classify_sqlite3_error_codes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		err        error
		wantNotADB bool
		wantBusy   bool
	}{
		{name: "not a database", err: sqlite3.Error{Code: sqlite3.ErrNotADB}, wantNotADB: true},
		{name: "not a database extended code", err: sqlite3.Error{Code: sqlite3.ErrNo(sqlite3.ErrNotADB.Extend(1))}, wantNotADB: true},
		{name: "busy", err: sqlite3.Error{Code: sqlite3.ErrBusy}, wantBusy: true},
		{name: "locked", err: sqlite3.Error{Code: sqlite3.ErrLocked}, wantBusy: true},
		{name: "busy extended code (recovery)", err: sqlite3.Error{Code: sqlite3.ErrNo(sqlite3.ErrBusyRecovery)}, wantBusy: true},
		{name: "locked extended code (shared cache)", err: sqlite3.Error{Code: sqlite3.ErrNo(sqlite3.ErrLockedSharedCache)}, wantBusy: true},
		{name: "unrelated sqlite3 error", err: sqlite3.Error{Code: sqlite3.ErrCorrupt}},
		{name: "non-sqlite3 error", err: errNotSQLite3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.wantNotADB, sqlite.IsNotADB(c.err))
			assert.Equal(t, c.wantBusy, sqlite.IsBusy(c.err))
		})
	}
}

// busy_timeout=0 disables SQLite's own retry, so any wait is runBackup's.
// Bounded by 2s so a still-broken loop times out the test, not the suite.
func Test_backup_fails_when_the_source_is_exclusively_locked_past_the_deadline(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func Test_query_rows_scans_every_row_in_order(t *testing.T) {
	t.Parallel()
	path := newMultiRowTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var got []string
	err = db.QueryRows(t.Context(), "SELECT v FROM t ORDER BY id", nil,
		func(scan func(dest ...any) error) error {
			var v string
			if err := scan(&v); err != nil {
				return err
			}
			got = append(got, v)
			return nil
		})

	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

func Test_query_rows_binds_args_into_the_query(t *testing.T) {
	t.Parallel()
	path := newMultiRowTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var got []string
	err = db.QueryRows(t.Context(), "SELECT v FROM t WHERE id > ? ORDER BY id", []any{1},
		func(scan func(dest ...any) error) error {
			var v string
			if err := scan(&v); err != nil {
				return err
			}
			got = append(got, v)
			return nil
		})

	require.NoError(t, err)
	assert.Equal(t, []string{"b", "c"}, got)
}

func Test_query_rows_fails_on_a_query_error(t *testing.T) {
	t.Parallel()
	path := newMultiRowTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	err = db.QueryRows(t.Context(), "SELECT v FROM missing_table", nil,
		func(func(dest ...any) error) error { return nil })

	require.Error(t, err)
}

func Test_query_rows_fails_on_a_scan_type_error(t *testing.T) {
	t.Parallel()
	path := newMultiRowTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	err = db.QueryRows(t.Context(), "SELECT v FROM t ORDER BY id", nil,
		func(scan func(dest ...any) error) error {
			var n int
			return scan(&n)
		})

	require.Error(t, err)
}

// Two rows exist; the callback errors on the first, so a call count of 1
// (not just the error) proves the loop stopped instead of continuing.
func Test_query_rows_stops_iterating_once_the_callback_errors(t *testing.T) {
	t.Parallel()
	path := newMultiRowTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	calls := 0
	err = db.QueryRows(t.Context(), "SELECT v FROM t ORDER BY id", nil,
		func(func(dest ...any) error) error {
			calls++
			return errQueryRowsCallback
		})

	require.ErrorIs(t, err, errQueryRowsCallback)
	assert.Equal(t, 1, calls)
}

func Test_query_rows_fails_when_the_context_is_cancelled_mid_iteration(t *testing.T) {
	t.Parallel()
	path := newMultiRowTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithCancel(t.Context())

	err = db.QueryRows(ctx, "SELECT v FROM t ORDER BY id", nil,
		func(func(dest ...any) error) error {
			cancel()
			return nil
		})

	require.Error(t, err)
}

// The context reports Canceled from the first row's callback on and never fires
// Done, so only the per-row context check can surface the cancellation.
func Test_query_rows_reports_a_cancel_that_lands_mid_iteration(t *testing.T) {
	t.Parallel()
	path := newMultiRowTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	calls := 0
	ctx := scriptedContext{err: func() error {
		if calls > 0 {
			return context.Canceled
		}
		return nil
	}}
	err = db.QueryRows(ctx, "SELECT v FROM t ORDER BY id", nil,
		func(func(dest ...any) error) error {
			calls++
			return nil
		})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}

func Test_query_rows_reports_a_deadline_that_passes_mid_iteration(t *testing.T) {
	t.Parallel()
	path := newMultiRowTestDatabase(t)
	db, err := sqlite.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	calls := 0
	ctx := scriptedContext{err: func() error {
		if calls > 0 {
			return context.DeadlineExceeded
		}
		return nil
	}}
	err = db.QueryRows(ctx, "SELECT v FROM t ORDER BY id", nil,
		func(func(dest ...any) error) error {
			calls++
			return nil
		})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, calls)
}
