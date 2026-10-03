package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMultiRowTestDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data")
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(t.Context(), "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), "INSERT INTO t (id, v) VALUES (1, 'a'), (2, 'b'), (3, 'c')")
	require.NoError(t, err)
	return path
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

var errQueryRowsCallback = errors.New("boom")

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

// scriptedContext is a context that never fires Done, so the driver's own interrupt
// cannot be what stops a read; its Err is whatever err returns.
type scriptedContext struct {
	err func() error
}

func (scriptedContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (scriptedContext) Done() <-chan struct{}       { return nil }
func (scriptedContext) Value(any) any               { return nil }
func (c scriptedContext) Err() error                { return c.err() }
