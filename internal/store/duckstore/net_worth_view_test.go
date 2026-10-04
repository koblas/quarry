package duckstore_test

import (
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func Test_net_worth_sums_accounts_of_one_type_and_currency_into_one_row(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "Second", "chequing", "CAD")},
		transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctTwo, marchDay(1), 5_000)))

	got := queryTexts(t, st, netWorthQuery(1))

	assert.Equal(t, [][]string{{"chequing", "CAD", "2", "150.00"}}, got)
}

func Test_net_worth_keeps_a_type_in_another_currency_in_its_own_row(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "US", "chequing", "USD")},
		transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctTwo, marchDay(1), 5_000)))

	got := queryTexts(t, st, netWorthQuery(1))

	assert.Equal(t, [][]string{{"chequing", "CAD", "1", "100.00"}, {"chequing", "USD", "1", "50.00"}}, got)
}

func Test_net_worth_keeps_a_currency_in_another_type_in_its_own_row(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "Savings", "savings", "CAD")},
		transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctTwo, marchDay(1), 5_000)))

	got := queryTexts(t, st, netWorthQuery(1))

	assert.Equal(t, [][]string{{"chequing", "CAD", "1", "100.00"}, {"savings", "CAD", "1", "50.00"}}, got)
}

func Test_net_worth_counts_a_closed_account_on_the_days_after_it_closed(t *testing.T) {
	t.Parallel()
	rows := netWorthRows(nil, transaction("t1", acctOne, marchDay(1), 10_000))
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			left := account(acctTwo, 2, "Left Out", "chequing", "CAD")
			c.flag(&left)
			st := newStoreWith(t, netWorthRows([]store.Account{left},
				transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctTwo, marchDay(1), 5_000)))

			got := queryTexts(t, st, netWorthQuery(1))

			assert.Equal(t, [][]string{{"chequing", "CAD", "1", "100.00"}}, got)
		})
	}
}

func Test_net_worth_has_no_row_for_a_date_where_every_account_is_left_out(t *testing.T) {
	t.Parallel()
	rows := netWorthRows(nil, transaction("t1", acctOne, marchDay(1), 10_000))
	rows.Accounts[0].NotInReports = true
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT count(*) FROM v_net_worth")

	assert.Equal(t, [][]string{{"0"}}, got)
}

func Test_net_worth_adds_an_account_on_the_day_of_its_first_transaction_not_before(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "Later", "chequing", "CAD")},
		transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctTwo, marchDay(5), 5_000)))

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), accounts, balance FROM v_net_worth WHERE date IN ('2026-03-04', '2026-03-05') ORDER BY date")

	assert.Equal(t, [][]string{{"2026-03-04", "1", "100.00"}, {"2026-03-05", "2", "150.00"}}, got)
}

func Test_net_worth_keeps_the_sign_of_a_negative_balance(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "Card", "credit", "CAD")},
		transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctTwo, marchDay(1), -30_000)))

	got := queryTexts(t, st, netWorthQuery(1))

	assert.Equal(t, [][]string{{"chequing", "CAD", "1", "100.00"}, {"credit", "CAD", "1", "-300.00"}}, got)
}

func Test_net_worth_converts_the_sum_of_each_accounts_rounded_conversion(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "Second", "chequing", "CAD")},
		transaction("t1", acctOne, marchDay(1), 2), transaction("t2", acctTwo, marchDay(1), 2)),
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
			rows.Transactions = []store.Transaction{transaction("t1", acctOne, marchDay(1), 10_000)}
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
	rows.Transactions = []store.Transaction{transaction("t1", acctOne, marchDay(1), 10_000)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := queryTexts(t, st, "SELECT currency, balance, balance_cad, balance_usd FROM v_net_worth WHERE date = '2026-03-05'")

	assert.Equal(t, [][]string{{"EUR", "100.00", "NULL", "NULL"}}, got)
}

func Test_net_worth_converts_a_day_in_a_rate_gap_at_the_prior_rate(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "US", "chequing", "USD")},
		transaction("t1", acctTwo, marchDay(1), 10_000)),
		ratesOn(10, 1_250_000, "FXUSDCAD"), ratesOn(16, 1_300_000, "FXUSDCAD"))

	got := queryTexts(t, st, netWorthConvertedQuery(15))

	assert.Equal(t, [][]string{{"USD", "125.00", "100.00"}}, got)
}

func Test_net_worth_converted_columns_sum_over_one_date_to_the_total(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "US", "chequing", "USD")},
		transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctTwo, marchDay(1), 10_000)),
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
