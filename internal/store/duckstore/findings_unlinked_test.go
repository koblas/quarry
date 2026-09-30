package duckstore_test

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const unlinkedAmount = 50000

// unlinkedRows is minimalRows with the transactions replaced by txns, the closed CAD account acct-2 and the USD
// account acct-3 added, and no splits or transfers until mutate adds them.
func unlinkedRows(mutate func(*store.Rows), txns ...store.Transaction) store.Rows {
	rows := minimalRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: "acct-2", SourceID: 2, Name: "Old Visa", Type: "credit", Currency: "CAD", Closed: true, NotInReports: true},
		store.Account{ID: "acct-3", SourceID: 3, Name: "US Savings", Type: "savings", Currency: "USD"})
	rows.Transactions = txns
	rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil
	if mutate != nil {
		mutate(&rows)
	}
	return rows
}

// unlinkedIDs returns the unlinked-transfer finding ids of the store built from unlinkedRows, comma-joined in id order.
func unlinkedIDs(t *testing.T, mutate func(*store.Rows), txns ...store.Transaction) string {
	t.Helper()
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), unlinkedRows(mutate, txns...))
	require.NoError(t, err)
	var ids string
	require.NoError(t, openReadOnly(t, replaced.Path).QueryRows(t.Context(),
		`SELECT COALESCE(string_agg(id, ',' ORDER BY id), '') FROM findings WHERE type = 'unlinked-transfer'`, nil,
		func(scan func(dest ...any) error) error { return scan(&ids) }))
	return ids
}

func Test_replace_flags_opposite_amounts_in_two_accounts_up_to_three_days_apart_as_an_unlinked_transfer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// lowerIDApart is the lower-id transaction's date minus the higher-id one's, in days.
		lowerIDApart int
		want         string
	}{
		{"the same day", 0, "unlinked-transfer:txn-1+txn-2"},
		{"three days apart, the last day in", -3, "unlinked-transfer:txn-1+txn-2"},
		{"four days apart, the first day out", -4, ""},
		{"three days apart, the lower id dated later", 3, "unlinked-transfer:txn-1+txn-2"},
		{"four days apart, the lower id dated later", 4, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			start := day(2026, 8, 10)

			got := unlinkedIDs(t, nil,
				dupTxn(1, "acct-1", start.AddDate(0, 0, c.lowerIDApart), -unlinkedAmount, uncleared),
				dupTxn(2, "acct-2", start, unlinkedAmount, uncleared))

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_flags_an_unlinked_transfer_whose_lower_id_transaction_is_the_deposit(t *testing.T) {
	t.Parallel()

	got := unlinkedIDs(t, nil,
		dupTxn(1, "acct-1", day(2026, 8, 3), unlinkedAmount, uncleared),
		dupTxn(2, "acct-2", day(2026, 8, 6), -unlinkedAmount, uncleared))

	assert.Equal(t, "unlinked-transfer:txn-1+txn-2", got)
}

func Test_replace_does_not_flag_an_unlinked_transfer_between_transactions_that_are_not_opposite_amounts_in_two_accounts_of_one_currency(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		firstAccount string
		otherAccount string
		firstAmount  int64
		otherAmount  int64
	}{
		{"in the same account", "acct-1", "acct-1", -unlinkedAmount, unlinkedAmount},
		{"a cent apart", "acct-1", "acct-2", -unlinkedAmount, unlinkedAmount + 1},
		{"the same sign", "acct-1", "acct-2", -unlinkedAmount, -unlinkedAmount},
		{"zero amounts", "acct-1", "acct-2", 0, 0},
		{"a CAD and a USD account", "acct-1", "acct-3", -unlinkedAmount, unlinkedAmount},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := unlinkedIDs(t, nil,
				dupTxn(1, c.firstAccount, day(2026, 8, 3), c.firstAmount, uncleared),
				dupTxn(2, c.otherAccount, day(2026, 8, 4), c.otherAmount, uncleared))

			assert.Empty(t, got)
		})
	}
}

func Test_replace_flags_an_unlinked_transfer_in_every_kind_of_account_and_beside_unrelated_splits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*store.Rows)
		first  string
		other  string
	}{
		{"the same currency in both accounts", nil, "acct-1", "acct-2"},
		{"a closed account outside reports on the lower id side", nil, "acct-2", "acct-1"},
		{"two USD accounts", func(r *store.Rows) {
			r.Accounts = append(r.Accounts, store.Account{ID: "acct-4", SourceID: 4, Name: "US Cheque", Type: "chequing", Currency: "USD"})
		}, "acct-3", "acct-4"},
		{"categorized splits that are not transfer legs", func(r *store.Rows) {
			r.Splits = []store.Split{
				{ID: "split-1", SourceID: 1, TransactionID: "txn-1", CategoryID: new("cat-1"), Amount: -unlinkedAmount},
				{ID: "split-2", SourceID: 2, TransactionID: "txn-2", CategoryID: new("cat-1"), Amount: unlinkedAmount},
			}
		}, "acct-1", "acct-2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := unlinkedIDs(t, c.mutate,
				dupTxn(1, c.first, day(2026, 8, 3), -unlinkedAmount, uncleared),
				dupTxn(2, c.other, day(2026, 8, 4), unlinkedAmount, uncleared))

			assert.Equal(t, "unlinked-transfer:txn-1+txn-2", got)
		})
	}
}

func Test_replace_does_not_flag_an_unlinked_transfer_when_either_transaction_has_a_transfer_leg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*store.Rows)
	}{
		{"the lower id transaction's split is a transfers leg only", func(r *store.Rows) {
			r.Splits = []store.Split{{ID: "split-1", SourceID: 1, TransactionID: "txn-1", Amount: -unlinkedAmount}}
			r.Transfers = []store.Transfer{{ID: "xfer-1", FromSplitID: "split-1", ToSplitID: new("split-99")}}
		}},
		{"the higher id transaction's split is a transfers to-leg only", func(r *store.Rows) {
			r.Splits = []store.Split{{ID: "split-2", SourceID: 2, TransactionID: "txn-2", Amount: unlinkedAmount}}
			r.Transfers = []store.Transfer{{ID: "xfer-1", FromSplitID: "split-99", ToSplitID: new("split-2")}}
		}},
		{"the lower id transaction's split names a transfer account only", func(r *store.Rows) {
			r.Splits = []store.Split{{ID: "split-1", SourceID: 1, TransactionID: "txn-1", Amount: -unlinkedAmount, TransferAccountID: new("acct-9")}}
		}},
		{"the higher id transaction's split names a transfer account only", func(r *store.Rows) {
			r.Splits = []store.Split{{ID: "split-2", SourceID: 2, TransactionID: "txn-2", Amount: unlinkedAmount, TransferAccountID: new("acct-9")}}
		}},
		{"a second split of the transaction is a leg", func(r *store.Rows) {
			r.Splits = []store.Split{
				{ID: "split-1", SourceID: 1, TransactionID: "txn-1", CategoryID: new("cat-1"), Amount: -30000},
				{ID: "split-4", SourceID: 4, TransactionID: "txn-1", Amount: -20000, TransferAccountID: new("acct-9")},
			}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := unlinkedIDs(t, c.mutate,
				dupTxn(1, "acct-1", day(2026, 8, 3), -unlinkedAmount, uncleared),
				dupTxn(2, "acct-2", day(2026, 8, 4), unlinkedAmount, uncleared))

			assert.Empty(t, got)
		})
	}
}

func Test_replace_records_one_unlinked_transfer_per_pair_of_three_matching_transactions(t *testing.T) {
	t.Parallel()

	got := unlinkedIDs(t, func(r *store.Rows) {
		r.Accounts = append(r.Accounts, store.Account{ID: "acct-4", SourceID: 4, Name: "Second Visa", Type: "credit", Currency: "CAD"})
	},
		dupTxn(9, "acct-1", day(2026, 8, 3), -unlinkedAmount, uncleared),
		dupTxn(10, "acct-2", day(2026, 8, 4), unlinkedAmount, uncleared),
		dupTxn(11, "acct-4", day(2026, 8, 5), unlinkedAmount, uncleared))

	assert.Equal(t, "unlinked-transfer:txn-9+txn-10,unlinked-transfer:txn-9+txn-11", got)
}

func Test_replace_records_the_two_transactions_of_an_unlinked_transfer_as_its_items_lower_source_id_first(t *testing.T) {
	t.Parallel()
	rows := unlinkedRows(nil,
		dupTxn(10, "acct-2", day(2026, 8, 4), unlinkedAmount, uncleared),
		dupTxn(9, "acct-1", day(2026, 8, 3), -unlinkedAmount, uncleared))
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)
	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)

	assertScalar(t, db, `SELECT string_agg(COALESCE(transaction_id, 'NULL') || '|' || COALESCE(split_id, 'NULL') || '|' ||
		COALESCE(payee_id, 'NULL') || '|' || COALESCE(category_id, 'NULL'), '; ' ORDER BY rowid)
		FROM finding_items WHERE finding_id = 'unlinked-transfer:txn-9+txn-10'`,
		"txn-9|NULL|NULL|NULL; txn-10|NULL|NULL|NULL")
}
