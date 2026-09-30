package duckstore_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalRows fills every table (transfers: one paired, one one-sided) so a
// round trip covers each table and each nullable column set and NULL.
func minimalRows() store.Rows {
	return store.Rows{
		Accounts: []store.Account{{
			ID: "acct-1", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD",
			Institution: new("Big Bank"), Closed: false, Active: true,
		}},
		Categories: []store.Category{{
			ID: "cat-1", SourceID: 1, Name: "Groceries", FullPath: "Groceries", Kind: "expense", Hidden: false,
		}},
		Payees: []store.Payee{{ID: "payee-1", SourceID: 1, Name: "Coffee Shop"}},
		Tags:   []store.Tag{{ID: "tag-1", SourceID: 1, Name: "Reimbursable"}},
		Transactions: []store.Transaction{{
			ID: "txn-1", SourceID: 1, AccountID: "acct-1",
			Date: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), PayeeID: new("payee-1"), Memo: new("Beans"),
			Amount: 1234, Currency: "CAD", Status: "uncleared", ChequeNumber: new("101"),
		}},
		Splits: []store.Split{{
			ID: "split-1", SourceID: 1, TransactionID: "txn-1", CategoryID: new("cat-1"),
			Amount: 1234, Memo: new("split memo"),
		}, {
			ID: "split-4", SourceID: 4, TransactionID: "txn-1", CategoryID: new("cat-1"), Amount: 1234,
		}},
		SplitTags: []store.SplitTag{{SplitID: "split-1", TagID: "tag-1"}},
		Transfers: []store.Transfer{
			{ID: "xfer-1", FromSplitID: "split-1", ToSplitID: new("split-2"), CrossCurrency: true},
			{ID: "xfer-3", FromSplitID: "split-3"},
		},
		ImportRuns: []store.ImportRun{{
			ID: 1, StartedAt: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), FinishedAt: time.Date(2026, 9, 27, 14, 30, 7, 0, time.UTC),
			Snapshot: store.SnapshotRef{
				Path: "/snapshots/20260927T143005Z.sqlite", SHA256: "9f86", SchemaFingerprint: "sha256:abc",
				TakenAt: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), Source: "/Users/alex/Documents/Home.quicken",
			},
			Counts: store.Counts{
				Accounts: 1, Categories: 2, Payees: 3, Tags: 4, Transactions: 5, Splits: 6, SplitTags: 7, Transfers: 8,
			},
			BalancesChecked: 9, BalancesMismatched: 10, SplitsMismatched: 11, TransfersOneSided: 12, InvestmentTransactionsNotImported: 13,
			BalancesNeverReconciled: 14, InvestmentAccounts: 15, TransfersPaired: 16, TransfersCrossCurrency: 17,
		}},
	}
}

// Reads back via a fresh read-only connection: the only proof of the
// bytes on disk, not just the in-memory build.
func Test_replace_swaps_in_a_store_that_reads_back_every_row(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)

	path, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "quarry.duckdb"), path)

	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	assertScalar(t, db, "SELECT name FROM accounts WHERE id = 'acct-1'", "Chequing")
	assertScalar(t, db, "SELECT institution FROM accounts WHERE id = 'acct-1'", "Big Bank")
	assertScalar(t, db, "SELECT full_path FROM categories WHERE id = 'cat-1'", "Groceries")
	assertScalar(t, db, "SELECT name FROM payees WHERE id = 'payee-1'", "Coffee Shop")
	assertScalar(t, db, "SELECT name FROM tags WHERE id = 'tag-1'", "Reimbursable")
	assertScalar(t, db, "SELECT CAST(amount AS VARCHAR) FROM transactions WHERE id = 'txn-1'", "12.34")
	assertScalar(t, db, "SELECT CAST(amount AS VARCHAR) FROM splits WHERE id = 'split-1'", "12.34")
	assertScalar(t, db, "SELECT tag_id FROM split_tags WHERE split_id = 'split-1'", "tag-1")
	assertScalar(t, db, "SELECT to_split_id || ' ' || CAST(cross_currency AS VARCHAR) FROM transfers WHERE id = 'xfer-1'", "split-2 true")
	assertScalar(t, db, "SELECT from_split_id || ' ' || COALESCE(to_split_id, 'NULL') || ' ' || CAST(cross_currency AS VARCHAR) FROM transfers WHERE id = 'xfer-3'", "split-3 NULL false")
	assertScalar(t, db, "SELECT CAST(started_at AS VARCHAR) || ' ' || CAST(finished_at AS VARCHAR) FROM import_runs WHERE id = 1",
		"2026-09-27 14:30:05 2026-09-27 14:30:07")
	assertScalar(t, db, "SELECT concat_ws(' ', snapshot_path, snapshot_sha256, schema_fingerprint) FROM import_runs WHERE id = 1",
		"/snapshots/20260927T143005Z.sqlite 9f86 sha256:abc")
	assertScalar(t, db, "SELECT concat_ws(' ', accounts_rows, categories_rows, payees_rows, tags_rows, transactions_rows, splits_rows, "+
		"split_tags_rows, transfers_rows, balances_checked, balances_mismatched, splits_mismatched, transfers_one_sided, "+
		"investment_transactions_not_imported) FROM import_runs WHERE id = 1", "1 2 3 4 5 6 7 8 9 10 11 12 13")
	assertScalar(t, db, "SELECT concat_ws(' ', CAST(snapshot_taken_at AS VARCHAR), source_path, balances_never_reconciled, "+
		"investment_accounts, transfers_paired, transfers_cross_currency) FROM import_runs WHERE id = 1",
		"2026-09-27 14:30:05 /Users/alex/Documents/Home.quicken 14 15 16 17")
}

func Test_replace_stores_the_report_flags_and_the_posted_date(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Accounts = append(rows.Accounts, store.Account{
		ID: "acct-2", SourceID: 2, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true,
	})
	rows.Transactions[0].ExcludedFromReports = true
	rows.Transactions[0].PostedDate = new(time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-2", SourceID: 2, AccountID: "acct-1", Date: time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		Amount: 500, Currency: "CAD", Status: "uncleared",
	})

	path, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assertScalar(t, db, "SELECT CAST(in_reports AS VARCHAR) FROM accounts WHERE id = 'acct-1'", "true")
	assertScalar(t, db, "SELECT CAST(in_reports AS VARCHAR) FROM accounts WHERE id = 'acct-2'", "false")
	assertScalar(t, db, "SELECT CAST(excluded_from_reports AS VARCHAR) FROM transactions WHERE id = 'txn-1'", "true")
	assertScalar(t, db, "SELECT CAST(excluded_from_reports AS VARCHAR) FROM transactions WHERE id = 'txn-2'", "false")
	assertScalar(t, db, "SELECT CAST(posted_date AS VARCHAR) FROM transactions WHERE id = 'txn-1'", "2026-03-14")
	assertScalar(t, db, "SELECT COALESCE(CAST(posted_date AS VARCHAR), 'NULL') FROM transactions WHERE id = 'txn-2'", "NULL")
}

// The held reader reads nothing before Replace: a cached page would hide an overwrite of its file.
func Test_replace_leaves_a_held_reader_on_the_old_rows_and_a_later_read_sees_the_new(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	path, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	held, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Close() })
	renamed := minimalRows()
	renamed.Accounts[0].Name = "Savings"

	_, err = st.Replace(t.Context(), renamed)

	require.NoError(t, err)
	assertScalar(t, held, "SELECT name FROM accounts", "Chequing")
	require.NoError(t, held.Close())
	got, err := st.Query(t.Context(), "SELECT name FROM accounts", 0)
	require.NoError(t, err)
	assert.Equal(t, "Savings", got.Rows[0][0].Text)
}

func Test_replace_writes_one_store_info_row_with_the_format_version_and_build_time(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	before := time.Now().UTC().Truncate(time.Microsecond)

	path, err := duckstore.New(dir).Replace(t.Context(), minimalRows())

	after := time.Now().UTC()
	require.NoError(t, err)
	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assertScalar(t, db, "SELECT CAST(count(*) AS VARCHAR) FROM store_info", "1")
	assertScalar(t, db, "SELECT CAST(format_version AS VARCHAR) FROM store_info", strconv.Itoa(duckstore.FormatVersion))
	var builtAt time.Time
	require.NoError(t, db.QueryRows(t.Context(), "SELECT built_at FROM store_info", nil,
		func(scan func(dest ...any) error) error { return scan(&builtAt) }))
	assert.False(t, builtAt.Before(before))
	assert.False(t, builtAt.After(after))
}

func Test_replace_records_the_quarry_version_it_is_given(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	path, err := duckstore.New(dir, duckstore.WithQuarryVersion("v1.2.3")).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assertScalar(t, db, "SELECT quarry_version FROM store_info", "v1.2.3")
}

func Test_replace_records_devel_when_no_quarry_version_is_given(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts []duckstore.Option
	}{
		{name: "no option", opts: nil},
		{name: "an empty version", opts: []duckstore.Option{duckstore.WithQuarryVersion("")}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			path, err := duckstore.New(t.TempDir(), c.opts...).Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			db, err := duckdb.OpenReadOnly(t.Context(), path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			assertScalar(t, db, "SELECT quarry_version FROM store_info", "(devel)")
		})
	}
}

func Test_replace_stores_null_when_the_snapshot_has_no_taken_at_or_source(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.ImportRuns[0].Snapshot.TakenAt = time.Time{}
	rows.ImportRuns[0].Snapshot.Source = ""

	path, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assertScalar(t, db, "SELECT COALESCE(CAST(snapshot_taken_at AS VARCHAR), 'NULL') || ' / ' || COALESCE(source_path, 'NULL') FROM import_runs",
		"NULL / NULL")
}

func Test_replace_keeps_the_previous_store_when_store_info_cannot_be_written(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	st := newFaultStore(dir, &faultDB{appendFaultTable: "store_info", appendFault: &duckdbdriver.Error{
		Type: duckdbdriver.ErrorTypeConstraint, Msg: "Constraint Error: NOT NULL constraint failed: store_info.built_at",
	}})
	rows := minimalRows()
	rows.Transactions[0].Amount = 999

	_, err = st.Replace(t.Context(), rows)

	require.ErrorContains(t, err, "load store_info")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

// A sign-drop bug would only show here, not in the positive-amount
// coverage above.
func Test_replace_keeps_a_negative_amounts_sign(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rows := minimalRows()
	rows.Transactions[0].Amount = -1204
	st := duckstore.New(dir)

	path, err := st.Replace(t.Context(), rows)
	require.NoError(t, err)

	db, err := duckdb.OpenReadOnly(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assertScalar(t, db, "SELECT CAST(amount AS VARCHAR) FROM transactions WHERE id = 'txn-1'", "-12.04")
}

func Test_replace_leaves_no_partial_or_wal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"quarry.duckdb"}, names)
}

func Test_replace_makes_the_store_owner_only(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)

	path, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// duplicatePKRows gives two accounts the same id, so the build fails when
// the Appender flushes accounts' primary key constraint at Close.
func duplicatePKRows() store.Rows {
	rows := minimalRows()
	rows.Accounts = append(rows.Accounts, rows.Accounts[0])
	return rows
}

// One row per table, each corrupted by duplicating its only row while
// every earlier table stays valid.
func Test_replace_fails_when_any_tables_rows_fail_to_append(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		corrupt func(store.Rows) store.Rows
	}{
		{"accounts", func(r store.Rows) store.Rows { r.Accounts = append(r.Accounts, r.Accounts[0]); return r }},
		{"categories", func(r store.Rows) store.Rows { r.Categories = append(r.Categories, r.Categories[0]); return r }},
		{"payees", func(r store.Rows) store.Rows { r.Payees = append(r.Payees, r.Payees[0]); return r }},
		{"tags", func(r store.Rows) store.Rows { r.Tags = append(r.Tags, r.Tags[0]); return r }},
		{"transactions", func(r store.Rows) store.Rows { r.Transactions = append(r.Transactions, r.Transactions[0]); return r }},
		{"splits", func(r store.Rows) store.Rows { r.Splits = append(r.Splits, r.Splits[0]); return r }},
		{"split_tags", func(r store.Rows) store.Rows { r.SplitTags = append(r.SplitTags, r.SplitTags[0]); return r }},
		{"transfers", func(r store.Rows) store.Rows { r.Transfers = append(r.Transfers, r.Transfers[0]); return r }},
		{"import_runs", func(r store.Rows) store.Rows { r.ImportRuns = append(r.ImportRuns, r.ImportRuns[0]); return r }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := duckstore.New(t.TempDir())

			_, err := st.Replace(t.Context(), c.corrupt(minimalRows()))

			require.Error(t, err)
		})
	}
}

func Test_replace_fails_when_a_transactions_amount_is_out_of_range(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rows := minimalRows()
	rows.Transactions[0].Amount = math.MaxInt64
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), rows)

	require.Error(t, err)
}

func Test_replace_fails_when_a_splits_amount_is_out_of_range(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rows := minimalRows()
	rows.Splits[0].Amount = math.MaxInt64
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), rows)

	require.Error(t, err)
}

func Test_replace_removes_the_partial_when_the_build_fails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), duplicatePKRows())

	require.Error(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func Test_replace_leaves_the_existing_store_byte_identical_when_the_build_fails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)
	path, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	_, err = st.Replace(t.Context(), duplicatePKRows())

	require.Error(t, err)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func Test_replace_removes_the_partial_when_the_rename_fails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "quarry.duckdb")
	require.NoError(t, os.Mkdir(finalPath, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(finalPath, "occupied"), []byte("x"), 0o600))
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.Error(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
	info, err := os.Stat(finalPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func Test_replace_tags_a_read_only_store_directory_as_not_writable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)
	path, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	rows := minimalRows()
	rows.Transactions[0].Amount = 999

	_, err = st.Replace(t.Context(), rows)

	require.ErrorIs(t, err, store.ErrStoreNotWritable)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func Test_replace_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	st := duckstore.New(dir)

	_, err := st.Replace(ctx, minimalRows())

	require.ErrorIs(t, err, context.Canceled)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// faultDB wraps the real partial-file connection and injects at most one
// fault; with no fault configured it passes every call through.
type faultDB struct {
	duckstore.DB

	path             string
	checkpointFault  error
	afterCheckpoint  func()
	walOnClose       bool
	appendFaultTable string
	appendFault      error
}

// AppendRows fails with appendFault for appendFaultTable, else appends for real.
func (f *faultDB) AppendRows(ctx context.Context, table string, rows [][]any) error {
	if f.appendFault != nil && table == f.appendFaultTable {
		return fmt.Errorf("append %s: %w", table, f.appendFault)
	}
	return f.DB.AppendRows(ctx, table, rows)
}

// CheckpointClose returns checkpointFault wrapped as duckdb.CheckpointClose
// wraps a driver error, or runs the real one and then afterCheckpoint.
func (f *faultDB) CheckpointClose(ctx context.Context) error {
	if f.checkpointFault != nil {
		return fmt.Errorf("checkpoint %s: %w", f.path, f.checkpointFault)
	}
	err := f.DB.CheckpointClose(ctx)
	if f.afterCheckpoint != nil {
		f.afterCheckpoint()
	}
	return err
}

// Close closes the real connection, then leaves a .wal beside the partial
// when walOnClose is set, as a crash mid-checkpoint would.
func (f *faultDB) Close() error {
	err := f.DB.Close()
	if f.walOnClose {
		_ = os.WriteFile(f.path+".wal", []byte("wal"), 0o600)
	}
	return err
}

// newFaultStore returns a Store over dir that builds its partial file
// through f over a real DuckDB connection.
func newFaultStore(dir string, f *faultDB) *duckstore.Store {
	return duckstore.New(dir, duckstore.WithCreate(func(ctx context.Context, path string) (duckstore.DB, error) {
		db, err := duckdb.Create(ctx, path)
		if err != nil {
			return nil, err
		}
		f.DB, f.path = db, path
		return f, nil
	}))
}

func Test_replace_removes_the_partial_and_wal_when_the_checkpoint_fails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := newFaultStore(dir, &faultDB{
		checkpointFault: &duckdbdriver.Error{
			Type: duckdbdriver.ErrorTypeIO, Msg: `IO Error: Could not write file "quarry.duckdb.partial": No space left on device`,
		},
		walOnClose: true,
	})

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, store.ErrDiskFull)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, direntNames(entries))
}

func Test_replace_does_not_swap_when_the_context_ends_after_the_checkpoint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	st := newFaultStore(dir, &faultDB{afterCheckpoint: cancel})
	rows := minimalRows()
	rows.Transactions[0].Amount = 999

	_, err = st.Replace(ctx, rows)

	require.ErrorIs(t, err, context.Canceled)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func Test_replace_removes_a_stale_wal_before_swapping_in_the_new_store(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	staleWAL := filepath.Join(dir, "quarry.duckdb.wal")
	require.NoError(t, os.WriteFile(staleWAL, []byte("wal"), 0o600))
	st := duckstore.New(dir)

	path, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "quarry.duckdb"), path)
	_, statErr := os.Stat(staleWAL)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_replace_leaves_a_missing_wal_alone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)

	path, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "quarry.duckdb"), path)
}

func Test_replace_does_not_remove_the_stale_wal_before_the_context_gate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	staleWAL := filepath.Join(dir, "quarry.duckdb.wal")
	require.NoError(t, os.WriteFile(staleWAL, []byte("wal"), 0o600))
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	st := newFaultStore(dir, &faultDB{afterCheckpoint: cancel})
	rows := minimalRows()
	rows.Transactions[0].Amount = 999

	_, err = st.Replace(ctx, rows)

	require.ErrorIs(t, err, context.Canceled)
	_, statErr := os.Stat(staleWAL)
	require.NoError(t, statErr)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func Test_replace_refuses_the_swap_when_the_stale_wal_cannot_be_removed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	staleWAL := filepath.Join(dir, "quarry.duckdb.wal")
	require.NoError(t, os.Mkdir(staleWAL, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(staleWAL, "occupied"), []byte("x"), 0o600))
	st := duckstore.New(dir)

	_, err = st.Replace(t.Context(), minimalRows())

	require.Error(t, err)
	require.NotErrorIs(t, err, store.ErrStoreNotWritable)
	require.NotErrorIs(t, err, store.ErrDiskFull)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func Test_replace_does_not_tag_an_unrelated_build_failure(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())

	_, err := st.Replace(t.Context(), duplicatePKRows())

	require.Error(t, err)
	require.NotErrorIs(t, err, store.ErrStoreNotWritable)
	assert.NotErrorIs(t, err, store.ErrDiskFull)
}

var errCreateBoom = errors.New("create boom")

func Test_replace_fails_when_the_partial_file_cannot_be_created(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir, duckstore.WithCreate(func(context.Context, string) (duckstore.DB, error) {
		return nil, errCreateBoom
	}))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, errCreateBoom)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, direntNames(entries))
}

func Test_replace_creates_a_missing_store_directory_owner_only(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "quarry")
	st := duckstore.New(dir)

	path, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.FileExists(t, path)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func Test_replace_tags_a_store_directory_it_cannot_create_as_not_writable(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	st := duckstore.New(filepath.Join(parent, "quarry"))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, store.ErrStoreNotWritable)
}

func Test_store_path_is_where_replace_writes_the_store(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())

	path, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, path, st.Path())
}

func Test_store_exists_only_once_a_store_file_is_there(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	before := st.Exists()

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.False(t, before)
	assert.True(t, st.Exists())
}

// The store directory is a regular file, so stat fails with ENOTDIR, not not-found.
func Test_store_exists_treats_a_stat_fault_as_a_store(t *testing.T) {
	t.Parallel()
	notADir := filepath.Join(t.TempDir(), "quarry")
	require.NoError(t, os.WriteFile(notADir, []byte("not a directory"), 0o600))

	assert.True(t, duckstore.New(notADir).Exists())
}

// createLeavingPartial returns a create func that writes a partial and its
// .wal at the path it is given, as a racing run would, then fails with err.
func createLeavingPartial(t *testing.T, err error) func(context.Context, string) (duckstore.DB, error) {
	t.Helper()
	return func(_ context.Context, path string) (duckstore.DB, error) {
		require.NoError(t, os.WriteFile(path, []byte("partial"), 0o600))
		require.NoError(t, os.WriteFile(path+".wal", []byte("wal"), 0o600))
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
}

func Test_replace_names_its_build_file_with_its_process_id(t *testing.T) {
	t.Parallel()
	var created string
	st := duckstore.New(t.TempDir(), duckstore.WithCreate(func(_ context.Context, path string) (duckstore.DB, error) {
		created = filepath.Base(path)
		return nil, errCreateBoom
	}))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, errCreateBoom)
	assert.Regexp(t, `^\.quarry-\d{8}T\d{6}Z-`+strconv.Itoa(os.Getpid())+`\.duckdb\.partial$`, created)
}

// The other run's partial is fresh, so neither this run's cleanup nor its sweep may take it.
func Test_replace_leaves_a_concurrent_runs_live_partial_alone_when_its_own_create_fails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	other := filepath.Join(dir, ".quarry-20260927T143005Z-"+strconv.Itoa(os.Getpid()+1)+".duckdb.partial")
	require.NoError(t, os.WriteFile(other, []byte("live build"), 0o600))
	require.NoError(t, os.WriteFile(other+".wal", []byte("live wal"), 0o600))
	st := duckstore.New(dir, duckstore.WithCreate(createLeavingPartial(t, errCreateBoom)))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, errCreateBoom)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{filepath.Base(other), filepath.Base(other) + ".wal"}, direntNames(entries))
}

func Test_replace_reports_a_create_collision_as_an_untagged_build_failure(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir(), duckstore.WithCreate(createLeavingPartial(t, duckdb.ErrExists)))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, duckdb.ErrExists)
	require.NotErrorIs(t, err, store.ErrStoreNotWritable)
	assert.NotErrorIs(t, err, store.ErrDiskFull)
}

// The colliding partial and .wal sit at this run's own name, so they are this run's to remove.
func Test_replace_removes_its_own_partial_and_wal_after_a_create_collision(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir, duckstore.WithCreate(createLeavingPartial(t, duckdb.ErrExists)))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, duckdb.ErrExists)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, direntNames(entries))
}

func Test_replace_removes_the_partial_its_failed_create_left_behind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir, duckstore.WithCreate(createLeavingPartial(t, errCreateBoom)))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, errCreateBoom)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, direntNames(entries))
}

func direntNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

func assertScalar(t *testing.T, db *duckdb.DB, query, want string) {
	t.Helper()
	var got string
	err := db.QueryRows(t.Context(), query, nil, func(scan func(dest ...any) error) error {
		return scan(&got)
	})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
