package duckstore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_status_reads_back_what_replace_wrote(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir, duckstore.WithQuarryVersion("v1.2.3"))
	rows := minimalRows()
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-2", SourceID: 2, AccountID: "acct-1", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Amount: 500, Currency: "CAD", Status: "uncleared",
	})
	before := time.Now().UTC().Truncate(time.Microsecond)
	_, err := st.Replace(t.Context(), rows)
	require.NoError(t, err)
	after := time.Now().UTC()

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "quarry.duckdb"), got.Path)
	assert.Equal(t, duckstore.FormatVersion, got.FormatVersion)
	assert.Equal(t, "v1.2.3", got.QuarryVersion)
	assert.False(t, got.BuiltAt.Before(before))
	assert.False(t, got.BuiltAt.After(after))
	assert.Equal(t, rows.ImportRuns[0], got.Run)
	assert.Equal(t, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), got.FirstDate)
	assert.Equal(t, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), got.LastDate)
}

func Test_status_reads_null_taken_at_and_source_as_zero(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.ImportRuns[0].Snapshot.TakenAt = time.Time{}
	rows.ImportRuns[0].Snapshot.Source = ""
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), rows)
	require.NoError(t, err)

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.True(t, got.Run.Snapshot.TakenAt.IsZero())
	assert.Empty(t, got.Run.Snapshot.Source)
}

func Test_status_reports_no_dates_for_a_store_without_transactions(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Transactions = nil
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), rows)
	require.NoError(t, err)

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.True(t, got.FirstDate.IsZero())
	assert.True(t, got.LastDate.IsZero())
}

func Test_status_refuses_a_store_without_exactly_one_import_run(t *testing.T) {
	t.Parallel()
	second := minimalRows().ImportRuns[0]
	second.ID = 2
	cases := []struct {
		name string
		runs []store.ImportRun
	}{
		{name: "no import run", runs: nil},
		{name: "two import runs", runs: append(minimalRows().ImportRuns, second)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := minimalRows()
			rows.ImportRuns = c.runs
			st := duckstore.New(t.TempDir())
			_, err := st.Replace(t.Context(), rows)
			require.NoError(t, err)

			_, err = st.Status(t.Context())

			assert.ErrorContains(t, err, "expected exactly one import run")
		})
	}
}

func Test_status_fails_on_a_missing_store_without_creating_it(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "quarry")

	_, err := duckstore.New(dir).Status(t.Context())

	require.Error(t, err)
	_, statErr := os.Stat(dir)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

var errQueryFailed = errors.New("query failed")

// spyReadDB wraps a real read connection (or none) and counts Close calls;
// a non-nil queryFault fails every query.
type spyReadDB struct {
	duckstore.ReadDB

	queryFault error
	closes     int
}

func (s *spyReadDB) QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error {
	if s.queryFault != nil {
		return s.queryFault
	}
	return s.ReadDB.QueryRows(ctx, query, args, row)
}

func (s *spyReadDB) Close() error {
	s.closes++
	if s.ReadDB == nil {
		return nil
	}
	return s.ReadDB.Close()
}

// ioFault is the error chain duckdb.DB returns for a failed read: the
// adapter's wrap around the driver's own error type.
func ioFault(op string) error {
	return fmt.Errorf("%s: %w", op, &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO, Msg: "IO Error: disk read failed"})
}

func Test_status_returns_the_open_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault("open store read-only")
	st := duckstore.New(t.TempDir(), duckstore.WithOpenReadOnly(func(context.Context, string) (duckstore.ReadDB, error) {
		return nil, fault
	}))

	_, err := st.Status(t.Context())

	require.ErrorIs(t, err, fault)
}

func Test_status_returns_the_query_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT"`)
	spy := &spyReadDB{queryFault: fault}
	st := duckstore.New(t.TempDir(), duckstore.WithOpenReadOnly(func(context.Context, string) (duckstore.ReadDB, error) {
		return spy, nil
	}))

	_, err := st.Status(t.Context())

	require.ErrorIs(t, err, fault)
	var derr *duckdbdriver.Error
	assert.ErrorAs(t, err, &derr)
}

func Test_status_closes_the_connection_on_success_and_on_a_query_fault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault error
	}{
		{name: "after a successful read", fault: nil},
		{name: "after a query fault", fault: errQueryFailed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			_, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
			require.NoError(t, err)
			spy := &spyReadDB{queryFault: c.fault}
			st := duckstore.New(dir, duckstore.WithOpenReadOnly(func(ctx context.Context, p string) (duckstore.ReadDB, error) {
				db, openErr := duckdb.OpenReadOnly(ctx, p)
				spy.ReadDB = db
				return spy, openErr
			}))

			_, _ = st.Status(t.Context())

			assert.Equal(t, 1, spy.closes)
		})
	}
}
