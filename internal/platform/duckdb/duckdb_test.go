package duckdb_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End to end: AppendRows writes negative/zero/boundary DECIMAL(18,2) values,
// CheckpointClose removes the .wal, and a fresh read-only connection reads every value back exactly.
func Test_bulk_inserted_decimals_read_back_exactly_after_checkpoint_close(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	db, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)

	_, err = db.Exec(t.Context(), "CREATE TABLE t (id INTEGER, a DECIMAL(18,2))")
	require.NoError(t, err)

	negative, err := duckdb.Decimal(-120417, 18, 2)
	require.NoError(t, err)
	zero, err := duckdb.Decimal(0, 18, 2)
	require.NoError(t, err)
	boundary, err := duckdb.Decimal(999999999999999999, 18, 2)
	require.NoError(t, err)

	err = db.AppendRows(t.Context(), "t", [][]any{
		{int32(1), negative},
		{int32(2), zero},
		{int32(3), boundary},
	})
	require.NoError(t, err)

	_, err = os.Stat(path + ".wal")
	require.NoError(t, err, "a .wal file must exist before CheckpointClose")

	require.NoError(t, db.CheckpointClose(t.Context()))

	_, err = os.Stat(path + ".wal")
	require.ErrorIs(t, err, os.ErrNotExist)

	reader, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	got := map[int32]string{}
	err = reader.QueryRows(t.Context(), "SELECT id, CAST(a AS VARCHAR) FROM t ORDER BY id", nil,
		func(scan func(dest ...any) error) error {
			var id int32
			var value string
			if err := scan(&id, &value); err != nil {
				return err
			}
			got[id] = value
			return nil
		})
	require.NoError(t, err)

	assert.Equal(t, map[int32]string{
		1: "-1204.17",
		2: "0.00",
		3: "9999999999999999.99",
	}, got)
}

func Test_create_refuses_an_existing_path(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))

	_, err := duckdb.Create(t.Context(), path)

	require.ErrorIs(t, err, duckdb.ErrExists)
}

func Test_create_makes_the_file_owner_only(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")

	db, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func Test_create_fails_with_a_permission_error_in_a_read_only_directory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	roDir := filepath.Join(dir, "ro")
	require.NoError(t, os.Mkdir(roDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o700) })
	path := filepath.Join(roDir, "data.duckdb")

	_, err := duckdb.Create(t.Context(), path)

	require.Error(t, err)
	assert.True(t, duckdb.IsPermission(err), "expected a permission-classified error, got %v", err)
}

// A regular file, not a directory, as a path segment makes os.Stat fail
// with ENOTDIR — an error Create must not mistake for "does not exist yet".
func Test_create_fails_when_a_path_segment_is_a_regular_file(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	require.NoError(t, os.WriteFile(regular, []byte("x"), 0o600))
	path := filepath.Join(regular, "sub", "data.duckdb")

	_, err := duckdb.Create(t.Context(), path)

	require.Error(t, err)
	require.NotErrorIs(t, err, duckdb.ErrExists)
}

// sql.Open's own eager file-create/open already succeeded (the path is a
// writable temp dir), so a context cancelled before Create runs lands
// specifically on PingContext, not on sql.Open.
func Test_create_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := duckdb.Create(ctx, path)

	require.ErrorIs(t, err, context.Canceled)
}

func Test_create_never_fetches_an_extension(t *testing.T) {
	t.Parallel()
	db, err := duckdb.Create(t.Context(), filepath.Join(t.TempDir(), "data.duckdb"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	cases := []struct {
		setting string
		want    string
	}{
		{setting: "autoload_known_extensions", want: "false"},
		{setting: "autoinstall_known_extensions", want: "false"},
		{setting: "enable_external_access", want: "false"},
	}

	for _, c := range cases {
		t.Run(c.setting, func(t *testing.T) {
			t.Parallel()

			table, err := db.QueryTable(t.Context(), "SELECT current_setting('"+c.setting+"')::VARCHAR", 0)

			require.NoError(t, err)
			require.Len(t, table.Rows, 1)
			assert.Equal(t, c.want, table.Rows[0][0].Text)
		})
	}
}

func Test_create_in_memory_gives_a_database_that_stores_and_reads_back_rows(t *testing.T) {
	t.Parallel()
	db, err := duckdb.CreateInMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(t.Context(), "CREATE TABLE t (id INTEGER)")
	require.NoError(t, err)
	require.NoError(t, db.AppendRows(t.Context(), "t", [][]any{{int32(7)}, {int32(8)}}))

	var got []int32
	err = db.QueryRows(t.Context(), "SELECT id FROM t ORDER BY id", nil, func(scan func(dest ...any) error) error {
		var id int32
		if err := scan(&id); err != nil {
			return err
		}
		got = append(got, id)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []int32{7, 8}, got)
}

func Test_create_in_memory_gives_each_call_a_database_of_its_own(t *testing.T) {
	t.Parallel()
	first, err := duckdb.CreateInMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := duckdb.CreateInMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	_, err = first.Exec(t.Context(), "CREATE TABLE only_in_first (id INTEGER)")
	require.NoError(t, err)

	_, err = second.Exec(t.Context(), "INSERT INTO only_in_first VALUES (1)")

	require.Error(t, err)
}

func Test_create_in_memory_never_fetches_an_extension_or_reaches_outside(t *testing.T) {
	t.Parallel()
	db, err := duckdb.CreateInMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	cases := []struct {
		setting string
		want    string
	}{
		{setting: "autoload_known_extensions", want: "false"},
		{setting: "autoinstall_known_extensions", want: "false"},
		{setting: "enable_external_access", want: "false"},
	}

	for _, c := range cases {
		t.Run(c.setting, func(t *testing.T) {
			t.Parallel()

			table, err := db.QueryTable(t.Context(), "SELECT current_setting('"+c.setting+"')::VARCHAR", 0)

			require.NoError(t, err)
			require.Len(t, table.Rows, 1)
			assert.Equal(t, c.want, table.Rows[0][0].Text)
		})
	}
}

func Test_create_in_memory_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := duckdb.CreateInMemory(ctx)

	require.ErrorIs(t, err, context.Canceled)
}

func Test_open_read_write_edits_an_existing_database(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	created, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)
	_, err = created.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")
	require.NoError(t, err)
	require.NoError(t, created.CheckpointClose(t.Context()))
	editor, err := duckdb.OpenReadWrite(t.Context(), path)
	require.NoError(t, err)
	_, err = editor.Exec(t.Context(), "INSERT INTO t VALUES (7)")
	require.NoError(t, err)
	require.NoError(t, editor.CheckpointClose(t.Context()))

	reader, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	var got int
	err = reader.QueryRows(t.Context(), "SELECT v FROM t", nil, func(scan func(dest ...any) error) error { return scan(&got) })
	require.NoError(t, err)
	assert.Equal(t, 7, got)
}

func Test_open_read_write_on_a_non_duckdb_file_fails_naming_the_path(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "data.duckdb")
	require.NoError(t, os.WriteFile(path, []byte("not a database\n"), 0o600))

	_, err := duckdb.OpenReadWrite(t.Context(), path)

	require.Error(t, err)
	assert.True(t, duckdb.IsNotDatabase(err), err.Error())
	assert.ErrorContains(t, err, "open "+path+" read-write: ")
}

func Test_open_read_only_reads_the_file_now_at_the_path_while_an_earlier_open_is_still_running(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeStoreHolding(t, filepath.Join(dir, "data.duckdb"), 1)
	replacement := writeStoreHolding(t, filepath.Join(dir, "replacement.duckdb"), 2)
	earlier, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = earlier.Close() })
	require.NoError(t, os.Rename(replacement, path))

	current, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = current.Close() })

	assert.Equal(t, int32(2), onlyValue(t, current))
}

func Test_exec_fails_on_a_syntax_error(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	_, err := db.Exec(t.Context(), "NOT VALID SQL")

	require.Error(t, err)
}

func Test_query_rows_fails_on_a_query_error(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	err := db.QueryRows(t.Context(), "SELECT id FROM missing_table", nil,
		func(func(dest ...any) error) error { return nil })

	require.Error(t, err)
}

func Test_query_rows_fails_on_a_scan_type_error(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	_, err := db.Exec(t.Context(), "CREATE TABLE t (v VARCHAR)")
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), "INSERT INTO t VALUES ('not a number')")
	require.NoError(t, err)

	err = db.QueryRows(t.Context(), "SELECT v FROM t", nil,
		func(scan func(dest ...any) error) error {
			var n int
			return scan(&n)
		})

	require.Error(t, err)
}

func Test_query_rows_stops_once_the_callback_errors(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	_, err := db.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")
	require.NoError(t, err)
	require.NoError(t, db.AppendRows(t.Context(), "t", [][]any{{int32(1)}, {int32(2)}}))
	boom := assert.AnError

	calls := 0
	err = db.QueryRows(t.Context(), "SELECT v FROM t ORDER BY v", nil,
		func(func(dest ...any) error) error {
			calls++
			return boom
		})

	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, calls)
}

func Test_open_read_only_fails_on_a_missing_path(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.duckdb")

	_, err := duckdb.OpenReadOnly(t.Context(), path)

	require.Error(t, err)
}

func Test_open_read_only_refuses_writes(t *testing.T) {
	t.Parallel()
	writer, path := newOpenDatabase(t)
	require.NoError(t, writer.Close())

	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")

	require.Error(t, err)
}

func Test_open_read_only_locks_down_the_session(t *testing.T) {
	t.Parallel()
	writer, path := newOpenDatabase(t)
	require.NoError(t, writer.Close())
	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	cases := []struct {
		setting string
		want    string
	}{
		{setting: "access_mode", want: "read_only"},
		{setting: "enable_external_access", want: "false"},
		{setting: "autoload_known_extensions", want: "false"},
		{setting: "autoinstall_known_extensions", want: "false"},
		{setting: "lock_configuration", want: "true"},
	}

	for _, c := range cases {
		t.Run(c.setting, func(t *testing.T) {
			t.Parallel()

			table, err := db.QueryTable(t.Context(), "SELECT current_setting('"+c.setting+"')::VARCHAR", 0)

			require.NoError(t, err)
			require.Len(t, table.Rows, 1)
			assert.Equal(t, c.want, table.Rows[0][0].Text)
		})
	}
}

// sql.Open's own eager open of the existing file already succeeded, so a
// context cancelled before OpenReadOnly runs lands specifically on
// PingContext, not on sql.Open.
func Test_open_read_only_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	writer, path := newOpenDatabase(t)
	require.NoError(t, writer.Close())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := duckdb.OpenReadOnly(ctx, path)

	require.ErrorIs(t, err, context.Canceled)
}

// The callback cancels on the first row and returns nil, so only the per-row
// context check can surface the cancellation.
func Test_query_rows_fails_when_the_context_is_cancelled_mid_iteration(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	_, err := db.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")
	require.NoError(t, err)
	require.NoError(t, db.AppendRows(t.Context(), "t", [][]any{{int32(1)}, {int32(2)}, {int32(3)}}))
	ctx, cancel := context.WithCancel(t.Context())

	calls := 0
	err = db.QueryRows(ctx, "SELECT v FROM t ORDER BY v", nil,
		func(func(dest ...any) error) error {
			calls++
			cancel()
			return nil
		})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}

func Test_query_rows_reports_a_deadline_that_passes_mid_iteration(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	_, err := db.Exec(t.Context(), "CREATE TABLE t (v INTEGER)")
	require.NoError(t, err)
	require.NoError(t, db.AppendRows(t.Context(), "t", [][]any{{int32(1)}, {int32(2)}, {int32(3)}}))

	calls := 0
	ctx := scriptedContext{err: func() error {
		if calls > 0 {
			return context.DeadlineExceeded
		}
		return nil
	}}
	err = db.QueryRows(ctx, "SELECT v FROM t ORDER BY v", nil,
		func(func(dest ...any) error) error {
			calls++
			return nil
		})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, calls)
}

// The Appender itself refuses a table that does not exist.
func Test_append_rows_fails_when_the_table_does_not_exist(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	err := db.AppendRows(t.Context(), "does_not_exist", [][]any{{int32(1)}})

	require.Error(t, err)
}

// A cancelled context makes CHECKPOINT itself fail, distinct from the
// connection-close or no-WAL steps that follow it.
func Test_checkpoint_close_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := db.CheckpointClose(ctx)

	require.ErrorIs(t, err, context.Canceled)
}

func Test_append_rows_reports_a_duplicate_primary_key(t *testing.T) {
	t.Parallel()
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY, v VARCHAR)")

	err := db.AppendRows(t.Context(), "t", [][]any{
		{int32(1), "a"},
		{int32(1), "b"},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "constraint")
}

// The pool's own Conn(ctx) acquisition checks ctx before this package's own
// per-row check ever runs, for a context already cancelled beforehand.
func Test_append_rows_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY)")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := db.AppendRows(ctx, "t", [][]any{{int32(1)}})

	require.ErrorIs(t, err, context.Canceled)
}

func Test_append_rows_reports_a_wrong_column_count(t *testing.T) {
	t.Parallel()
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY, v VARCHAR)")

	err := db.AppendRows(t.Context(), "t", [][]any{
		{int32(1)},
	})

	require.Error(t, err)
}

// Three rows exist; Err() cancels after the first, so a row count of
// exactly 1 (not just an error) proves the loop stopped there.
func Test_append_rows_stops_when_the_context_is_cancelled(t *testing.T) {
	t.Parallel()
	db := newTestTable(t, "CREATE TABLE t (id INTEGER PRIMARY KEY)")
	ctx := &cancelAfterNErrCalls{Context: t.Context(), n: 1}

	err := db.AppendRows(ctx, "t", [][]any{
		{int32(1)},
		{int32(2)},
		{int32(3)},
	})
	require.ErrorIs(t, err, context.Canceled)

	var count int64
	require.NoError(t, db.QueryRows(t.Context(), "SELECT count(*) FROM t", nil,
		func(scan func(dest ...any) error) error { return scan(&count) }))
	assert.Equal(t, int64(1), count)
}

func Test_decimal_refuses_an_unscaled_magnitude_of_10_to_the_width(t *testing.T) {
	t.Parallel()
	_, err := duckdb.Decimal(1_000_000_000_000_000_000, 18, 2)

	require.Error(t, err)
}

func Test_decimal_accepts_an_unscaled_magnitude_one_below_10_to_the_width(t *testing.T) {
	t.Parallel()
	value, err := duckdb.Decimal(999_999_999_999_999_999, 18, 2)

	require.NoError(t, err)
	assert.NotNil(t, value)
}

func Test_decimal_refuses_a_negative_unscaled_magnitude_of_10_to_the_width(t *testing.T) {
	t.Parallel()
	_, err := duckdb.Decimal(-1_000_000_000_000_000_000, 18, 2)

	require.Error(t, err)
}
