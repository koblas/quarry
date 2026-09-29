package duckstore_test

import (
	"errors"
	"os"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
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
	st := replaceWith(t, []store.Account{chequing, usChequing, rrsp, visa, savings}, []store.Transaction{
		transaction("txn-1", "acct-1", past, 123456),
		transaction("txn-2", "acct-1", past, 500),
		transaction("txn-3", "acct-2", past, 800),
		transaction("txn-4", "acct-3", past, 100000),
		transaction("txn-5", "acct-4", past, -2500),
	})

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, store.AccountList{AsOf: localToday(), Accounts: []store.AccountBalance{
		{Account: chequing, Balance: new(int64(123956))},
		{Account: rrsp, Balance: nil},
		{Account: savings, Balance: new(int64(0))},
		{Account: usChequing, Balance: new(int64(800))},
		{Account: visa, Balance: new(int64(-2500))},
	}}, got)
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
	assert.Equal(t, []*int64{new(int64(100)), new(int64(0))},
		[]*int64{got.Accounts[0].Balance, got.Accounts[1].Balance})
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

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, store.AccountList{AsOf: localToday()}, got)
}

func Test_accounts_returns_the_open_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault("open store read-only")
	st := newBuiltStore(t, failingOpener(fault))

	_, err := st.Accounts(t.Context())

	require.ErrorIs(t, err, fault)
}

func Test_accounts_returns_the_query_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: fault}))

	_, err := st.Accounts(t.Context())

	require.ErrorIs(t, err, fault)
	var derr *duckdbdriver.Error
	assert.ErrorAs(t, err, &derr)
}

func Test_accounts_returns_the_scan_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{scanFault: errScanFailed}))

	_, err := st.Accounts(t.Context())

	require.ErrorIs(t, err, errScanFailed)
}

func Test_accounts_closes_the_connection_on_success_and_on_a_query_fault(t *testing.T) {
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

			_, _ = st.Accounts(t.Context())

			assert.Equal(t, 1, spy.closes)
		})
	}
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
