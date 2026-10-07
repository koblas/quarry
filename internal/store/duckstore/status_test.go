package duckstore_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
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

// The count columns are nullable in the schema, and Replace never writes NULL,
// so the test nulls them through a writable connection of its own.
func Test_status_reads_null_check_counts_as_zero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	conn, err := duckdb.OpenReadWrite(t.Context(), st.Path())
	require.NoError(t, err)
	_, err = conn.Exec(t.Context(), `UPDATE import_runs SET balances_never_reconciled = NULL,
		investment_accounts = NULL, transfers_paired = NULL, transfers_cross_currency = NULL,
		securities_rows = NULL, prices_rows = NULL, investment_transactions_rows = NULL, shares_checked = NULL`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.Zero(t, got.Run.BalancesNeverReconciled)
	assert.Zero(t, got.Run.InvestmentAccounts)
	assert.Zero(t, got.Run.TransfersPaired)
	assert.Zero(t, got.Run.TransfersCrossCurrency)
	assert.Zero(t, got.Run.Counts.Securities)
	assert.Zero(t, got.Run.Counts.Prices)
	assert.Zero(t, got.Run.Counts.InvestmentTransactions)
	assert.Zero(t, got.Run.SharesChecked)
	assert.Equal(t, minimalRows().ImportRuns[0].BalancesChecked, got.Run.BalancesChecked)
}

func Test_status_reads_the_shares_checked_count_of_the_latest_run(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.Equal(t, 21, got.Run.SharesChecked)
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
	conn, err := duckdb.OpenReadWrite(t.Context(), st.Path())
	require.NoError(t, err)
	const clone = "INSERT INTO import_runs SELECT * REPLACE (? AS id, ? AS snapshot_path, ? AS accounts_rows) " + //nolint:unqueryvet // a copy of the row is the point
		"FROM import_runs WHERE id = (SELECT min(id) FROM import_runs)"
	_, err = conn.Exec(t.Context(), clone, id, snapshotPath, accounts)
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

// setFetchError sets run id's rates_fetch_error through a writable connection closed before any read.
func setFetchError(t *testing.T, st *duckstore.Store, id int, reason any) {
	t.Helper()
	conn, err := duckdb.OpenReadWrite(t.Context(), st.Path())
	require.NoError(t, err)
	_, err = conn.Exec(t.Context(), "UPDATE import_runs SET rates_fetch_error = ? WHERE id = ?", reason, id)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func Test_status_reads_the_rate_coverage_from_fx_rates_and_the_run_that_fetched_them(t *testing.T) {
	t.Parallel()
	src := &fakeRates{refresh: store.RatesRefresh{
		Rates:      []store.Rate{ratesOn(16, 1_310_000, "FXUSDCAD"), ratesOn(13, 1_250_000, "IEXE0101")},
		FetchError: "www.bankofcanada.ca answered 503 Service Unavailable",
	}}
	st := duckstore.New(t.TempDir(), duckstore.WithRates(src))
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.Equal(t, store.StatusRates{
		First:      time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC),
		Last:       time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		FetchError: "www.bankofcanada.ca answered 503 Service Unavailable",
	}, got.Rates)
}

func Test_status_reads_no_rate_coverage_from_a_store_without_rates_or_a_fetch_error(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.Equal(t, store.StatusRates{}, got.Rates)
}

func Test_status_reads_the_fetch_error_of_the_latest_run_not_an_earlier_one(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	addImportRun(t, st, 2, "/snapshots/run-2.sqlite", 22)
	setFetchError(t, st, 2, "cannot reach www.bankofcanada.ca")

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "cannot reach www.bankofcanada.ca", got.Rates.FetchError)
}

func Test_status_reads_no_fetch_error_when_only_an_earlier_run_had_one(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	setFetchError(t, st, 1, "cannot reach www.bankofcanada.ca")
	addImportRun(t, st, 2, "/snapshots/run-2.sqlite", 22)
	setFetchError(t, st, 2, nil)

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.Empty(t, got.Rates.FetchError)
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

func Test_status_carries_each_finding_with_its_state_from_the_latest_build(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)
	_, err = duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	got, err := duckstore.New(dir).Status(t.Context())

	require.NoError(t, err)
	states := make([]finding.State, 0, len(got.Findings))
	for _, f := range got.Findings {
		states = append(states, finding.State{ID: f.ID, Fixed: f.FixedAt != nil, New: f.New, NewlyFixed: f.NewlyFixed})
	}
	assert.Equal(t, []finding.State{
		{ID: "one-sided-transfer:xfer-3"},
		{ID: "uncategorized:no-payee", Fixed: true, NewlyFixed: true},
		{ID: "uncategorized:payee-1", Fixed: true, NewlyFixed: true},
	}, states)
}

func Test_status_returns_a_findings_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: fault, passQueries: 1}))

	_, err := st.Status(t.Context())

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_status_returns_a_findings_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{scanFault: errScanFailed, passQueries: 1}))

	_, err := st.Status(t.Context())

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}

func Test_status_reads_every_account_closed_included_sorted_by_id(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: "acct-3", SourceID: 3, Name: "Old RRSP", Type: "retirement", Currency: "CAD", Closed: true},
		store.Account{ID: "acct-2", SourceID: 2, Name: "Questrade TFSA", Type: "brokerage", Currency: "USD", Active: true},
	)
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), rows)
	require.NoError(t, err)

	got, err := duckstore.New(dir).Status(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []store.Account{
		{ID: "acct-1", Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
		{ID: "acct-2", Name: "Questrade TFSA", Type: "brokerage", Currency: "USD", Active: true},
		{ID: "acct-3", Name: "Old RRSP", Type: "retirement", Currency: "CAD", Closed: true},
	}, got.Accounts)
}

func Test_status_returns_an_accounts_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT id"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: fault, passQueries: 2}))

	_, err := st.Status(t.Context())

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_status_returns_an_account_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{scanFault: errScanFailed, passQueries: 2}))

	_, err := st.Status(t.Context())

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}
