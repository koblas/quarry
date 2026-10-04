package duckstore_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errScanFailed has the shape of database/sql's Scan conversion error.
var errScanFailed = errors.New(`sql: Scan error on column index 1, name "id": converting NULL to string is unsupported`)

// localToday is today's local calendar date at UTC midnight, the shape a DATE column reads back as.
func localToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func account(id string, sourceID int64, name, accountType, currency string) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: name, Type: accountType, Currency: currency, Active: true}
}

func transaction(id, accountID string, date time.Time, cents int64) store.Transaction {
	return store.Transaction{ID: id, SourceID: 1, AccountID: accountID, Date: date, Amount: cents, Currency: "CAD", Status: "uncleared"}
}

// replaceWith builds a store in a fresh directory from accounts and transactions only.
func replaceWith(t *testing.T, accounts []store.Account, txns []store.Transaction) *duckstore.Store {
	t.Helper()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), store.Rows{Accounts: accounts, Transactions: txns, ImportRuns: minimalRows().ImportRuns})
	require.NoError(t, err)
	return st
}

// ownBalances is accounts without their converted cells, which the conversion tests pin.
func ownBalances(accounts []store.AccountBalance) []store.AccountBalance {
	out := make([]store.AccountBalance, len(accounts))
	for i, a := range accounts {
		a.BalanceCAD, a.BalanceUSD = nil, nil
		out[i] = a
	}
	return out
}

func Test_accounts_reads_each_accounts_balance(t *testing.T) {
	t.Parallel()
	past := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	chequing := account("acct-1", 1, "Chequing", "chequing", "CAD")
	chequing.Institution = new("Big Bank")
	usChequing := account("acct-2", 2, "US Chequing", "chequing", "USD")
	rrsp := account("acct-3", 3, "RRSP", "retirement", "CAD")
	visa := account("acct-4", 4, "Visa", "credit_card", "CAD")
	visa.Closed, visa.Active = true, false
	savings := account("acct-5", 5, "Savings", "savings", "CAD")
	savings.NotInReports = true
	st := replaceWith(t, []store.Account{chequing, usChequing, rrsp, visa, savings}, []store.Transaction{
		transaction("txn-1", "acct-1", past, 123456),
		transaction("txn-2", "acct-1", past, 500),
		transaction("txn-3", "acct-2", past, 800),
		transaction("txn-4", "acct-3", past, 100000),
		transaction("txn-5", "acct-4", past, -2500),
	})

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []store.AccountBalance{
		{Account: chequing, Balance: 123956, Cash: 123956},
		{Account: rrsp, Balance: 100000, Cash: 100000, HoldingsValue: new(int64(0))},
		{Account: savings},
		{Account: usChequing, Balance: 800, Cash: 800},
		{Account: visa, Balance: -2500, Cash: -2500},
	}, ownBalances(got.Accounts))
}

func Test_accounts_reads_which_accounts_use_linked_account_tracking(t *testing.T) {
	t.Parallel()
	chequing := account("acct-1", 1, "Chequing", "chequing", "CAD")
	linked := account("acct-2", 2, "Linked", "retirement", "USD")
	linked.LinkedTracking = true
	both := account("acct-3", 3, "Both", "savings", "CAD")
	both.NotInReports, both.LinkedTracking = true, true
	st := replaceWith(t, []store.Account{chequing, linked, both}, nil)

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []store.AccountBalance{
		{Account: both},
		{Account: chequing},
		{Account: linked, HoldingsValue: new(int64(0))},
	}, ownBalances(got.Accounts))
}

func Test_accounts_counts_transactions_dated_today_but_not_tomorrow(t *testing.T) {
	t.Parallel()
	today := localToday()
	tomorrow := today.AddDate(0, 0, 1)
	st := replaceWith(t, []store.Account{
		account("acct-1", 1, "Chequing", "chequing", "CAD"),
		account("acct-2", 2, "Savings", "savings", "CAD"),
	}, []store.Transaction{
		transaction("txn-1", "acct-1", today, 100),
		transaction("txn-2", "acct-1", tomorrow, 1000),
		transaction("txn-3", "acct-2", tomorrow, 700),
	})

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	require.Len(t, got.Accounts, 2)
	// Past midnight, as_of is tomorrow and counts the tomorrow rows too.
	want := map[time.Time][]int64{today: {100, 0}, tomorrow: {1100, 700}}
	balances := []int64{got.Accounts[0].Balance, got.Accounts[1].Balance}
	assert.Equal(t, want[got.AsOf], balances)
}

func Test_accounts_sorts_by_name_then_source_id(t *testing.T) {
	t.Parallel()
	st := replaceWith(t, []store.Account{
		account("zed", 1, "Zed", "chequing", "CAD"),
		account("visa-lower", 4, "visa", "credit_card", "CAD"),
		account("visa-upper", 6, "Visa", "credit_card", "CAD"),
		account("chequing-a", 9, "Chequing", "chequing", "CAD"),
		account("chequing-b", 7, "Chequing", "chequing", "CAD"),
		account("chequing-c", 3, "Chequing", "chequing", "CAD"),
	}, nil)

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	ids := make([]string, 0, len(got.Accounts))
	for _, a := range got.Accounts {
		ids = append(ids, a.ID)
	}
	assert.Equal(t, []string{"chequing-c", "chequing-b", "chequing-a", "visa-upper", "visa-lower", "zed"}, ids)
}

func Test_accounts_reads_as_of_with_no_accounts(t *testing.T) {
	t.Parallel()
	st := replaceWith(t, nil, nil)
	before := localToday()

	got, err := st.Accounts(t.Context())

	after := localToday()
	require.NoError(t, err)
	assert.Empty(t, got.Accounts)
	assert.Contains(t, []time.Time{before, after}, got.AsOf)
}

func Test_accounts_fails_on_a_missing_store_without_creating_it(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := duckstore.New(dir).Accounts(t.Context())

	require.Error(t, err)
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}
