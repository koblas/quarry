package duckstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

// minimalRows is one row per table, every ref column populated, so a round
// trip exercises every table and every nullable-but-set column.
func minimalRows() store.Rows {
	return store.Rows{
		Accounts: []store.Account{{
			ID: "acct-1", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD",
			Institution: strPtr("Big Bank"), Closed: false, Active: true,
		}},
		Categories: []store.Category{{
			ID: "cat-1", SourceID: 1, Name: "Groceries", FullPath: "Groceries", Kind: "expense", Hidden: false,
		}},
		Payees: []store.Payee{{ID: "payee-1", SourceID: 1, Name: "Coffee Shop"}},
		Tags:   []store.Tag{{ID: "tag-1", SourceID: 1, Name: "Reimbursable"}},
		Transactions: []store.Transaction{{
			ID: "txn-1", SourceID: 1, AccountID: "acct-1",
			Date: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), PayeeID: strPtr("payee-1"), Memo: strPtr("Beans"),
			Amount: 1234, Currency: "CAD", Status: "uncleared", ChequeNumber: strPtr("101"),
		}},
		Splits: []store.Split{{
			ID: "split-1", SourceID: 1, TransactionID: "txn-1", CategoryID: strPtr("cat-1"),
			Amount: 1234, Memo: strPtr("split memo"),
		}},
		SplitTags: []store.SplitTag{{SplitID: "split-1", TagID: "tag-1"}},
	}
}

// Proves every table, every column and negative money round-trip through
// the built file: reading back via a fresh read-only connection is the
// only proof the bytes on disk, not just the in-memory build, are correct.
func Test_replace_swaps_in_a_store_that_reads_back_every_row(t *testing.T) {
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
}

// A negative amount must round-trip through DECIMAL(18,2) with its sign
// intact: split from the positive-amount coverage above by one assertion
// shape (both prove exact-cents round trip, but a sign-drop bug would only
// show here).
func Test_replace_keeps_a_negative_amounts_sign(t *testing.T) {
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
	dir := t.TempDir()
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"quarry.duckdb"}, names)
}

func Test_replace_makes_the_store_owner_only(t *testing.T) {
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

func Test_replace_removes_the_partial_when_the_build_fails(t *testing.T) {
	dir := t.TempDir()
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), duplicatePKRows())

	require.Error(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func Test_replace_leaves_the_existing_store_byte_identical_when_the_build_fails(t *testing.T) {
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

func Test_replace_fails_in_a_read_only_store_directory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	st := duckstore.New(dir)

	_, err := st.Replace(t.Context(), minimalRows())

	require.Error(t, err)
}

func Test_replace_fails_when_the_context_is_already_cancelled(t *testing.T) {
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
