package duckstore_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
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

// The count columns are nullable in the schema, and Replace never writes NULL,
// so the test nulls them through a writable connection of its own.
func Test_status_reads_null_check_counts_as_zero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	conn, err := sql.Open("duckdb", st.Path())
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), `UPDATE import_runs SET balances_never_reconciled = NULL,
		investment_accounts = NULL, transfers_paired = NULL, transfers_cross_currency = NULL`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.Zero(t, got.Run.BalancesNeverReconciled)
	assert.Zero(t, got.Run.InvestmentAccounts)
	assert.Zero(t, got.Run.TransfersPaired)
	assert.Zero(t, got.Run.TransfersCrossCurrency)
	assert.Equal(t, minimalRows().ImportRuns[0].BalancesChecked, got.Run.BalancesChecked)
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
		name  string
		runs  []store.ImportRun
		found string
	}{
		{name: "no import run", runs: nil, found: "0"},
		{name: "two import runs", runs: append(minimalRows().ImportRuns, second), found: "2"},
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

			assertOtherFault(t, err, "expected exactly one import run, found "+c.found)
		})
	}
}

func Test_status_fails_on_a_missing_store_without_creating_it(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := duckstore.New(dir).Status(t.Context())

	openErr, ok := errors.AsType[*store.OpenError](err)
	require.True(t, ok, "want *store.OpenError, got %v", err)
	assert.Equal(t, store.OpenFaultMissing, openErr.Fault)
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func Test_status_returns_the_open_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault("open store read-only")
	st := newBuiltStore(t, failingOpener(fault))

	_, err := st.Status(t.Context())

	require.ErrorIs(t, err, fault)
	var openErr *store.OpenError
	assert.ErrorAs(t, err, &openErr)
}

// assertOtherFault requires err to be the *store.OpenError of an unclassified fault, its Reason the one line reason.
func assertOtherFault(t *testing.T, err error, reason string) {
	t.Helper()
	openErr, ok := errors.AsType[*store.OpenError](err)
	require.True(t, ok, "want *store.OpenError, got %v", err)
	assert.Equal(t, store.OpenFaultOther, openErr.Fault)
	assert.Equal(t, reason, openErr.Reason)
}

func Test_status_returns_the_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: fault}))

	_, err := st.Status(t.Context())

	assertOtherFault(t, err, "disk read failed")
	var derr *duckdbdriver.Error
	require.ErrorAs(t, err, &derr)
	assert.ErrorIs(t, err, fault)
}

func Test_status_returns_a_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{scanFault: errScanFailed}))

	_, err := st.Status(t.Context())

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
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
			spy := &spyReadDB{queryFault: c.fault}
			st := newBuiltStore(t, spyOpener(spy))

			_, _ = st.Status(t.Context())

			assert.Equal(t, 1, spy.closes)
		})
	}
}
