package duckstore_test

import (
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

const (
	acctInvestment = "acct-brokerage"
	fridayRate     = 1_250_000
	mondayRate     = 1_300_000
)

// fxRows is reportRows with a USD account that Quicken's reports count.
func fxRows() store.Rows {
	rows := reportRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctUSD, SourceID: 3, Name: "US Chequing", Type: "chequing", Currency: "USD", Active: true},
		store.Account{ID: acctInvestment, SourceID: 4, Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true})
	return rows
}

// expense adds an expense split of cents in currency on date, on the account of that currency.
func expense(rows *store.Rows, id, currency string, date time.Time, cents int64) {
	account := acctInReports
	if currency == "USD" {
		account = acctUSD
	}
	addSplit(rows, splitSpec{id: id, account: account, currency: currency, date: date, category: new(catExpense), amount: cents})
}

const convertedAmounts = "SELECT amount_cad, amount_usd FROM v_cash_flow WHERE split_id = 'x'"

func Test_cash_flow_converts_a_weekend_split_at_the_fridays_rate(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "USD", march(14), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, convertedAmounts)

	assert.Equal(t, [][]string{{"12.50", "10.00"}}, got)
}

func Test_cash_flow_converts_a_split_on_a_rate_date_at_that_dates_rate(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "USD", march(16), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, convertedAmounts)

	assert.Equal(t, [][]string{{"13.00", "10.00"}}, got)
}

func Test_cash_flow_converts_a_split_after_the_last_rate_at_the_last_rate(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "USD", march(20), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, convertedAmounts)

	assert.Equal(t, [][]string{{"13.00", "10.00"}}, got)
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
