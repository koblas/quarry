package duckstore_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// addImportRun copies the store's first run as a run of its own id, snapshot path and accounts count,
// through a writable connection closed before any read.
func addImportRun(t *testing.T, st *duckstore.Store, id int, snapshotPath string, accounts int) {
	t.Helper()
	conn, err := sql.Open("duckdb", st.Path())
	require.NoError(t, err)
	const clone = "INSERT INTO import_runs SELECT * REPLACE (? AS id, ? AS snapshot_path, ? AS accounts_rows) " + //nolint:unqueryvet // a copy of the row is the point
		"FROM import_runs WHERE id = (SELECT min(id) FROM import_runs)"
	_, err = conn.ExecContext(t.Context(), clone, id, snapshotPath, accounts)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func Test_status_reads_the_latest_import_run(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	addImportRun(t, st, 3, "/snapshots/run-3.sqlite", 33)
	addImportRun(t, st, 2, "/snapshots/run-2.sqlite", 22)

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.EqualValues(t, 3, got.Run.ID)
	assert.Equal(t, "/snapshots/run-3.sqlite", got.Run.Snapshot.Path)
	assert.Equal(t, 33, got.Run.Counts.Accounts)
}

func Test_status_refuses_a_store_without_an_import_run(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.ImportRuns = nil
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), rows)
	require.NoError(t, err)

	_, err = st.Status(t.Context())

	assertOtherFault(t, err, "the store has no import history")
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

// assertOtherFault requires err to be the *store.OpenError of an unclassified fault, its Reason the one line reason.
func assertOtherFault(t *testing.T, err error, reason string) {
	t.Helper()
	openErr, ok := errors.AsType[*store.OpenError](err)
	require.True(t, ok, "want *store.OpenError, got %v", err)
	assert.Equal(t, store.OpenFaultOther, openErr.Fault)
	assert.Equal(t, reason, openErr.Reason)
}
