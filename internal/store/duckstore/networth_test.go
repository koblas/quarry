package duckstore_test

import (
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// netWorthOn reads st's net worth rows on dates.
func netWorthOn(t *testing.T, st *duckstore.Store, dates ...time.Time) []store.NetWorthRow {
	t.Helper()
	return netWorthRead(t, st, dates...).Rows
}

func Test_net_worth_reads_every_column_of_a_converted_row(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "Second", "chequing", "CAD")},
		transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctTwo, march(1), 5_000)),
		ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := netWorthOn(t, st, march(2))

	assert.Equal(t, []store.NetWorthRow{{
		Date: march(2), Type: "chequing", Currency: "CAD", Accounts: 2,
		Balance: big.NewInt(15_000), BalanceCAD: big.NewInt(15_000), BalanceUSD: big.NewInt(12_000),
	}}, got)
}

func Test_net_worth_leaves_nil_a_balance_the_store_holds_as_null(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, cashRows(transaction("t1", acctOne, march(1), 10_000)), ratesOn(10, 1_250_000, "FXUSDCAD"))

	got := netWorthOn(t, st, march(5))

	assert.Equal(t, []store.NetWorthRow{{
		Date: march(5), Type: "chequing", Currency: "CAD", Accounts: 1,
		Balance: big.NewInt(10_000), BalanceCAD: big.NewInt(10_000), BalanceUSD: nil,
	}}, got)
}

func Test_net_worth_orders_rows_by_type_then_currency_whatever_order_the_accounts_were_stored_in(t *testing.T) {
	t.Parallel()
	cad, usd := account(acctOne, 1, "Savings CAD", "savings", "CAD"), account(acctTwo, 2, "Savings USD", "savings", "USD")
	card := account("acct-3", 3, "Card", "credit_card", "CAD")
	cases := []struct {
		name     string
		accounts []store.Account
	}{
		{name: "CAD, USD, then the earlier type", accounts: []store.Account{cad, usd, card}},
		{name: "the earlier type, USD, then CAD", accounts: []store.Account{card, usd, cad}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := noTransactionRows()
			rows.Accounts = c.accounts
			rows.Transactions = []store.Transaction{
				transaction("t1", acctOne, march(1), 100), transaction("t2", acctTwo, march(1), 100),
				transaction("t3", "acct-3", march(1), 100),
			}
			st := newStoreWith(t, rows)

			got := netWorthOn(t, st, march(1))

			require.Len(t, got, 3)
			assert.Equal(t, [][2]string{{"credit_card", "CAD"}, {"savings", "CAD"}, {"savings", "USD"}},
				[][2]string{{got[0].Type, got[0].Currency}, {got[1].Type, got[1].Currency}, {got[2].Type, got[2].Currency}})
		})
	}
}

func Test_net_worth_reads_two_dates_in_one_call_in_date_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctOne, march(3), 500)))

	got := netWorthOn(t, st, march(3), march(1))

	require.Len(t, got, 2)
	assert.Equal(t, []time.Time{march(1), march(3)}, []time.Time{got[0].Date, got[1].Date})
	assert.Equal(t, []*big.Int{big.NewInt(10_000), big.NewInt(10_500)}, []*big.Int{got[0].Balance, got[1].Balance})
}

func Test_net_worth_has_no_rows_for_a_date_before_the_first_transaction(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, march(5), 10_000)))

	before := netWorthOn(t, st, march(4))
	onFirst := netWorthOn(t, st, march(5))

	assert.Empty(t, before)
	assert.Len(t, onFirst, 1)
}

func Test_net_worth_leaves_out_a_transaction_dated_after_the_day_asked(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctOne, march(3), 500)))

	got := netWorthOn(t, st, march(2))

	require.Len(t, got, 1)
	assert.Equal(t, big.NewInt(10_000), got[0].Balance)
}

func Test_net_worth_reads_no_rows_for_no_dates(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, march(1), 10_000)))

	assert.Len(t, netWorthOn(t, st, march(1)), 1)
	assert.Empty(t, netWorthOn(t, st))
}

func Test_net_worth_gives_the_date_of_the_first_exchange_rate(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, cashRows(transaction("t1", acctOne, march(1), 10_000)),
		ratesOn(12, 1_250_000, "FXUSDCAD"), ratesOn(10, 1_250_000, "FXUSDCAD"))

	got := netWorthRead(t, st, march(5))

	assert.Equal(t, march(10), got.FirstRate)
}

func Test_net_worth_gives_no_first_rate_date_for_a_store_without_rates(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, march(1), 10_000)))

	got := netWorthRead(t, st, march(5))

	assert.True(t, got.FirstRate.IsZero())
	assert.Len(t, got.Rows, 1)
}

func Test_net_worth_returns_a_first_rate_read_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, c := range otherFaults("SELECT min", 2) {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(c.spy))

			_, err := st.NetWorth(t.Context(), store.NetWorthParams{Dates: []time.Time{march(5)}})

			assertOtherFault(t, err, c.reason)
			assert.ErrorIs(t, err, c.fault)
		})
	}
}

func Test_net_worth_for_no_dates_still_refuses_a_missing_store(t *testing.T) {
	t.Parallel()

	_, err := duckstore.New(t.TempDir()).NetWorth(t.Context(), store.NetWorthParams{})

	openErr, ok := errors.AsType[*store.OpenError](err)
	require.True(t, ok, "want *store.OpenError, got %v", err)
	assert.Equal(t, store.OpenFaultMissing, openErr.Fault)
}

func Test_net_worth_reads_a_balance_past_64_bits_in_exact_cents(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), maxDecimal18x6))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), maxDecimal18x6)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := netWorthOn(t, st, march(2))

	require.Len(t, got, 1)
	assert.Equal(t, []*big.Int{cents("99999999999999999800000000"), cents("99999999999999999800000000"), cents("79999999999999999840000000")},
		[]*big.Int{got[0].Balance, got[0].BalanceCAD, got[0].BalanceUSD})
}

// firstBalanceOn reads st's first balance date.
func firstBalanceOn(t *testing.T, st *duckstore.Store) time.Time {
	t.Helper()
	return netWorthRead(t, st, march(10)).FirstBalance
}

func Test_net_worth_first_balance_is_the_earliest_transaction_of_any_counted_account(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, netWorthRows([]store.Account{account(acctTwo, 2, "Second", "chequing", "CAD")},
		transaction("t1", acctOne, march(5), 100), transaction("t2", acctTwo, march(3), 100), transaction("t3", acctOne, march(7), 100)))

	assert.Equal(t, march(3), firstBalanceOn(t, st))
}

func Test_net_worth_first_balance_is_zero_for_a_store_with_no_transactions_or_holdings(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, noTransactionRows())

	assert.True(t, firstBalanceOn(t, st).IsZero())
}

func Test_net_worth_first_balance_ignores_accounts_left_out_of_reports(t *testing.T) {
	t.Parallel()
	left := account(acctTwo, 2, "Left out", "chequing", "CAD")
	left.NotInReports = true
	linked := account("acct-3", 3, "Linked", "chequing", "CAD")
	linked.LinkedTracking = true
	st := newStoreWith(t, netWorthRows([]store.Account{left, linked},
		transaction("t1", acctTwo, march(1), 100), transaction("t2", "acct-3", march(2), 100), transaction("t3", acctOne, march(6), 100)))

	assert.Equal(t, march(6), firstBalanceOn(t, st))
}

func Test_net_worth_first_balance_counts_an_account_in_reports_that_is_not_linked(t *testing.T) {
	t.Parallel()
	counted := account(acctTwo, 2, "Counted", "chequing", "CAD")
	st := newStoreWith(t, netWorthRows([]store.Account{counted},
		transaction("t1", acctTwo, march(1), 100), transaction("t3", acctOne, march(6), 100)))

	assert.Equal(t, march(1), firstBalanceOn(t, st))
}

func Test_net_worth_first_balance_is_a_holding_that_starts_before_the_first_transaction(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(2), oneShare))
	rows.Transactions = []store.Transaction{transaction("t1", acctChequing, march(4), 100)}
	st := newStoreWith(t, rows)

	assert.Equal(t, march(2), firstBalanceOn(t, st))
}

func Test_net_worth_first_balance_is_a_transaction_that_starts_before_the_first_holding(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(4), oneShare))
	rows.Transactions = []store.Transaction{transaction("t1", acctChequing, march(2), 100)}
	st := newStoreWith(t, rows)

	assert.Equal(t, march(2), firstBalanceOn(t, st))
}

func Test_net_worth_first_balance_ignores_a_holding_of_an_account_left_out_of_reports(t *testing.T) {
	t.Parallel()
	rows := holdingRows(buy(acctOne, secAcme, 1, march(2), oneShare))
	rows.Accounts[0].NotInReports = true
	st := newStoreWith(t, rows)

	assert.True(t, firstBalanceOn(t, st).IsZero())
}

func Test_net_worth_returns_a_first_balance_read_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, c := range otherFaults("SELECT min", 3) {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(c.spy))

			_, err := st.NetWorth(t.Context(), store.NetWorthParams{Dates: []time.Time{march(5)}})

			assertOtherFault(t, err, c.reason)
			assert.ErrorIs(t, err, c.fault)
		})
	}
}

// netWorthRows is cashRows with accounts joining the base CAD chequing account acct-1.
func netWorthRows(accounts []store.Account, txns ...store.Transaction) store.Rows {
	rows := cashRows(txns...)
	rows.Accounts = append(rows.Accounts, accounts...)
	return rows
}

// netWorthQuery is the type, currency, accounts and balance of every v_net_worth row on the March day.
func netWorthQuery(day int) string {
	return fmt.Sprintf("SELECT type, currency, accounts, balance FROM v_net_worth WHERE date = '2026-03-%02d' ORDER BY type, currency", day)
}

// netWorthConvertedQuery is the currency, balance_cad and balance_usd of every v_net_worth row on the March day.
func netWorthConvertedQuery(day int) string {
	return fmt.Sprintf("SELECT currency, balance_cad, balance_usd FROM v_net_worth WHERE date = '2026-03-%02d' ORDER BY currency", day)
}

func Test_net_worth_groups_accounts_by_type_and_currency(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		second store.Account
		cents  int64
		want   [][]string
	}{
		{
			name:   "sums_accounts_of_one_type_and_currency_into_one_row",
			second: account(acctTwo, 2, "Second", "chequing", "CAD"), cents: 5_000,
			want: [][]string{{"chequing", "CAD", "2", "150.00"}},
		},
		{
			name:   "keeps_a_type_in_another_currency_in_its_own_row",
			second: account(acctTwo, 2, "US", "chequing", "USD"), cents: 5_000,
			want: [][]string{{"chequing", "CAD", "1", "100.00"}, {"chequing", "USD", "1", "50.00"}},
		},
		{
			name:   "keeps_a_currency_in_another_type_in_its_own_row",
			second: account(acctTwo, 2, "Savings", "savings", "CAD"), cents: 5_000,
			want: [][]string{{"chequing", "CAD", "1", "100.00"}, {"savings", "CAD", "1", "50.00"}},
		},
		{
			name:   "keeps_the_sign_of_a_negative_balance",
			second: account(acctTwo, 2, "Card", "credit", "CAD"), cents: -30_000,
			want: [][]string{{"chequing", "CAD", "1", "100.00"}, {"credit", "CAD", "1", "-300.00"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, netWorthRows([]store.Account{c.second},
				transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctTwo, march(1), c.cents)))

			got := queryTexts(t, st, netWorthQuery(1))

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_net_worth_counts_a_closed_account_on_the_days_after_it_closed(t *testing.T) {
	t.Parallel()
	rows := netWorthRows(nil, transaction("t1", acctOne, march(1), 10_000))
	rows.Accounts[0].Closed = true
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), accounts, balance FROM v_net_worth WHERE date IN ('2026-03-01', '2026-03-20') ORDER BY date")

	assert.Equal(t, [][]string{{"2026-03-01", "1", "100.00"}, {"2026-03-20", "1", "100.00"}}, got)
}

func Test_net_worth_leaves_out_accounts_not_in_reports_and_linked_tracking(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		flag func(*store.Account)
	}{
		{name: "an account not in reports", flag: func(a *store.Account) { a.NotInReports = true }},
		{name: "a linked-tracking account", flag: func(a *store.Account) { a.LinkedTracking = true }},
		{name: "an account both not in reports and linked-tracking", flag: func(a *store.Account) { a.NotInReports, a.LinkedTracking = true, true }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			left := account(acctTwo, 2, "Left Out", "chequing", "CAD")
			c.flag(&left)
			st := newStoreWith(t, netWorthRows([]store.Account{left},
				transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctTwo, march(1), 5_000)))

			got := queryTexts(t, st, netWorthQuery(1))

			assert.Equal(t, [][]string{{"chequing", "CAD", "1", "100.00"}}, got)
		})
	}
}

func Test_net_worth_balance_of_an_investment_account_adds_its_valued_holdings_to_cash(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), 3*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), tenUnits)}
	rows.Transactions = []store.Transaction{
		transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctChequing, march(1), 5_000),
	}
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, netWorthQuery(2))

	assert.Equal(t, [][]string{{"brokerage", "CAD", "1", "130.00"}, {"chequing", "CAD", "1", "50.00"}}, got)
}

func Test_net_worth_converts_an_investment_accounts_cash_plus_valued_holdings(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), 3*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), tenUnits)}
	rows.Transactions = []store.Transaction{
		transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctChequing, march(1), 5_000),
	}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := queryTexts(t, st, "SELECT type, balance_cad, balance_usd FROM v_net_worth WHERE date = '2026-03-02' ORDER BY type")

	assert.Equal(t, [][]string{{"brokerage", "130.00", "104.00"}, {"chequing", "50.00", "40.00"}}, got)
}

func Test_net_worth_has_no_row_for_a_date_where_every_account_is_left_out(t *testing.T) {
	t.Parallel()
	rows := netWorthRows(nil, transaction("t1", acctOne, march(1), 10_000))
	rows.Accounts[0].NotInReports = true
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT count(*) FROM v_net_worth")

	assert.Equal(t, [][]string{{"0"}}, got)
}

func Test_net_worth_adds_an_account_on_the_day_of_its_first_transaction_not_before(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "Later", "chequing", "CAD")},
		transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctTwo, march(5), 5_000)))

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), accounts, balance FROM v_net_worth WHERE date IN ('2026-03-04', '2026-03-05') ORDER BY date")

	assert.Equal(t, [][]string{{"2026-03-04", "1", "100.00"}, {"2026-03-05", "2", "150.00"}}, got)
}

func Test_net_worth_converts_the_sum_of_each_accounts_rounded_conversion(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "Second", "chequing", "CAD")},
		transaction("t1", acctOne, march(1), 2), transaction("t2", acctTwo, march(1), 2)),
		ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := queryTexts(t, st, netWorthConvertedQuery(1))

	assert.Equal(t, [][]string{{"CAD", "0.04", "0.04"}}, got)
}

func Test_net_worth_has_no_conversion_to_the_other_currency_before_the_first_rate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		currency string
		want     []string
	}{
		{name: "a CAD row keeps balance_cad and has no balance_usd", currency: "CAD", want: []string{"CAD", "100.00", "NULL"}},
		{name: "a USD row keeps balance_usd and has no balance_cad", currency: "USD", want: []string{"USD", "NULL", "100.00"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := cashRows()
			rows.Accounts = []store.Account{account(acctOne, 1, "Only", "chequing", c.currency)}
			rows.Transactions = []store.Transaction{transaction("t1", acctOne, march(1), 10_000)}
			st := newStoreWithRates(t, rows, ratesOn(10, 1_250_000, "FXUSDCAD"))

			got := queryTexts(t, st, netWorthConvertedQuery(5))

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}

func Test_net_worth_has_no_converted_balance_for_a_currency_other_than_cad_and_usd(t *testing.T) {
	t.Parallel()
	rows := cashRows()
	rows.Accounts = []store.Account{account(acctOne, 1, "Euro", "chequing", "EUR")}
	rows.Transactions = []store.Transaction{transaction("t1", acctOne, march(1), 10_000)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := queryTexts(t, st, "SELECT currency, balance, balance_cad, balance_usd FROM v_net_worth WHERE date = '2026-03-05'")

	assert.Equal(t, [][]string{{"EUR", "100.00", "NULL", "NULL"}}, got)
}

func Test_net_worth_converts_a_day_in_a_rate_gap_at_the_prior_rate(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "US", "chequing", "USD")},
		transaction("t1", acctTwo, march(1), 10_000)),
		ratesOn(10, 1_250_000, "FXUSDCAD"), ratesOn(16, 1_300_000, "FXUSDCAD"))

	got := queryTexts(t, st, netWorthConvertedQuery(15))

	assert.Equal(t, [][]string{{"USD", "125.00", "100.00"}}, got)
}

func Test_net_worth_converted_columns_sum_over_one_date_to_the_total(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "US", "chequing", "USD")},
		transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctTwo, march(1), 10_000)),
		ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := queryTexts(t, st, "SELECT sum(balance_cad), sum(balance_usd) FROM v_net_worth WHERE date = '2026-03-05'")

	assert.Equal(t, [][]string{{"225.00", "180.00"}}, got)
}

func Test_net_worth_view_lists_its_columns_in_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows())

	got, err := st.Query(t.Context(), "SELECT * FROM v_net_worth", 0) //nolint:unqueryvet // every column is the point

	require.NoError(t, err)
	assert.Equal(t, []store.QueryColumn{
		{Name: "date", Type: "DATE"},
		{Name: "type", Type: "VARCHAR"},
		{Name: "currency", Type: "VARCHAR"},
		{Name: "accounts", Type: "BIGINT"},
		{Name: "balance", Type: "DECIMAL(38,2)"},
		{Name: "balance_cad", Type: "DECIMAL(38,2)"},
		{Name: "balance_usd", Type: "DECIMAL(38,2)"},
	}, got.Columns)
}

func Test_net_worth_view_carries_its_note(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows())

	got := queryTexts(t, st, "SELECT comment FROM duckdb_views() WHERE view_name = 'v_net_worth'")

	assert.Equal(t, [][]string{{"net worth by day, account type and currency over the accounts Quicken's reports count, as quarry networth does; " +
		"sum balance_cad or balance_usd over one date for the total; a NULL there means no exchange rate for that day."}}, got)
}
