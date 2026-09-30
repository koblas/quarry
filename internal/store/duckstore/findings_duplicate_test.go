package duckstore_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	duplicateAmount = -14217
	reconciled      = "reconciled"
	uncleared       = "uncleared"
)

// dupTxn is transaction txn-n in account, dated day, for amount; its source id is n so pair ids sort numerically.
func dupTxn(n int64, account string, date time.Time, amount int64, status string) store.Transaction {
	return store.Transaction{
		ID: fmt.Sprintf("txn-%d", n), SourceID: n, AccountID: account, Date: date, Amount: amount, Currency: "CAD", Status: status,
	}
}

// duplicateIDs replaces the transactions of minimalRows with txns (and the extra closed account acct-2) and returns the
// duplicate finding ids of the built store, comma-joined in id order.
func duplicateIDs(t *testing.T, mutate func(*store.Rows), txns ...store.Transaction) string {
	t.Helper()
	rows := minimalRows()
	rows.Accounts = append(rows.Accounts, store.Account{
		ID: "acct-2", SourceID: 2, Name: "Old Visa", Type: "credit", Currency: "CAD", Closed: true, NotInReports: true,
	})
	rows.Transactions = txns
	rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil
	if mutate != nil {
		mutate(&rows)
	}
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)
	require.NoError(t, err)
	var ids string
	require.NoError(t, openReadOnly(t, replaced.Path).QueryRows(t.Context(),
		`SELECT COALESCE(string_agg(id, ',' ORDER BY id), '') FROM findings WHERE type = 'duplicate'`, nil,
		func(scan func(dest ...any) error) error { return scan(&ids) }))
	return ids
}

func Test_replace_flags_two_same_amount_transactions_up_to_three_days_apart_as_a_duplicate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// lowerIDApart is the lower-id transaction's date minus the higher-id one's, in days.
		lowerIDApart int
		want         string
	}{
		{"the same day", 0, "duplicate:txn-1+txn-2"},
		{"three days apart, the last day in", -3, "duplicate:txn-1+txn-2"},
		{"four days apart, the first day out", -4, ""},
		{"three days apart, the lower id dated later", 3, "duplicate:txn-1+txn-2"},
		{"four days apart, the lower id dated later", 4, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			start := day(2026, 8, 10)

			got := duplicateIDs(t, nil,
				dupTxn(1, "acct-1", start.AddDate(0, 0, c.lowerIDApart), duplicateAmount, uncleared),
				dupTxn(2, "acct-1", start, duplicateAmount, uncleared))

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_flags_a_pair_with_one_reconciled_transaction_but_not_a_reconciled_pair(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		first, other string
		want         string
	}{
		{"neither reconciled", uncleared, uncleared, "duplicate:txn-1+txn-2"},
		{"only the first reconciled", reconciled, uncleared, "duplicate:txn-1+txn-2"},
		{"only the second reconciled", uncleared, reconciled, "duplicate:txn-1+txn-2"},
		{"both reconciled", reconciled, reconciled, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := duplicateIDs(t, nil,
				dupTxn(1, "acct-1", day(2026, 8, 3), duplicateAmount, c.first),
				dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, c.other))

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_does_not_flag_transactions_that_differ_in_account_or_amount_or_are_zero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		firstAccount string
		otherAccount string
		firstAmount  int64
		otherAmount  int64
	}{
		{"in different accounts", "acct-1", "acct-2", duplicateAmount, duplicateAmount},
		{"a cent apart", "acct-1", "acct-1", duplicateAmount, duplicateAmount - 1},
		{"opposite signs", "acct-1", "acct-1", duplicateAmount, -duplicateAmount},
		{"zero amounts", "acct-1", "acct-1", 0, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := duplicateIDs(t, nil,
				dupTxn(1, c.firstAccount, day(2026, 8, 3), c.firstAmount, uncleared),
				dupTxn(2, c.otherAccount, day(2026, 8, 4), c.otherAmount, uncleared))

			assert.Empty(t, got)
		})
	}
}

func Test_replace_flags_duplicates_in_every_kind_of_account_and_ignores_the_payee(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*store.Rows)
		txns   []store.Transaction
	}{
		{"a closed account outside reports", nil, []store.Transaction{
			dupTxn(1, "acct-2", day(2026, 8, 3), duplicateAmount, uncleared), dupTxn(2, "acct-2", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
		{"a linked-tracking account", func(r *store.Rows) { r.Accounts[0].LinkedTracking = true }, []store.Transaction{
			dupTxn(1, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared), dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
		{"transactions excluded from reports", nil, []store.Transaction{
			{ID: "txn-1", SourceID: 1, AccountID: "acct-1", Date: day(2026, 8, 3), Amount: duplicateAmount, Currency: "CAD", Status: uncleared, ExcludedFromReports: true},
			dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
		{"transfer legs", func(r *store.Rows) {
			r.Splits = []store.Split{
				{ID: "split-1", SourceID: 1, TransactionID: "txn-1", Amount: duplicateAmount, TransferAccountID: new("acct-2")},
				{ID: "split-2", SourceID: 2, TransactionID: "txn-2", Amount: duplicateAmount, TransferAccountID: new("acct-2")},
			}
		}, []store.Transaction{
			dupTxn(1, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared), dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
		{"a payee on only one", nil, []store.Transaction{
			{ID: "txn-1", SourceID: 1, AccountID: "acct-1", Date: day(2026, 8, 3), Amount: duplicateAmount, Currency: "CAD", Status: uncleared, PayeeID: new("payee-1")},
			dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := duplicateIDs(t, c.mutate, c.txns...)

			assert.Equal(t, "duplicate:txn-1+txn-2", got)
		})
	}
}

func Test_replace_records_one_duplicate_finding_per_pair_of_three_matching_transactions(t *testing.T) {
	t.Parallel()

	got := duplicateIDs(t, nil,
		dupTxn(9, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared),
		dupTxn(10, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		dupTxn(11, "acct-1", day(2026, 8, 5), duplicateAmount, uncleared))

	assert.Equal(t, "duplicate:txn-10+txn-11,duplicate:txn-9+txn-10,duplicate:txn-9+txn-11", got)
}

func Test_replace_records_the_two_transactions_of_a_duplicate_as_its_items(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Transactions = []store.Transaction{
		dupTxn(9, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared),
		dupTxn(10, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
	}
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)
	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)

	assertScalar(t, db, `SELECT string_agg(COALESCE(transaction_id, 'NULL') || '|' || COALESCE(split_id, 'NULL') || '|' ||
		COALESCE(payee_id, 'NULL') || '|' || COALESCE(category_id, 'NULL'), '; ' ORDER BY transaction_id)
		FROM finding_items WHERE finding_id = 'duplicate:txn-9+txn-10'`,
		"txn-10|NULL|NULL|NULL; txn-9|NULL|NULL|NULL")
}
