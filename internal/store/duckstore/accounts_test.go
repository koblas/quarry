package duckstore_test

import (
	"math/big"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		{Account: chequing, Balance: big.NewInt(123956), Cash: big.NewInt(123956)},
		{Account: rrsp, Balance: big.NewInt(100000), Cash: big.NewInt(100000), HoldingsValue: big.NewInt(0)},
		{Account: savings, Balance: big.NewInt(0), Cash: big.NewInt(0)},
		{Account: usChequing, Balance: big.NewInt(800), Cash: big.NewInt(800)},
		{Account: visa, Balance: big.NewInt(-2500), Cash: big.NewInt(-2500)},
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
		{Account: both, Balance: big.NewInt(0), Cash: big.NewInt(0)},
		{Account: chequing, Balance: big.NewInt(0), Cash: big.NewInt(0)},
		{Account: linked, Balance: big.NewInt(0), Cash: big.NewInt(0), HoldingsValue: big.NewInt(0)},
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
	want := map[time.Time][]*big.Int{today: {big.NewInt(100), big.NewInt(0)}, tomorrow: {big.NewInt(1100), big.NewInt(700)}}
	balances := []*big.Int{got.Accounts[0].Balance, got.Accounts[1].Balance}
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

const (
	cadChequing   = "acct-cad"
	usdChequing   = "acct-usd"
	usdBrokerage  = "acct-brk"
	zoneChildEnv  = "QUARRY_ACCOUNTS_ZONE_CHILD"
	zoneChildTest = "Test_accounts_zone_probe"
)

// accountRates is the rows behind the account conversion tests: a CAD chequing account holding 100.00, a USD one
// holding 8.00 and a USD brokerage account, all with a balance dated in the past.
func accountRates(t *testing.T, rates ...store.Rate) store.AccountList {
	t.Helper()
	past := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	rows := store.Rows{
		Accounts: []store.Account{
			account(cadChequing, 1, "CAD Chequing", "chequing", "CAD"),
			account(usdChequing, 2, "USD Chequing", "chequing", "USD"),
			account(usdBrokerage, 3, "USD Brokerage", store.AccountTypeBrokerage, "USD"),
		},
		Transactions: []store.Transaction{
			transaction("txn-1", cadChequing, past, 10000),
			{ID: "txn-2", SourceID: 1, AccountID: usdChequing, Date: past, Amount: 800, Currency: "USD", Status: "uncleared"},
		},
		ImportRuns: minimalRows().ImportRuns,
	}
	got, err := newStoreWithRates(t, rows, rates...).Accounts(t.Context())
	require.NoError(t, err)
	return got
}

func cells(list store.AccountList) [][]*big.Int {
	out := make([][]*big.Int, len(list.Accounts))
	for i, a := range list.Accounts {
		out[i] = []*big.Int{a.Balance, a.BalanceCAD, a.BalanceUSD}
	}
	return out
}

func Test_accounts_converts_at_the_latest_rate_dated_today_or_earlier_ignoring_a_later_one(t *testing.T) {
	t.Parallel()
	past := store.Rate{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: 1_250_000, Series: store.SeriesCurrent}
	later := store.Rate{Date: time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: 1_600_000, Series: store.SeriesCurrent}

	got := accountRates(t, past, later)

	assert.Equal(t, [][]*big.Int{
		{big.NewInt(10000), big.NewInt(10000), big.NewInt(8000)},
		{big.NewInt(0), big.NewInt(0), big.NewInt(0)},
		{big.NewInt(800), big.NewInt(1000), big.NewInt(800)},
	}, cells(got))
}

func Test_accounts_gives_a_cad_account_its_own_cad_cell_and_no_other_cell_without_rates(t *testing.T) {
	t.Parallel()

	got := accountRates(t)

	assert.Equal(t, [][]*big.Int{
		{big.NewInt(10000), big.NewInt(10000), nil},
		{big.NewInt(0), nil, big.NewInt(0)},
		{big.NewInt(800), nil, big.NewInt(800)},
	}, cells(got))
}

func Test_accounts_gives_the_date_of_the_earliest_rate(t *testing.T) {
	t.Parallel()
	first := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	later := store.Rate{Date: time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: 1_600_000, Series: store.SeriesCurrent}

	got := accountRates(t, later, store.Rate{Date: first, USDCAD: 1_250_000, Series: store.SeriesCurrent})

	assert.Equal(t, first, got.FirstRate)
}

func Test_accounts_gives_no_first_rate_date_for_a_store_without_rates(t *testing.T) {
	t.Parallel()

	got := accountRates(t)

	assert.True(t, got.FirstRate.IsZero())
}

func Test_accounts_gives_the_first_rate_date_of_a_store_with_rates_after_today_and_no_cross_cell(t *testing.T) {
	t.Parallel()
	later := time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)

	got := accountRates(t, store.Rate{Date: later, USDCAD: 1_600_000, Series: store.SeriesCurrent})

	assert.Equal(t, later, got.FirstRate)
	assert.Nil(t, got.Accounts[2].BalanceCAD)
}

func Test_accounts_gives_the_first_rate_date_of_a_store_with_no_accounts(t *testing.T) {
	t.Parallel()
	first := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	rows := store.Rows{ImportRuns: minimalRows().ImportRuns}

	got, err := newStoreWithRates(t, rows, store.Rate{Date: first, USDCAD: 1_250_000, Series: store.SeriesCurrent}).Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, first, got.FirstRate)
}

func Test_accounts_converts_a_cad_balance_to_usd_at_the_latest_rate(t *testing.T) {
	t.Parallel()
	rate := store.Rate{Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), USDCAD: money.Rate(1_600_000), Series: store.SeriesCurrent}

	got := accountRates(t, rate)

	assert.Equal(t, big.NewInt(6250), got.Accounts[0].BalanceUSD)
}

func Test_accounts_use_the_local_date_in_every_zone(t *testing.T) {
	zones := []string{"Pacific/Kiritimati", "Pacific/Pago_Pago"}

	for _, zone := range zones {
		t.Run(zone, func(t *testing.T) {
			probe := exec.CommandContext( //nolint:gosec // re-executes this test binary
				t.Context(), os.Args[0], "-test.run=^"+zoneChildTest+"$", "-test.v", "-test.count=1")
			probe.Env = append(os.Environ(), "TZ="+zone, zoneChildEnv+"=1")

			out, err := probe.CombinedOutput()

			require.NoError(t, err, string(out))
			assert.Contains(t, string(out), "--- PASS: "+zoneChildTest)
		})
	}
}

// Test_accounts_zone_probe is the child of Test_accounts_use_the_local_date_in_every_zone; alone it skips.
func Test_accounts_zone_probe(t *testing.T) {
	if os.Getenv(zoneChildEnv) == "" {
		t.Skip("runs only inside Test_accounts_use_the_local_date_in_every_zone")
	}
	today := localToday()
	tomorrow := today.AddDate(0, 0, 1)

	got := accountRates(t,
		store.Rate{Date: today, USDCAD: 1_300_000, Series: store.SeriesCurrent},
		store.Rate{Date: tomorrow, USDCAD: 1_400_000, Series: store.SeriesCurrent})

	assert.Equal(t, today, got.AsOf)
	assert.Equal(t, big.NewInt(1040), got.Accounts[2].BalanceCAD)
}

// balanceOf is the account with id in list.
func balanceOf(t *testing.T, list store.AccountList, id string) store.AccountBalance {
	t.Helper()
	for _, a := range list.Accounts {
		if a.ID == id {
			return a
		}
	}
	require.Failf(t, "account not listed", "id %s", id)
	return store.AccountBalance{}
}

func Test_accounts_reads_an_investment_accounts_cash_and_valued_holdings(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), 3*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), tenUnits)}
	rows.Transactions = []store.Transaction{transaction("t1", acctOne, march(1), 10_000)}
	st := newStoreWith(t, rows)

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	a := balanceOf(t, got, acctOne)
	require.NotNil(t, a.HoldingsValue)
	assert.Equal(t, []*big.Int{big.NewInt(10_000), big.NewInt(3_000), big.NewInt(13_000)}, []*big.Int{a.Cash, a.HoldingsValue, a.Balance})
}

func Test_accounts_reads_a_negative_investment_cash_balance(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), 3*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), tenUnits)}
	rows.Transactions = []store.Transaction{transaction("t1", acctOne, march(1), -10_000)}
	st := newStoreWith(t, rows)

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	a := balanceOf(t, got, acctOne)
	require.NotNil(t, a.HoldingsValue)
	assert.Equal(t, []*big.Int{big.NewInt(-10_000), big.NewInt(3_000), big.NewInt(-7_000)}, []*big.Int{a.Cash, a.HoldingsValue, a.Balance})
}

func Test_accounts_leaves_an_unpriced_holding_out_of_an_investment_balance(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secControl, 2, march(1), oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), tenUnits)}
	st := newStoreWith(t, rows)

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	a := balanceOf(t, got, acctOne)
	require.NotNil(t, a.HoldingsValue)
	assert.Equal(t, []*big.Int{big.NewInt(0), big.NewInt(1_000), big.NewInt(1_000)}, []*big.Int{a.Cash, a.HoldingsValue, a.Balance})
}

func Test_accounts_converts_a_usd_holding_into_a_cad_investment_balance(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secUSD, 1, march(1), oneShare))
	rows.Prices = []store.Price{quote(secUSD, 1, march(1), tenUnits)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, big.NewInt(1_250), balanceOf(t, got, acctOne).Balance)
}

func Test_accounts_gives_a_non_investment_account_no_holdings_value(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows())

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Nil(t, balanceOf(t, got, acctChequing).HoldingsValue)
}

func Test_account_balances_lists_an_account_with_no_balance_row_as_zero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		rows    store.Rows
		account string
		want    []string
	}{
		{
			name:    "an account whose only transaction is future-dated",
			rows:    cashRows(transaction("t1", acctOne, localToday().AddDate(0, 0, 3), 100)),
			account: acctOne, want: []string{"0.00", "NULL", "0.00"},
		},
		{
			name: "an account with no transaction", rows: cashRows(),
			account: acctOne, want: []string{"0.00", "NULL", "0.00"},
		},
		{
			name: "an investment account with no transaction and no holding", rows: balanceRows(),
			account: acctTwo, want: []string{"0.00", "0.00", "0.00"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, c.rows)

			got := queryTexts(t, st, "SELECT cash, holdings_value, balance FROM v_account_balances WHERE id = '"+c.account+"'")

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}

func Test_accounts_reads_a_balance_past_64_bits_in_exact_cents(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), maxDecimal18x6))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), maxDecimal18x6)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	a := balanceOf(t, got, acctOne)
	assert.Equal(t, []*big.Int{
		big.NewInt(0), cents("99999999999999999800000000"), cents("99999999999999999800000000"),
		cents("99999999999999999800000000"), cents("79999999999999999840000000"),
	},
		[]*big.Int{a.Cash, a.HoldingsValue, a.Balance, a.BalanceCAD, a.BalanceUSD})
}
