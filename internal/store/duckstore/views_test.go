package duckstore_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_cash_flow_leaves_out_what_quicken_reports_leave_out(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		add   splitSpec
		xfers []store.Transfer
	}{
		{
			name:  "the from leg of a paired transfer",
			add:   splitSpec{id: "left-out", category: new(catExpense), amount: -500},
			xfers: []store.Transfer{{ID: "xfer-1", FromSplitID: "left-out", ToSplitID: new("peer-leg")}},
		},
		{
			name:  "the to leg of a paired transfer",
			add:   splitSpec{id: "left-out", category: new(catExpense), amount: 500},
			xfers: []store.Transfer{{ID: "xfer-1", FromSplitID: "peer-leg", ToSplitID: new("left-out")}},
		},
		{
			name:  "an unmatched transfer leg with no transfer account",
			add:   splitSpec{id: "left-out", category: new(catExpense), amount: -500},
			xfers: []store.Transfer{{ID: "xfer-1", FromSplitID: "left-out"}},
		},
		{name: "a split in a system category", add: splitSpec{id: "left-out", category: new(catSystem), amount: -500}},
		{
			name: "a split on a transaction excluded from reports",
			add:  splitSpec{id: "left-out", category: new(catExpense), amount: -500, excluded: true},
		},
		{
			name: "a split in an account that is not in reports",
			add:  splitSpec{id: "left-out", account: acctNotReports, category: new(catExpense), amount: -500},
		},
		{
			name: "an expense in an account that uses linked account tracking",
			add:  splitSpec{id: "left-out", account: acctLinked, category: new(catExpense), amount: -500},
		},
		{
			name: "an uncategorized deposit in an account that uses linked account tracking",
			add:  splitSpec{id: "left-out", account: acctLinked, amount: 500},
		},
		{name: "a zero-amount uncategorized split", add: splitSpec{id: "left-out", amount: 0}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := reportRows()
			addSplit(&rows, c.add)
			rows.Transfers = append(rows.Transfers, c.xfers...)
			st := newStoreWith(t, rows)

			got := queryTexts(t, st, "SELECT split_id FROM v_cash_flow ORDER BY split_id")

			assert.Equal(t, [][]string{{keepSplit}}, got)
		})
	}
}

func Test_cash_flow_keeps_closed_accounts_hidden_categories_and_investment_accounts(t *testing.T) {
	t.Parallel()
	rows := reportRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: "acct-closed", SourceID: 3, Name: "Closed", Type: "savings", Currency: "CAD", Closed: true},
		store.Account{ID: "acct-brokerage", SourceID: 4, Name: "Broker", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	)
	rows.Categories = append(rows.Categories,
		store.Category{ID: "cat-hidden", SourceID: 4, Name: "Old", FullPath: "Old", Kind: "expense", Hidden: true})
	addSplit(&rows, splitSpec{id: "in-closed", account: "acct-closed", category: new(catExpense), amount: -100})
	addSplit(&rows, splitSpec{id: "in-brokerage", account: "acct-brokerage", category: new(catExpense), amount: -200})
	addSplit(&rows, splitSpec{id: "in-hidden", category: new("cat-hidden"), amount: -300})
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT split_id FROM v_cash_flow ORDER BY split_id")

	assert.Equal(t, [][]string{{"in-brokerage"}, {"in-closed"}, {"in-hidden"}, {keepSplit}}, got)
}

func Test_cash_flow_takes_flow_from_the_splits_own_category_kind(t *testing.T) {
	t.Parallel()
	rows := reportRows()
	rows.Categories = append(rows.Categories,
		store.Category{ID: "cat-income-child", SourceID: 4, ParentID: new(catIncome), Name: "Refunds", FullPath: "Salary:Refunds", Kind: "expense"},
		store.Category{ID: "cat-expense-child", SourceID: 5, ParentID: new(catExpense), Name: "Rebates", FullPath: "Groceries:Rebates", Kind: "income"},
	)
	addSplit(&rows, splitSpec{id: "expense-under-income", category: new("cat-income-child"), amount: 100})
	addSplit(&rows, splitSpec{id: "income-under-expense", category: new("cat-expense-child"), amount: -200})
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT split_id, flow FROM v_cash_flow WHERE split_id <> 'keep' ORDER BY split_id")

	assert.Equal(t, [][]string{{"expense-under-income", "expense"}, {"income-under-expense", "income"}}, got)
}

func Test_cash_flow_keeps_a_zero_amount_split_that_has_a_category(t *testing.T) {
	t.Parallel()
	rows := reportRows()
	addSplit(&rows, splitSpec{id: "zero-categorized", category: new(catExpense), amount: 0})
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT split_id, flow FROM v_cash_flow WHERE split_id <> 'keep'")

	assert.Equal(t, [][]string{{"zero-categorized", "expense"}}, got)
}

func Test_cash_flow_takes_an_uncategorized_splits_flow_from_its_sign(t *testing.T) {
	t.Parallel()
	rows := reportRows()
	addSplit(&rows, splitSpec{id: "uncategorized-out", amount: -100})
	addSplit(&rows, splitSpec{id: "uncategorized-in", amount: 200})
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT split_id, flow FROM v_cash_flow WHERE split_id <> 'keep' ORDER BY split_id")

	assert.Equal(t, [][]string{{"uncategorized-in", "income"}, {"uncategorized-out", "expense"}}, got)
}

func Test_cash_flow_lists_its_columns_in_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, reportRows())

	got, err := st.Query(t.Context(), "SELECT * FROM v_cash_flow", 0) //nolint:unqueryvet // every column is the point

	require.NoError(t, err)
	assert.Equal(t, []store.QueryColumn{
		{Name: "split_id", Type: "VARCHAR"},
		{Name: "transaction_id", Type: "VARCHAR"},
		{Name: "account_id", Type: "VARCHAR"},
		{Name: "date", Type: "DATE"},
		{Name: "month", Type: "DATE"},
		{Name: "currency", Type: "VARCHAR"},
		{Name: "category_id", Type: "VARCHAR"},
		{Name: "category", Type: "VARCHAR"},
		{Name: "payee_id", Type: "VARCHAR"},
		{Name: "payee", Type: "VARCHAR"},
		{Name: "flow", Type: "VARCHAR"},
		{Name: "amount", Type: "DECIMAL(18,2)"},
		{Name: "amount_cad", Type: "DECIMAL(18,2)"},
		{Name: "amount_usd", Type: "DECIMAL(18,2)"},
		{Name: "usd_cad", Type: "DECIMAL(10,6)"},
	}, got.Columns)
}

func Test_cash_flow_names_the_month_category_and_payee(t *testing.T) {
	t.Parallel()
	rows := reportRows()
	rows.Payees = []store.Payee{{ID: "payee-1", SourceID: 1, Name: "Coffee Shop"}}
	rows.Categories[0].FullPath = "Food:Groceries"
	rows.Splits = nil
	rows.Transactions = nil
	addSplit(&rows, splitSpec{id: "s1", category: new(catExpense), amount: -1250, payee: new("payee-1")})
	addSplit(&rows, splitSpec{id: "s2", amount: 300})
	st := newStoreWith(t, rows)

	got, err := st.Query(t.Context(), "SELECT split_id, transaction_id, account_id, date, month, currency, category_id, category, payee_id, payee, flow, amount FROM v_cash_flow ORDER BY split_id", 0)

	require.NoError(t, err)
	assert.Equal(t, []store.QueryValue{
		{Text: "s1"},
		{Text: "txn-s1"},
		{Text: acctInReports},
		{Text: "2026-03-15"},
		{Text: "2026-03-01"},
		{Text: "CAD"},
		{Text: catExpense},
		{Text: "Food:Groceries"},
		{Text: "payee-1"},
		{Text: "Coffee Shop"},
		{Text: "expense"},
		{Text: "-12.50"},
	}, textOnly(got.Rows[0]))
	assert.Equal(t, []store.QueryValue{
		{Text: "s2"},
		{Text: "txn-s2"},
		{Text: acctInReports},
		{Text: "2026-03-15"},
		{Text: "2026-03-01"},
		{Text: "CAD"},
		{Null: true, Text: "NULL"},
		{Null: true, Text: "NULL"},
		{Null: true, Text: "NULL"},
		{Null: true, Text: "NULL"},
		{Text: "income"},
		{Text: "3.00"},
	}, textOnly(got.Rows[1]))
}

func Test_cash_flow_takes_account_and_currency_from_the_transaction(t *testing.T) {
	t.Parallel()
	rows := reportRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctUSD, SourceID: 3, Name: "US Chequing", Type: "chequing", Currency: "USD", Active: true})
	addSplit(&rows, splitSpec{id: "in-usd-account", account: acctUSD, currency: "USD", category: new(catExpense), amount: -100})
	addSplit(&rows, splitSpec{id: "in-foreign-currency", currency: "EUR", category: new(catExpense), amount: -200})
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT split_id, account_id, currency FROM v_cash_flow ORDER BY split_id")

	assert.Equal(t, [][]string{
		{"in-foreign-currency", acctInReports, "EUR"},
		{"in-usd-account", acctUSD, "USD"},
		{keepSplit, acctInReports, "CAD"},
	}, got)
}

// textOnly drops each cell's Native so a row compares on its printed form and NULL flag.
func textOnly(row []store.QueryValue) []store.QueryValue {
	out := make([]store.QueryValue, len(row))
	for i, cell := range row {
		out[i] = store.QueryValue{Null: cell.Null, Text: cell.Text}
	}
	return out
}

func Test_cash_flow_view_carries_its_reports_note(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, reportRows())

	got := queryTexts(t, st, "SELECT comment FROM duckdb_views() WHERE view_name = 'v_cash_flow'")

	assert.Equal(t, [][]string{{"excludes accounts where accounts.in_reports is false or accounts.linked_tracking is true, as Quicken reports do."}}, got)
}

func Test_spending_view_carries_its_reports_note(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, reportRows())

	got := queryTexts(t, st, "SELECT comment FROM duckdb_views() WHERE view_name = 'v_spending'")

	assert.Equal(t, [][]string{{"expense splits of v_cash_flow with spent = -amount; same exclusions as v_cash_flow, so totals match quarry spend."}}, got)
}

func Test_spending_holds_expense_flow_with_its_sign_flipped(t *testing.T) {
	t.Parallel()
	rows := reportRows()
	addSplit(&rows, splitSpec{id: "refund", category: new(catExpense), amount: 250})
	addSplit(&rows, splitSpec{id: "salary", category: new(catIncome), amount: 500})
	addSplit(&rows, splitSpec{id: "uncategorized-in", amount: 300})
	addSplit(&rows, splitSpec{id: "uncategorized-out", amount: -400})
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT split_id, spent FROM v_spending ORDER BY split_id")

	assert.Equal(t, [][]string{{keepSplit, "10.00"}, {"refund", "-2.50"}, {"uncategorized-out", "4.00"}}, got)
}

func Test_spending_lists_its_columns_in_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, reportRows())

	got, err := st.Query(t.Context(), "SELECT * FROM v_spending", 0) //nolint:unqueryvet // every column is the point

	require.NoError(t, err)
	assert.Equal(t, []store.QueryColumn{
		{Name: "split_id", Type: "VARCHAR"},
		{Name: "transaction_id", Type: "VARCHAR"},
		{Name: "account_id", Type: "VARCHAR"},
		{Name: "date", Type: "DATE"},
		{Name: "month", Type: "DATE"},
		{Name: "currency", Type: "VARCHAR"},
		{Name: "category_id", Type: "VARCHAR"},
		{Name: "category", Type: "VARCHAR"},
		{Name: "payee_id", Type: "VARCHAR"},
		{Name: "payee", Type: "VARCHAR"},
		{Name: "spent", Type: "DECIMAL(18,2)"},
		{Name: "spent_cad", Type: "DECIMAL(18,2)"},
		{Name: "spent_usd", Type: "DECIMAL(18,2)"},
		{Name: "usd_cad", Type: "DECIMAL(10,6)"},
	}, got.Columns)
}

func Test_spending_holds_exactly_the_cash_flow_expense_rows(t *testing.T) {
	t.Parallel()
	rows := reportRows()
	addSplit(&rows, splitSpec{id: "from-leg", category: new(catExpense), amount: -500})
	addSplit(&rows, splitSpec{id: "unmatched-leg", category: new(catExpense), amount: -500})
	addSplit(&rows, splitSpec{id: "to-leg", category: new(catExpense), amount: 500})
	addSplit(&rows, splitSpec{id: "system", category: new(catSystem), amount: -500})
	addSplit(&rows, splitSpec{id: "excluded", category: new(catExpense), amount: -500, excluded: true})
	addSplit(&rows, splitSpec{id: "not-in-reports", account: acctNotReports, category: new(catExpense), amount: -500})
	addSplit(&rows, splitSpec{id: "linked", account: acctLinked, category: new(catExpense), amount: -500})
	addSplit(&rows, splitSpec{id: "zero", amount: 0})
	addSplit(&rows, splitSpec{id: "uncategorized-out", amount: -100})
	rows.Transfers = append(rows.Transfers,
		store.Transfer{ID: "xfer-1", FromSplitID: "from-leg", ToSplitID: new("peer-leg")},
		store.Transfer{ID: "xfer-2", FromSplitID: "unmatched-leg"},
		store.Transfer{ID: "xfer-3", FromSplitID: "peer-leg-2", ToSplitID: new("to-leg")})
	st := newStoreWith(t, rows)

	spending := queryTexts(t, st, "SELECT split_id FROM v_spending ORDER BY split_id")
	cashFlowExpense := queryTexts(t, st, "SELECT split_id FROM v_cash_flow WHERE flow = 'expense' ORDER BY split_id")

	assert.Equal(t, [][]string{{keepSplit}, {"uncategorized-out"}}, spending)
	assert.Equal(t, cashFlowExpense, spending)
}

const convertedAmounts = "SELECT amount_cad, amount_usd FROM v_cash_flow WHERE split_id = 'x'"

func Test_cash_flow_view_converts_a_split_at_the_latest_rate_on_or_before_its_date(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		date time.Time
		want string
	}{
		{name: "a_weekend_split_at_the_fridays_rate", date: march(14), want: "12.50"},
		{name: "a_split_on_a_rate_date_at_that_dates_rate", date: march(16), want: "13.00"},
		{name: "a_split_after_the_last_rate_at_the_last_rate", date: march(20), want: "13.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := fxRows()
			expense(&rows, "x", "USD", c.date, 1000)
			st := newStoreWithRates(t, rows, fridayAndMonday()...)

			got := queryTexts(t, st, convertedAmounts)

			assert.Equal(t, [][]string{{c.want, "10.00"}}, got)
		})
	}
}

func Test_cash_flow_rounds_half_a_cent_away_from_zero_and_below_half_down(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		currency string
		cents    int64
		rate     money.Rate
		want     []string
	}{
		{name: "USD to CAD at exactly half a cent", currency: "USD", cents: 10, rate: 1_250_000, want: []string{"0.13", "0.10"}},
		{name: "USD to CAD at exactly minus half a cent", currency: "USD", cents: -10, rate: 1_250_000, want: []string{"-0.13", "-0.10"}},
		{name: "USD to CAD just below half a cent", currency: "USD", cents: 10, rate: 1_249_999, want: []string{"0.12", "0.10"}},
		{name: "CAD to USD at exactly half a cent", currency: "CAD", cents: 20, rate: 1_600_000, want: []string{"0.20", "0.13"}},
		{name: "CAD to USD at exactly minus half a cent", currency: "CAD", cents: -20, rate: 1_600_000, want: []string{"-0.20", "-0.13"}},
		{name: "CAD to USD just below half a cent", currency: "CAD", cents: 20, rate: 1_600_001, want: []string{"0.20", "0.12"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := fxRows()
			expense(&rows, "x", c.currency, march(13), c.cents)
			st := newStoreWithRates(t, rows, ratesOn(13, c.rate, "FXUSDCAD"))

			got := queryTexts(t, st, convertedAmounts)

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}

func Test_cash_flow_keeps_an_amount_in_its_own_currency_when_the_store_has_no_rates(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "CAD", march(13), -250)
	expense(&rows, "y", "USD", march(13), -300)
	st := newStoreWithRates(t, rows)

	got := queryTexts(t, st, "SELECT split_id, amount_cad, amount_usd FROM v_cash_flow WHERE split_id IN ('x', 'y') ORDER BY split_id")

	assert.Equal(t, [][]string{{"x", "-2.50", "NULL"}, {"y", "NULL", "-3.00"}}, got)
}

func Test_cash_flow_leaves_the_other_currency_empty_for_a_date_before_the_first_rate(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "CAD", march(12), -250)
	expense(&rows, "y", "USD", march(12), -300)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, "SELECT split_id, amount_cad, amount_usd FROM v_cash_flow WHERE split_id IN ('x', 'y') ORDER BY split_id")

	assert.Equal(t, [][]string{{"x", "-2.50", "NULL"}, {"y", "NULL", "-3.00"}}, got)
}

func Test_cash_flow_carries_the_rate_in_force_on_each_rows_date(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "CAD", march(12), -250)
	expense(&rows, "y", "CAD", march(14), -250)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, "SELECT split_id, usd_cad FROM v_cash_flow WHERE split_id IN ('x', 'y') ORDER BY split_id")

	assert.Equal(t, [][]string{{"x", "NULL"}, {"y", "1.250000"}}, got)
}

func Test_cash_flow_leaves_a_third_currency_unconverted(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "EUR", march(13), -250)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, convertedAmounts)

	assert.Equal(t, [][]string{{"NULL", "NULL"}}, got)
}

func Test_spending_leaves_a_third_currency_unconverted(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "EUR", march(13), -250)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, "SELECT spent_cad, spent_usd FROM v_spending WHERE split_id = 'x'")

	assert.Equal(t, [][]string{{"NULL", "NULL"}}, got)
}

func Test_spending_keeps_an_amount_in_its_own_currency_when_the_store_has_no_rates(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "CAD", march(13), -250)
	expense(&rows, "y", "USD", march(13), -300)
	st := newStoreWithRates(t, rows)

	got := queryTexts(t, st, "SELECT split_id, spent_cad, spent_usd, usd_cad FROM v_spending WHERE split_id IN ('x', 'y') ORDER BY split_id")

	assert.Equal(t, [][]string{{"x", "2.50", "NULL", "NULL"}, {"y", "NULL", "3.00", "NULL"}}, got)
}

func Test_spending_carries_each_cash_flow_rows_converted_amounts_negated(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "USD", march(14), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, "SELECT spent, spent_cad, spent_usd, usd_cad FROM v_spending WHERE split_id = 'x'")

	assert.Equal(t, [][]string{{"10.00", "12.50", "10.00", "1.250000"}}, got)
}

func Test_money_convert_matches_every_spending_rows_converted_columns(t *testing.T) {
	t.Parallel()
	currencies := map[string]money.Currency{"CAD": money.CAD, "USD": money.USD}
	rateDates := []int{13, 16, 17}
	rows := fxRows()
	var id int
	for currency := range currencies {
		for _, cents := range []int64{-1, 1, 10, -10, 20, -20, 99, 12_345, -99_999_999} {
			for _, rateDate := range rateDates {
				id++
				expense(&rows, "s"+strconv.Itoa(id), currency, march(rateDate), cents)
			}
		}
	}
	st := newStoreWithRates(t, rows,
		ratesOn(13, 1_250_000, "FXUSDCAD"), ratesOn(16, 1_600_001, "FXUSDCAD"), ratesOn(17, 1_249_999, "FXUSDCAD"))
	got, err := st.Query(t.Context(), `SELECT currency, CAST(spent * 100 AS BIGINT), CAST(usd_cad * 1000000 AS BIGINT),
		CAST(spent_cad * 100 AS BIGINT), CAST(spent_usd * 100 AS BIGINT) FROM v_spending WHERE split_id <> 'keep'`, 0)
	require.NoError(t, err)

	require.Len(t, got.Rows, id)
	for _, row := range got.Rows {
		currency := currencies[row[0].Text]
		cents, rate := nativeInt(t, row[1]), money.Rate(nativeInt(t, row[2]))
		wantCAD, okCAD := money.Convert(cents, currency, money.CAD, rate)
		wantUSD, okUSD := money.Convert(cents, currency, money.USD, rate)
		require.True(t, okCAD && okUSD)
		assert.Equal(t, []int64{wantCAD, wantUSD}, []int64{nativeInt(t, row[3]), nativeInt(t, row[4])}, "%s %d at %d", row[0].Text, cents, rate)
	}
}

func nativeInt(t *testing.T, v store.QueryValue) int64 {
	t.Helper()
	n, ok := v.Native.(int64)
	require.True(t, ok, "cell %q is not a BIGINT", v.Text)
	return n
}

func balancesOf(t *testing.T, st *duckstore.Store, account string) [][]string {
	t.Helper()
	return queryTexts(t, st, "SELECT balance, balance_cad, balance_usd FROM v_account_balances WHERE id = '"+account+"'")
}

func Test_account_balances_convert_the_summed_balance_not_each_transaction(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "a", "USD", march(13), 10)
	expense(&rows, "b", "USD", march(14), 10)
	st := newStoreWithRates(t, rows, ratesOn(13, fridayRate, "FXUSDCAD"))

	got := balancesOf(t, st, acctUSD)

	assert.Equal(t, [][]string{{"0.20", "0.25", "0.20"}}, got)
}

func Test_account_balances_convert_at_the_latest_rate_dated_today_or_earlier(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "a", "USD", march(13), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := balancesOf(t, st, acctUSD)

	assert.Equal(t, [][]string{{"10.00", "13.00", "10.00"}}, got)
}

func Test_account_balances_ignore_a_rate_dated_after_today(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "a", "USD", march(13), 1000)
	future := store.Rate{Date: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), USDCAD: 2_000_000, Series: "FXUSDCAD"}
	st := newStoreWithRates(t, rows, ratesOn(13, fridayRate, "FXUSDCAD"), future)

	got := balancesOf(t, st, acctUSD)

	assert.Equal(t, [][]string{{"10.00", "12.50", "10.00"}}, got)
}

func Test_account_balances_convert_a_cad_balance_to_usd(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	st := newStoreWithRates(t, rows, ratesOn(13, fridayRate, "FXUSDCAD"))

	got := balancesOf(t, st, acctInReports)

	assert.Equal(t, [][]string{{"-10.00", "-10.00", "-8.00"}}, got)
}

func Test_account_balances_stay_in_their_own_currency_when_the_store_has_no_rates(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "a", "USD", march(13), 1000)
	st := newStoreWithRates(t, rows)

	got := queryTexts(t, st, "SELECT id, balance_cad, balance_usd FROM v_account_balances WHERE id IN ('"+acctInReports+"', '"+acctUSD+"') ORDER BY id")

	assert.Equal(t, [][]string{{acctInReports, "-10.00", "NULL"}, {acctUSD, "NULL", "10.00"}}, got)
}

func Test_account_balances_give_an_investment_account_with_nothing_held_zero_in_both_currencies(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, fxRows(), ratesOn(13, fridayRate, "FXUSDCAD"))

	got := balancesOf(t, st, acctInvestment)

	assert.Equal(t, [][]string{{"0.00", "0.00", "0.00"}}, got)
}

func Test_account_balances_leave_a_third_currency_unconverted(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	rows.Accounts = append(rows.Accounts, store.Account{ID: "acct-eur", SourceID: 5, Name: "Euro Chequing", Type: "chequing", Currency: "EUR", Active: true})
	st := newStoreWithRates(t, rows, ratesOn(13, fridayRate, "FXUSDCAD"))

	got := balancesOf(t, st, "acct-eur")

	assert.Equal(t, [][]string{{"0.00", "NULL", "NULL"}}, got)
}

func Test_account_balances_lists_its_columns_in_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, reportRows())

	got, err := st.Query(t.Context(), "SELECT * FROM v_account_balances", 0) //nolint:unqueryvet // every column is the point

	require.NoError(t, err)
	assert.Equal(t, []store.QueryColumn{
		{Name: "id", Type: "VARCHAR"},
		{Name: "source_id", Type: "BIGINT"},
		{Name: "name", Type: "VARCHAR"},
		{Name: "type", Type: "VARCHAR"},
		{Name: "currency", Type: "VARCHAR"},
		{Name: "institution", Type: "VARCHAR"},
		{Name: "closed", Type: "BOOLEAN"},
		{Name: "active", Type: "BOOLEAN"},
		{Name: "cash", Type: "DECIMAL(18,2)"},
		{Name: "holdings_value", Type: "DECIMAL(38,2)"},
		{Name: "balance", Type: "DECIMAL(38,2)"},
		{Name: "balance_cad", Type: "DECIMAL(38,2)"},
		{Name: "balance_usd", Type: "DECIMAL(38,2)"},
	}, got.Columns)
}

// dayQuery is the first day, last day and row count of v_balances_daily for account.
func dayQuery(account string) string {
	return fmt.Sprintf("SELECT CAST(min(date) AS VARCHAR), CAST(max(date) AS VARCHAR), count(*) = count(DISTINCT date) FROM v_balances_daily WHERE account_id = '%s'", account)
}

func Test_balances_daily_runs_from_the_first_transaction_through_today(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, march(3), 100)))

	got := queryTexts(t, st, dayQuery(acctOne))

	assert.Equal(t, [][]string{{"2026-03-03", localToday().Format(time.DateOnly), "true"}}, got)
}

func Test_balances_daily_starts_on_the_earlier_of_the_first_transaction_and_the_first_holding(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		txnDay      int
		holdingDay  int
		wantFirstOn string
	}{
		{name: "the transaction comes first", txnDay: 2, holdingDay: 5, wantFirstOn: "2026-03-02"},
		{name: "the holding comes first", txnDay: 5, holdingDay: 2, wantFirstOn: "2026-03-02"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := balanceRows(buy(acctOne, secAcme, 1, march(c.holdingDay), oneShare))
			rows.Transactions = []store.Transaction{transaction("t1", acctOne, march(c.txnDay), 100)}
			st := newStoreWith(t, rows)

			got := queryTexts(t, st, dayQuery(acctOne))

			assert.Equal(t, [][]string{{c.wantFirstOn, localToday().Format(time.DateOnly), "true"}}, got)
		})
	}
}

func Test_balances_daily_starts_on_the_first_holding_when_the_account_has_no_transaction(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows(buy(acctOne, secAcme, 1, march(4), oneShare)))

	got := queryTexts(t, st, dayQuery(acctOne))

	assert.Equal(t, [][]string{{"2026-03-04", localToday().Format(time.DateOnly), "true"}}, got)
}

func Test_balances_daily_has_no_rows_for_an_account_with_no_transaction_and_no_holding(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows(buy(acctOne, secAcme, 1, march(4), oneShare)))

	got := queryTexts(t, st, "SELECT count(*) FROM v_balances_daily WHERE account_id = '"+acctTwo+"'")

	assert.Equal(t, [][]string{{"0"}}, got)
}

func Test_balances_daily_has_no_rows_for_an_account_whose_only_transaction_is_future_dated(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, localToday().AddDate(0, 0, 3), 100)))

	got := queryTexts(t, st, "SELECT count(*) FROM v_balances_daily")

	assert.Equal(t, [][]string{{"0"}}, got)
}

func Test_balances_daily_lists_closed_left_out_and_linked_tracking_accounts(t *testing.T) {
	t.Parallel()
	rows := cashRows(
		transaction("t1", acctOne, march(1), 100), transaction("t2", acctTwo, march(1), 100),
		transaction("t3", acctChequing, march(1), 100))
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctTwo, SourceID: 2, Name: "Not In Reports", Type: "chequing", Currency: "CAD", Active: true, NotInReports: true},
		store.Account{ID: acctChequing, SourceID: 3, Name: "Linked", Type: "chequing", Currency: "CAD", Active: true, LinkedTracking: true})
	rows.Accounts[0].Closed = true
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT DISTINCT account_id FROM v_balances_daily ORDER BY account_id")

	assert.Equal(t, [][]string{{acctOne}, {acctTwo}, {acctChequing}}, got)
}

func Test_balances_daily_cash_is_the_running_sum_of_transactions_on_or_before_each_day(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(
		transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctOne, march(3), 5_000),
		transaction("t3", acctOne, march(3), -250)))

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), cash FROM v_balances_daily WHERE date <= '2026-03-04' ORDER BY date")

	assert.Equal(t, [][]string{
		{"2026-03-01", "100.00"}, {"2026-03-02", "100.00"}, {"2026-03-03", "147.50"}, {"2026-03-04", "147.50"},
	}, got)
}

func Test_balances_daily_cash_includes_a_transaction_excluded_from_reports(t *testing.T) {
	t.Parallel()
	excluded := transaction("t2", acctOne, march(1), 5_000)
	excluded.ExcludedFromReports = true
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, march(1), 10_000), excluded))

	got := queryTexts(t, st, "SELECT cash FROM v_balances_daily WHERE date = '2026-03-01'")

	assert.Equal(t, [][]string{{"150.00"}}, got)
}

func Test_balances_daily_cash_leaves_out_a_transaction_dated_after_today(t *testing.T) {
	t.Parallel()
	today := localToday()
	st := newStoreWith(t, cashRows(
		transaction("t1", acctOne, march(1), 10_000), transaction("t2", acctOne, today.AddDate(0, 0, 2), 50_000),
		transaction("t3", acctOne, today, 700)))

	got := queryTexts(t, st, "SELECT cash FROM v_balances_daily WHERE date = current_date")

	assert.Equal(t, [][]string{{"107.00"}}, got)
}

func Test_balances_daily_has_no_holdings_figures_outside_brokerage_and_retirement_accounts(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, march(1), 10_000)))

	got := queryTexts(t, st, "SELECT holdings_value, holdings_unvalued FROM v_balances_daily WHERE date = '2026-03-01'")

	assert.Equal(t, [][]string{{"NULL", "NULL"}}, got)
}

// holdingsQuery is account's holdings_value and holdings_unvalued on date.
func holdingsQuery(account, date string) string {
	return fmt.Sprintf("SELECT holdings_value, holdings_unvalued FROM v_balances_daily WHERE account_id = '%s' AND date = '%s'", account, date)
}

func Test_balances_daily_counts_each_holding_it_cannot_value(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		account string
		txns    []store.InvestmentTransaction
		prices  []store.Price
	}{
		{
			name: "a holding with no price", account: acctOne,
			txns:   []store.InvestmentTransaction{buy(acctOne, secControl, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(1), oneShare)},
			prices: []store.Price{quote(secControl, 1, march(1), tenUnits)},
		},
		{
			name: "a holding with no currency", account: acctOne,
			txns:   []store.InvestmentTransaction{buy(acctOne, secControl, 1, march(1), oneShare), buy(acctOne, secNoCurrency, 2, march(1), oneShare)},
			prices: []store.Price{quote(secControl, 1, march(1), tenUnits), quote(secNoCurrency, 2, march(1), tenUnits)},
		},
		{
			name: "a EUR holding in a CAD account", account: acctOne,
			txns:   []store.InvestmentTransaction{buy(acctOne, secControl, 1, march(1), oneShare), buy(acctOne, secEUR, 2, march(1), oneShare)},
			prices: []store.Price{quote(secControl, 1, march(1), tenUnits), quote(secEUR, 2, march(1), tenUnits)},
		},
		{
			name: "a USD holding in a CAD account before the first rate", account: acctOne,
			txns:   []store.InvestmentTransaction{buy(acctOne, secControl, 1, march(1), oneShare), buy(acctOne, secUSD, 2, march(1), oneShare)},
			prices: []store.Price{quote(secControl, 1, march(1), tenUnits), quote(secUSD, 2, march(1), tenUnits)},
		},
		{
			name: "a CAD holding in a USD account before the first rate", account: acctTwo,
			txns:   []store.InvestmentTransaction{buy(acctTwo, secUSD, 1, march(1), oneShare), buy(acctTwo, secControl, 2, march(1), oneShare)},
			prices: []store.Price{quote(secUSD, 1, march(1), tenUnits), quote(secControl, 2, march(1), tenUnits)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := balanceRows(c.txns...)
			rows.Prices = c.prices
			st := newStoreWithRates(t, rows)

			got := queryTexts(t, st, holdingsQuery(c.account, "2026-03-02"))

			assert.Equal(t, [][]string{{"10.00", "1"}}, got)
		})
	}
}

func Test_balances_daily_values_holdings_in_the_accounts_currency(t *testing.T) {
	t.Parallel()
	const oneAndATenth = 10_010_000
	cases := []struct {
		name         string
		account      string
		txns         []store.InvestmentTransaction
		prices       []store.Price
		wantValue    string
		wantUnvalued string
		onMarch      int
	}{
		{
			name: "an own-currency holding as is", account: acctOne, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), 3*oneShare)},
			prices:    []store.Price{quote(secAcme, 1, march(1), tenUnits)},
			wantValue: "30.00", wantUnvalued: "0",
		},
		{
			name: "a USD holding in a CAD account converted at the rate and rounded", account: acctOne, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctOne, secUSD, 1, march(1), oneShare)},
			prices:    []store.Price{quote(secUSD, 1, march(1), oneAndATenth)},
			wantValue: "12.51", wantUnvalued: "0",
		},
		{
			name: "a CAD holding in a USD account converted at the rate and rounded", account: acctTwo, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctTwo, secAcme, 1, march(1), oneShare)},
			prices:    []store.Price{quote(secAcme, 1, march(1), oneAndATenth)},
			wantValue: "8.01", wantUnvalued: "0",
		},
		{
			name: "a retirement account valued like a brokerage account", account: acctRetirement, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctRetirement, secAcme, 1, march(1), 2*oneShare)},
			prices:    []store.Price{quote(secAcme, 1, march(1), tenUnits)},
			wantValue: "20.00", wantUnvalued: "0",
		},
		{
			name: "a EUR holding in a EUR account as is", account: acctEUR, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctEUR, secEUR, 1, march(1), oneShare)},
			prices:    []store.Price{quote(secEUR, 1, march(1), tenUnits)},
			wantValue: "10.00", wantUnvalued: "0",
		},
		{
			name: "a CAD holding in a EUR account left out", account: acctEUR, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctEUR, secAcme, 1, march(1), oneShare)},
			prices:    []store.Price{quote(secAcme, 1, march(1), tenUnits)},
			wantValue: "0.00", wantUnvalued: "1",
		},
		{
			name: "nothing held on a day between spans", account: acctOne, onMarch: 5,
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, march(1), oneShare), buy(acctOne, secAcme, 2, march(3), -oneShare),
				buy(acctOne, secControl, 3, march(7), oneShare),
			},
			prices:    []store.Price{quote(secAcme, 1, march(1), tenUnits), quote(secControl, 2, march(7), tenUnits)},
			wantValue: "0.00", wantUnvalued: "0",
		},
		{
			name: "every holding unvalued", account: acctOne, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), oneShare)},
			prices:    nil,
			wantValue: "0.00", wantUnvalued: "1",
		},
		{
			name: "the largest holding without overflow", account: acctOne, onMarch: 2,
			txns:      []store.InvestmentTransaction{buy(acctOne, secAcme, 1, march(1), maxDecimal18x6)},
			prices:    []store.Price{quote(secAcme, 1, march(1), maxDecimal18x6)},
			wantValue: maxHoldingValue, wantUnvalued: "0",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := balanceRows(c.txns...)
			rows.Prices = c.prices
			st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

			got := queryTexts(t, st, holdingsQuery(c.account, fmt.Sprintf("2026-03-%02d", c.onMarch)))

			assert.Equal(t, [][]string{{c.wantValue, c.wantUnvalued}}, got)
		})
	}
}

// convertedQuery is account's balance, balance_cad, balance_usd and usd_cad on March day.
func convertedQuery(account string, day int) string {
	return fmt.Sprintf("SELECT balance, balance_cad, balance_usd, usd_cad FROM v_balances_daily WHERE account_id = '%s' AND date = '2026-03-%02d'", account, day)
}

func Test_balances_daily_converts_balance_at_the_rate_for_the_day(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		account string
		day     int
		want    []string
	}{
		{
			name: "a CAD account before the first rate keeps balance_cad and has no balance_usd", account: acctOne, day: 5,
			want: []string{"100.01", "100.01", "NULL", "NULL"},
		},
		{
			name: "a USD account before the first rate keeps balance_usd and has no balance_cad", account: acctTwo, day: 5,
			want: []string{"100.01", "NULL", "100.01", "NULL"},
		},
		{
			name: "a CAD account converts to USD at the rate and rounds", account: acctOne, day: 10,
			want: []string{"100.01", "100.01", "80.01", "1.250000"},
		},
		{
			name: "a USD account converts to CAD at the rate and rounds", account: acctTwo, day: 10,
			want: []string{"100.01", "125.01", "100.01", "1.250000"},
		},
		{
			name: "a day in a rate gap takes the prior rate", account: acctTwo, day: 15,
			want: []string{"100.01", "125.01", "100.01", "1.250000"},
		},
		{
			name: "a rate date takes its own rate", account: acctTwo, day: 16,
			want: []string{"100.01", "130.01", "100.01", "1.300000"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := balanceRows()
			rows.Transactions = []store.Transaction{
				transaction("t1", acctOne, march(1), 10_001), transaction("t2", acctTwo, march(1), 10_001),
			}
			st := newStoreWithRates(t, rows, ratesOn(10, 1_250_000, "FXUSDCAD"), ratesOn(16, 1_300_000, "FXUSDCAD"))

			got := queryTexts(t, st, convertedQuery(c.account, c.day))

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}

func Test_balances_daily_has_no_converted_balance_for_an_account_in_another_currency(t *testing.T) {
	t.Parallel()
	rows := balanceRows()
	rows.Transactions = []store.Transaction{transaction("t1", acctEUR, march(1), 10_000)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := queryTexts(t, st, convertedQuery(acctEUR, 5))

	assert.Equal(t, [][]string{{"100.00", "NULL", "NULL", "1.250000"}}, got)
}

func Test_balances_daily_balance_adds_valued_holdings_to_cash(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), 3*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), tenUnits)}
	rows.Transactions = []store.Transaction{transaction("t1", acctOne, march(1), 10_000)}
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT cash, holdings_value, balance FROM v_balances_daily WHERE date = '2026-03-02'")

	assert.Equal(t, [][]string{{"100.00", "30.00", "130.00"}}, got)
}

func Test_balances_daily_balance_stays_negative_when_the_cash_overdraws_past_the_holdings(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), 3*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), tenUnits)}
	rows.Transactions = []store.Transaction{transaction("t1", acctOne, march(1), -10_000)}
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT cash, holdings_value, balance FROM v_balances_daily WHERE date = '2026-03-02'")

	assert.Equal(t, [][]string{{"-100.00", "30.00", "-70.00"}}, got)
}

func Test_balances_daily_balance_is_cash_in_an_account_without_holdings_figures(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, march(1), 10_000)))

	got := queryTexts(t, st, "SELECT cash, balance FROM v_balances_daily WHERE date = '2026-03-01'")

	assert.Equal(t, [][]string{{"100.00", "100.00"}}, got)
}

func Test_balances_daily_balance_is_holdings_value_in_an_account_with_no_transaction(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(1), 3*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, march(1), tenUnits)}
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT cash, holdings_value, balance, balance_cad FROM v_balances_daily WHERE account_id = '"+acctOne+"' AND date = '2026-03-02'")

	assert.Equal(t, [][]string{{"0.00", "30.00", "30.00", "30.00"}}, got)
}

func Test_balances_daily_cash_is_zero_on_the_days_before_the_first_transaction(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, march(2), oneShare))
	rows.Transactions = []store.Transaction{transaction("t1", acctOne, march(5), 10_000)}
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT CAST(date AS VARCHAR), cash, balance FROM v_balances_daily WHERE account_id = '"+acctOne+"' AND date BETWEEN '2026-03-02' AND '2026-03-05' ORDER BY date")

	assert.Equal(t, [][]string{
		{"2026-03-02", "0.00", "0.00"}, {"2026-03-03", "0.00", "0.00"}, {"2026-03-04", "0.00", "0.00"}, {"2026-03-05", "100.00", "100.00"},
	}, got)
}

func Test_balances_daily_view_lists_its_columns_in_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows())

	got, err := st.Query(t.Context(), "SELECT * FROM v_balances_daily", 0) //nolint:unqueryvet // every column is the point

	require.NoError(t, err)
	assert.Equal(t, []store.QueryColumn{
		{Name: "date", Type: "DATE"},
		{Name: "account_id", Type: "VARCHAR"},
		{Name: "account", Type: "VARCHAR"},
		{Name: "type", Type: "VARCHAR"},
		{Name: "currency", Type: "VARCHAR"},
		{Name: "cash", Type: "DECIMAL(18,2)"},
		{Name: "holdings_value", Type: "DECIMAL(38,2)"},
		{Name: "holdings_unvalued", Type: "BIGINT"},
		{Name: "balance", Type: "DECIMAL(38,2)"},
		{Name: "balance_cad", Type: "DECIMAL(38,2)"},
		{Name: "balance_usd", Type: "DECIMAL(38,2)"},
		{Name: "usd_cad", Type: "DECIMAL(10,6)"},
	}, got.Columns)
}

func Test_balances_daily_view_carries_its_note(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows())

	got := queryTexts(t, st, "SELECT comment FROM duckdb_views() WHERE view_name = 'v_balances_daily'")

	assert.Equal(t, [][]string{{"one row per account per day from its first transaction or holding through today; " +
		"cash is the sum of its transactions to that day, holdings_value its holdings' value in its own currency " +
		"(NULL outside brokerage and retirement accounts), balance is cash plus holdings_value, " +
		"as quarry accounts and quarry networth use; filter by date."}}, got)
}
