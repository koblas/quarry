package duckstore_test

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
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
