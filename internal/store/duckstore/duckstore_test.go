package duckstore_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reads back via a fresh read-only connection: the only proof of the
// bytes on disk, not just the in-memory build.
func Test_replace_swaps_in_a_store_that_reads_back_every_row(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	path := replaced.Path
	assert.Equal(t, filepath.Join(dir, "quarry.duckdb"), path)

	db := openReadOnly(t, path)

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
	assertScalar(t, db, "SELECT COALESCE(other_account, 'NULL') FROM transfers WHERE id = 'xfer-1'", "NULL")
	assertScalar(t, db, "SELECT other_account FROM transfers WHERE id = 'xfer-3'", "Savings")
	assertScalar(t, db, "SELECT CAST(started_at AS VARCHAR) || ' ' || CAST(finished_at AS VARCHAR) FROM import_runs WHERE id = 1",
		"2026-09-27 14:30:05 2026-09-27 14:30:07")
	assertScalar(t, db, "SELECT concat_ws(' ', snapshot_path, snapshot_sha256, schema_fingerprint) FROM import_runs WHERE id = 1",
		"/snapshots/20260927T143005Z.sqlite 9f86 sha256:abc")
	assertScalar(t, db, "SELECT concat_ws(' ', accounts_rows, categories_rows, payees_rows, tags_rows, transactions_rows, splits_rows, "+
		"split_tags_rows, transfers_rows, balances_checked, balances_mismatched, splits_mismatched, transfers_one_sided) "+
		"FROM import_runs WHERE id = 1", "1 2 3 4 5 6 7 8 9 10 11 12")
	assertScalar(t, db, "SELECT concat_ws(' ', CAST(snapshot_taken_at AS VARCHAR), source_path, balances_never_reconciled, "+
		"investment_accounts, transfers_paired, transfers_cross_currency) FROM import_runs WHERE id = 1",
		"2026-09-27 14:30:05 /Users/alex/Documents/Home.quicken 14 15 16 17")
	assertScalar(t, db, "SELECT concat_ws(' ', securities_rows, prices_rows, investment_transactions_rows) FROM import_runs WHERE id = 1", "18 19 20")
	assertScalar(t, db, "SELECT CAST(shares_checked AS VARCHAR) FROM import_runs WHERE id = 1", "21")
	assertScalar(t, db, "SELECT concat_ws(' ', source_id, name, ticker, currency) FROM securities WHERE id = 'sec-1'", "1 Acme Corp ACME CAD")
	assertScalar(t, db, "SELECT concat_ws(' ', source_id, CAST(date AS VARCHAR), CAST(price AS VARCHAR)) FROM prices WHERE security_id = 'sec-1'",
		"7 2026-03-15 12.345678")
}

func Test_replace_stores_an_investment_transaction_with_every_nullable_column_set(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, "SELECT concat_ws(' ', source_id, account_id, security_id, CAST(date AS VARCHAR), action, CAST(shares AS VARCHAR), "+
		"CAST(amount AS VARCHAR), CAST(commission AS VARCHAR), CAST(cost_basis AS VARCHAR), currency, memo, CAST(split_new_shares AS VARCHAR), "+
		"CAST(split_old_shares AS VARCHAR)) FROM investment_transactions WHERE id = 'inv-1'",
		"21 acct-1 sec-1 2026-03-16 split 1.500000 123.45 8.4998 1000.50 CAD note 12.000000 1.000000")
}

func Test_replace_stores_an_investment_transaction_with_every_nullable_column_null(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, "SELECT concat_ws(' ', source_id, action, CAST(amount AS VARCHAR), currency) FROM investment_transactions WHERE id = 'inv-2'",
		"22 dividend 5.00 CAD")
	assertScalar(t, db, "SELECT CAST(count(*) AS VARCHAR) FROM investment_transactions WHERE id = 'inv-2' AND security_id IS NULL AND shares IS NULL "+
		"AND commission IS NULL AND cost_basis IS NULL AND memo IS NULL AND split_new_shares IS NULL AND split_old_shares IS NULL", "1")
}

func Test_replace_stores_the_investment_transaction_id_of_a_cash_row_and_null_for_a_register_row(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-2", SourceID: 2, AccountID: "acct-1", Date: time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
		Amount: 500, Currency: "CAD", Status: "uncleared", InvestmentTransactionID: new("inv-2"),
	})
	st := duckstore.New(t.TempDir())

	_, err := st.Replace(t.Context(), rows)

	require.NoError(t, err)
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, "SELECT COALESCE(investment_transaction_id, 'NULL') FROM transactions WHERE id = 'txn-1'", "NULL")
	assertScalar(t, db, "SELECT investment_transaction_id FROM transactions WHERE id = 'txn-2'", "inv-2")
}

func Test_replace_stores_a_security_with_no_ticker_or_currency_as_null(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, "SELECT concat_ws('|', ticker IS NULL, currency IS NULL) FROM securities WHERE id = 'sec-2'", "true|true")
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

	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	path := replaced.Path
	db := openReadOnly(t, path)
	assertScalar(t, db, "SELECT CAST(in_reports AS VARCHAR) FROM accounts WHERE id = 'acct-1'", "true")
	assertScalar(t, db, "SELECT CAST(in_reports AS VARCHAR) FROM accounts WHERE id = 'acct-2'", "false")
	assertScalar(t, db, "SELECT CAST(excluded_from_reports AS VARCHAR) FROM transactions WHERE id = 'txn-1'", "true")
	assertScalar(t, db, "SELECT CAST(excluded_from_reports AS VARCHAR) FROM transactions WHERE id = 'txn-2'", "false")
	assertScalar(t, db, "SELECT CAST(posted_date AS VARCHAR) FROM transactions WHERE id = 'txn-1'", "2026-03-14")
	assertScalar(t, db, "SELECT COALESCE(CAST(posted_date AS VARCHAR), 'NULL') FROM transactions WHERE id = 'txn-2'", "NULL")
}

func Test_replace_stores_linked_tracking_per_account(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Accounts = append(rows.Accounts, store.Account{
		ID: "acct-2", SourceID: 2, Name: "Netskope 401(k)", Type: "retirement", Currency: "USD", Active: true, LinkedTracking: true,
	})

	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	path := replaced.Path
	db := openReadOnly(t, path)
	assertScalar(t, db, "SELECT CAST(linked_tracking AS VARCHAR) FROM accounts WHERE id = 'acct-1'", "false")
	assertScalar(t, db, "SELECT CAST(linked_tracking AS VARCHAR) FROM accounts WHERE id = 'acct-2'", "true")
	assertScalar(t, db, "SELECT CAST(in_reports AS VARCHAR) FROM accounts WHERE id = 'acct-2'", "true")
}

// The held reader reads nothing before Replace: a cached page would hide an overwrite of its file.
func Test_replace_leaves_a_held_reader_on_the_old_rows_and_a_later_read_sees_the_new(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	replaced, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	path := replaced.Path
	held := openReadOnly(t, path)
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

	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())

	after := time.Now().UTC()
	require.NoError(t, err)
	path := replaced.Path
	db := openReadOnly(t, path)
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

	replaced, err := duckstore.New(dir, duckstore.WithQuarryVersion("v1.2.3")).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	path := replaced.Path
	db := openReadOnly(t, path)
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

			replaced, err := duckstore.New(t.TempDir(), c.opts...).Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			path := replaced.Path
			db := openReadOnly(t, path)
			assertScalar(t, db, "SELECT quarry_version FROM store_info", "(devel)")
		})
	}
}

func Test_replace_stores_null_when_the_snapshot_has_no_taken_at_or_source(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.ImportRuns[0].Snapshot.TakenAt = time.Time{}
	rows.ImportRuns[0].Snapshot.Source = ""

	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	path := replaced.Path
	db := openReadOnly(t, path)
	assertScalar(t, db, "SELECT COALESCE(CAST(snapshot_taken_at AS VARCHAR), 'NULL') || ' / ' || COALESCE(source_path, 'NULL') FROM import_runs",
		"NULL / NULL")
}

func Test_replace_keeps_the_previous_store_when_store_info_cannot_be_written(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	path := replaced.Path
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

	replaced, err := st.Replace(t.Context(), rows)
	require.NoError(t, err)
	path := replaced.Path

	db := openReadOnly(t, path)
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

	replaced, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	path := replaced.Path

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

// One row per table but import_runs (the store numbers its ids), each corrupted by
// duplicating its only row while every earlier table stays valid.
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
		{"securities", func(r store.Rows) store.Rows { r.Securities = append(r.Securities, r.Securities[0]); return r }},
		{"prices", func(r store.Rows) store.Rows { r.Prices = append(r.Prices, r.Prices[0]); return r }},
		{"investment_transactions", func(r store.Rows) store.Rows {
			r.InvestmentTransactions = append(r.InvestmentTransactions, r.InvestmentTransactions[0])
			return r
		}},
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

func Test_replace_fails_when_import_runs_cannot_be_written(t *testing.T) {
	t.Parallel()
	st := newFaultStore(t.TempDir(), &faultDB{appendFaultTable: "import_runs", appendFault: &duckdbdriver.Error{
		Type: duckdbdriver.ErrorTypeConstraint, Msg: "Constraint Error: Duplicate key \"id: 1\" violates primary key constraint",
	}})

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorContains(t, err, "load import_runs")
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

func Test_replace_stores_the_largest_price_a_decimal_18_6_column_holds(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Prices[0].Price = 999_999_999_999_999_999
	st := duckstore.New(t.TempDir())

	_, err := st.Replace(t.Context(), rows)

	require.NoError(t, err)
	assertScalar(t, openReadOnly(t, st.Path()), "SELECT CAST(price AS VARCHAR) FROM prices", "999999999999.999999")
}

func Test_replace_fails_when_a_price_is_out_of_range(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Prices[0].Price = 1_000_000_000_000_000_000
	st := duckstore.New(t.TempDir())

	_, err := st.Replace(t.Context(), rows)

	require.ErrorContains(t, err, "price of sec-1 on 2026-03-15")
}

func Test_replace_stores_the_largest_investment_amounts_and_shares_the_columns_hold(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	inv := &rows.InvestmentTransactions[0]
	inv.Shares, inv.SplitNewShares, inv.SplitOldShares = new(int64(999_999_999_999_999_999)), new(int64(999_999_999_999_999_999)), new(int64(999_999_999_999_999_999))
	inv.Amount, inv.Commission, inv.CostBasis = 999_999_999_999_999_999, new(int64(-999_999_999_999_999_999)), new(int64(999_999_999_999_999_999))
	st := duckstore.New(t.TempDir())

	_, err := st.Replace(t.Context(), rows)

	require.NoError(t, err)
	assertScalar(t, openReadOnly(t, st.Path()), "SELECT concat_ws(' ', CAST(shares AS VARCHAR), CAST(amount AS VARCHAR), CAST(commission AS VARCHAR), "+
		"CAST(cost_basis AS VARCHAR), CAST(split_new_shares AS VARCHAR), CAST(split_old_shares AS VARCHAR)) FROM investment_transactions WHERE id = 'inv-1'",
		"999999999999.999999 9999999999999999.99 -99999999999999.9999 9999999999999999.99 999999999999.999999 999999999999.999999")
}

func Test_replace_fails_when_an_investment_transaction_value_is_out_of_range(t *testing.T) {
	t.Parallel()
	const beyond18Digits, beyondAmount = 1_000_000_000_000_000_000, math.MaxInt64
	cases := []struct {
		name    string
		corrupt func(*store.InvestmentTransaction)
		column  string
	}{
		{"shares", func(i *store.InvestmentTransaction) { i.Shares = new(int64(beyond18Digits)) }, "shares"},
		{"amount", func(i *store.InvestmentTransaction) { i.Amount = beyondAmount }, "amount"},
		{"commission", func(i *store.InvestmentTransaction) { i.Commission = new(int64(beyond18Digits)) }, "commission"},
		{"cost basis", func(i *store.InvestmentTransaction) { i.CostBasis = new(int64(beyond18Digits)) }, "cost_basis"},
		{"split new shares", func(i *store.InvestmentTransaction) { i.SplitNewShares = new(int64(-beyond18Digits)) }, "split_new_shares"},
		{"split old shares", func(i *store.InvestmentTransaction) { i.SplitOldShares = new(int64(beyond18Digits)) }, "split_old_shares"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := minimalRows()
			c.corrupt(&rows.InvestmentTransactions[0])

			_, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

			require.ErrorContains(t, err, "investment transaction inv-1: "+c.column)
		})
	}
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
	replaced, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	path := replaced.Path
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
	replaced, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	path := replaced.Path
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
	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	path := replaced.Path
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

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	path := replaced.Path
	assert.Equal(t, filepath.Join(dir, "quarry.duckdb"), path)
	_, statErr := os.Stat(staleWAL)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_replace_leaves_a_missing_wal_alone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := duckstore.New(dir)

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	path := replaced.Path
	assert.Equal(t, filepath.Join(dir, "quarry.duckdb"), path)
}

func Test_replace_does_not_remove_the_stale_wal_before_the_context_gate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	path := replaced.Path
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
	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	path := replaced.Path
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

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	path := replaced.Path
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

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	path := replaced.Path
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

var (
	errTwoLines = errors.New("connector refused\nsecond line")
	errEmpty    = errors.New("")
)

// storeInfoDDL creates store_info holding one row per format version given.
func storeInfoDDL(versions ...string) string {
	var ddl strings.Builder
	ddl.WriteString("CREATE TABLE store_info (format_version INTEGER, quarry_version VARCHAR, built_at TIMESTAMP);")
	for _, v := range versions {
		ddl.WriteString("INSERT INTO store_info VALUES (" + v + ", '(devel)', now());")
	}
	return ddl.String()
}

// openRefusal is the *store.OpenError a read of st refuses with.
func openRefusal(t *testing.T, st *duckstore.Store) *store.OpenError {
	t.Helper()
	_, err := st.Status(t.Context())
	openErr, ok := errors.AsType[*store.OpenError](err)
	require.True(t, ok, "want *store.OpenError, got %v", err)
	return openErr
}

func Test_open_read_classifies_each_store_file_it_cannot_read(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		arrange func(t *testing.T, dir string)
		want    store.OpenFault
	}{
		{name: "no file", arrange: func(*testing.T, string) {}, want: store.OpenFaultMissing},
		{
			name: "a file that is not a database",
			arrange: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, duckstore.FileName), []byte("text\n"), 0o600))
			},
			want: store.OpenFaultNotDuckDB,
		},
		{
			name: "a file the process may not read",
			arrange: func(t *testing.T, dir string) {
				t.Helper()
				skipAsRoot(t)
				path := filepath.Join(dir, duckstore.FileName)
				require.NoError(t, os.WriteFile(path, []byte("text\n"), 0o600))
				require.NoError(t, os.Chmod(path, 0o000))
			},
			want: store.OpenFaultPermission,
		},
		{
			name: "a directory the process may not search",
			arrange: func(t *testing.T, dir string) {
				t.Helper()
				skipAsRoot(t)
				require.NoError(t, os.WriteFile(filepath.Join(dir, duckstore.FileName), []byte("text\n"), 0o600))
				require.NoError(t, os.Chmod(dir, 0o000))
				t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			},
			want: store.OpenFaultPermission,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			c.arrange(t, dir)

			got := openRefusal(t, duckstore.New(dir))

			assert.Equal(t, c.want, got.Fault)
		})
	}
}

func Test_open_read_classifies_each_fault_the_open_returns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault error
		want  store.OpenFault
	}{
		{
			name: "another process holds the file",
			fault: driverIOError(`IO Error: Could not set lock on file "x.duckdb": Conflicting lock is held in ` +
				`/usr/local/bin/quarry (PID 42) by user dave. See also https://duckdb.org/docs/stable/connect/concurrency`),
			want: store.OpenFaultLocked,
		},
		{
			name:  "an OS permission fault",
			fault: &fs.PathError{Op: "open", Path: "x", Err: fs.ErrPermission},
			want:  store.OpenFaultPermission,
		},
		{
			name:  "the driver's missing-database text while the file is there",
			fault: driverIOError(`IO Error: Cannot open database "x.duckdb" in read-only mode: database does not exist`),
			want:  store.OpenFaultOther,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, failingOpener(c.fault))

			got := openRefusal(t, st)

			assert.Equal(t, c.want, got.Fault)
		})
	}
}

func Test_open_read_refuses_a_store_removed_between_the_check_and_the_open(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, duckstore.WithOpenReadOnly(func(_ context.Context, path string) (duckstore.ReadDB, error) {
		require.NoError(t, os.Remove(path))
		return nil, driverIOError(`IO Error: Cannot open database "x.duckdb" in read-only mode: database does not exist`)
	}))

	got := openRefusal(t, st)

	assert.Equal(t, store.OpenFaultMissing, got.Fault)
}

func Test_open_read_refuses_a_store_of_another_format(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ddl  string
	}{
		{name: "no store_info", ddl: importRunsDDL},
		{name: "an older format", ddl: importRunsDDL + storeInfoDDL(strconv.Itoa(duckstore.FormatVersion-1))},
		{name: "a newer format", ddl: importRunsDDL + storeInfoDDL(strconv.Itoa(duckstore.FormatVersion+1))},
		{name: "a NULL format", ddl: importRunsDDL + storeInfoDDL("NULL")},
		{name: "store_info with no row", ddl: importRunsDDL + storeInfoDDL()},
		{
			name: "store_info with two rows of this format",
			ddl:  importRunsDDL + storeInfoDDL(strconv.Itoa(duckstore.FormatVersion), strconv.Itoa(duckstore.FormatVersion)),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreFile(t, c.ddl)

			got := openRefusal(t, st)

			assert.Equal(t, store.OpenError{
				Fault: store.OpenFaultOtherFormat, Path: st.Path(), SnapshotPath: snapshotFile, Err: got.Err,
			}, *got)
		})
	}
}

func Test_open_read_names_a_snapshot_only_when_import_runs_yields_one(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ddl  string
		want string
	}{
		{name: "one import run", ddl: importRunsDDL, want: snapshotFile},
		{name: "no import_runs table", ddl: "CREATE TABLE accounts (id VARCHAR);", want: ""},
		{name: "no snapshot_path column", ddl: "CREATE TABLE import_runs (id BIGINT); INSERT INTO import_runs VALUES (1);", want: ""},
		{name: "no import run", ddl: "CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR);", want: ""},
		{name: "a NULL snapshot path", ddl: "CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR); INSERT INTO import_runs VALUES (1, NULL);", want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreFile(t, c.ddl)

			got := openRefusal(t, st)

			assert.Equal(t, c.want, got.SnapshotPath)
		})
	}
}

func Test_open_read_names_the_latest_import_runs_snapshot(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsDDL+
		"INSERT INTO import_runs VALUES (3, '/snapshots/run-3.sqlite'); INSERT INTO import_runs VALUES (2, '/snapshots/run-2.sqlite');")

	got := openRefusal(t, st)

	assert.Equal(t, "/snapshots/run-3.sqlite", got.SnapshotPath)
}

// The store's directory is reached through a symlink, so the path given and the path the driver names differ on every OS.
func Test_open_read_reports_other_faults_naming_the_store_by_its_given_path(t *testing.T) {
	t.Parallel()
	realDir := t.TempDir()
	_, err := duckstore.New(realDir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	linkDir := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(realDir, linkDir))
	given := filepath.Join(linkDir, duckstore.FileName)
	resolved, err := filepath.EvalSymlinks(given)
	require.NoError(t, err)
	st := duckstore.New(linkDir, failingOpener(driverIOError(`IO Error: Cannot open file "`+resolved+`": Input/output error`)))

	got := openRefusal(t, st)

	assert.Equal(t, store.OpenFaultOther, got.Fault)
	assert.Equal(t, `Cannot open file "`+given+`": Input/output error`, got.Reason)
}

func Test_open_read_reports_other_faults_with_a_one_line_reason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault error
		want  string
	}{
		{
			name:  "the driver's first line without its error type",
			fault: driverIOError("IO Error: Could not read from file: Is a directory\nmore detail"),
			want:  "Could not read from file: Is a directory",
		},
		{
			name:  "only the OS reason of a path error",
			fault: &fs.PathError{Op: "open", Path: "/elsewhere/quarry.duckdb", Err: syscall.EIO},
			want:  syscall.EIO.Error(),
		},
		{name: "the first line of any other error", fault: errTwoLines, want: "connector refused"},
		{name: "a driver message of only its error type", fault: driverIOError("IO Error: "), want: "unknown error"},
		{name: "an empty error", fault: errEmpty, want: "unknown error"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, failingOpener(c.fault))

			got := openRefusal(t, st)

			assert.Equal(t, c.want, got.Reason)
		})
	}
}

func Test_open_read_reports_a_failed_format_check_as_another_fault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
	}{
		{name: "the catalog query", query: duckstore.ColumnExistsQuery},
		{name: "the format_version read", query: duckstore.FormatVersionQuery},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			fault := ioFault(`query rows "SELECT"`)
			st := newBuiltStore(t, spyOpener(&spyReadDB{checkFaults: map[string]error{c.query: fault}}))

			got := openRefusal(t, st)

			assert.Equal(t, store.OpenFaultOther, got.Fault)
			assert.ErrorIs(t, got, fault)
		})
	}
}

func Test_open_read_refuses_another_format_without_a_snapshot_when_its_read_fails(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{checkFaults: map[string]error{duckstore.SnapshotPathQuery: ioFault(`query rows "SELECT"`)}}
	st := newStoreFile(t, importRunsDDL, spyOpener(spy))

	got := openRefusal(t, st)

	assert.Equal(t, store.OpenFaultOtherFormat, got.Fault)
	assert.Empty(t, got.SnapshotPath)
}

func Test_open_read_closes_the_connection_when_it_refuses_the_store(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		ddl   string
		spy   *spyReadDB
		fault store.OpenFault
	}{
		{name: "another format", ddl: importRunsDDL, spy: &spyReadDB{}, fault: store.OpenFaultOtherFormat},
		{
			name: "a failed format check", ddl: importRunsDDL + storeInfoDDL(strconv.Itoa(duckstore.FormatVersion)),
			spy:   &spyReadDB{checkFaults: map[string]error{duckstore.FormatVersionQuery: errQueryFailed}},
			fault: store.OpenFaultOther,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreFile(t, c.ddl, spyOpener(c.spy))

			got := openRefusal(t, st)

			require.Equal(t, c.fault, got.Fault)
			assert.Equal(t, 1, c.spy.closes)
		})
	}
}

func Test_replace_removes_a_leftover_partial_just_past_the_age_gate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		file string
	}{
		{"per-run name", ".quarry-20260101T000000Z-4242.duckdb.partial"},
		{"per-run name's wal", ".quarry-20260101T000000Z-4242.duckdb.partial.wal"},
		{"seconds-only name from an older release", ".quarry-20260101T000000Z.duckdb.partial"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			leftover := filepath.Join(dir, c.file)
			require.NoError(t, os.WriteFile(leftover, []byte("stale"), 0o600))
			old := time.Now().Add(-61 * time.Minute)
			require.NoError(t, os.Chtimes(leftover, old, old))
			st := duckstore.New(dir)

			_, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			_, statErr := os.Stat(leftover)
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func Test_replace_leaves_a_leftover_partial_just_inside_the_age_gate_alone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	leftover := filepath.Join(dir, ".quarry-20260101T000000Z.duckdb.partial")
	require.NoError(t, os.WriteFile(leftover, []byte("stale"), 0o600))
	recent := time.Now().Add(-59 * time.Minute)
	require.NoError(t, os.Chtimes(leftover, recent, recent))
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	_, statErr := os.Stat(leftover)
	assert.NoError(t, statErr)
}

// Each case is old enough for the age gate alone to remove it, so only the
// pattern's precision keeps it.
func Test_replace_sweep_leaves_near_miss_and_unrelated_files_alone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		file string
	}{
		{"no .partial suffix", ".quarry-20260927T143005Z.duckdb"},
		{"no leading dot", "quarry-20260927T143005Z.duckdb.partial"},
		{"trailing suffix after partial", ".quarry-20260927T143005Z.duckdb.partial.bak"},
		{"extra prefix before the dot", "x.quarry-20260927T143005Z.duckdb.partial"},
		{"sqlite wal suffix, not duckdb's", ".quarry-20260927T143005Z.duckdb.partial-wal"},
		{"snapshot-shaped, wrong extension", ".20260927T143005Z.sqlite.partial"},
		{"empty process id", ".quarry-20260927T143005Z-.duckdb.partial"},
		{"non-numeric process id", ".quarry-20260927T143005Z-12a.duckdb.partial"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, c.file)
			require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
			old := time.Now().Add(-2 * time.Hour)
			require.NoError(t, os.Chtimes(path, old, old))
			st := duckstore.New(dir)

			_, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			_, statErr := os.Stat(path)
			assert.NoError(t, statErr)
		})
	}
}

// The directory's own name matches buildFilePattern, so only the
// entry.IsDir() check (not the pattern) keeps it.
func Test_replace_sweep_ignores_subdirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, ".quarry-20200101T000000Z.duckdb.partial")
	require.NoError(t, os.Mkdir(sub, 0o700))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(sub, old, old))
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	info, statErr := os.Stat(sub)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// The pattern must never match the live store's own files: unlike a
// leftover, quarry.duckdb(.wal) is the store still in place, and the sweep
// runs before a build that can still fail.
func Test_replace_sweep_never_matches_the_live_stores_own_files(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	storeFile := filepath.Join(dir, "quarry.duckdb")
	storeWAL := filepath.Join(dir, "quarry.duckdb.wal")
	require.NoError(t, os.WriteFile(storeFile, []byte("live store"), 0o600))
	require.NoError(t, os.WriteFile(storeWAL, []byte("live wal"), 0o600))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(storeFile, old, old))
	require.NoError(t, os.Chtimes(storeWAL, old, old))
	st := duckstore.New(dir, duckstore.WithCreate(func(context.Context, string) (duckstore.DB, error) {
		return nil, errCreateBoom
	}))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, errCreateBoom)
	after, statErr := os.ReadFile(storeFile)
	require.NoError(t, statErr)
	assert.Equal(t, []byte("live store"), after)
	afterWAL, statErr := os.ReadFile(storeWAL)
	require.NoError(t, statErr)
	assert.Equal(t, []byte("live wal"), afterWAL)
}

// A concurrent sync's own in-flight partial matches the pattern but is not
// yet aged; the sweep must not race it.
func Test_replace_leaves_a_fresh_partial_alone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fresh := filepath.Join(dir, ".quarry-20260927T143005Z.duckdb.partial")
	require.NoError(t, os.WriteFile(fresh, []byte("in flight"), 0o600))
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	_, statErr := os.Stat(fresh)
	assert.NoError(t, statErr)
}

// dir is write+execute only, so the sweep's own os.ReadDir fails; WithCreate
// is wired to a distinct forced error, independent of the filesystem.
func Test_replace_ignores_a_sweep_that_cannot_list_the_directory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o300))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	st := duckstore.New(dir, duckstore.WithCreate(func(context.Context, string) (duckstore.DB, error) {
		return nil, errCreateBoom
	}))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, errCreateBoom)
}

// dir is chmoded read-only, WithCreate wired to a distinct forced error:
// both faults independent, only the sweep's must never surface.
func Test_replace_leaves_a_leftover_alone_when_the_sweep_cannot_remove_it(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	leftover := filepath.Join(dir, ".quarry-20260101T000000Z.duckdb.partial")
	require.NoError(t, os.WriteFile(leftover, []byte("stale"), 0o600))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(leftover, old, old))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	st := duckstore.New(dir, duckstore.WithCreate(func(context.Context, string) (duckstore.DB, error) {
		return nil, errCreateBoom
	}))

	_, err := st.Replace(t.Context(), minimalRows())

	require.ErrorIs(t, err, errCreateBoom)
	_, statErr := os.Stat(leftover)
	assert.NoError(t, statErr)
}
