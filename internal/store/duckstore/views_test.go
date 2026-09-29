package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acctInReports  = "acct-in"
	acctNotReports = "acct-out"
	catExpense     = "cat-expense"
	catIncome      = "cat-income"
	catSystem      = "cat-system"
	keepSplit      = "keep"
)

// reportRows is a store whose only view-visible split is `keep`, an expense.
// Its unmatched transfer leg has a NULL to_split_id, as real stores do.
func reportRows() store.Rows {
	rows := store.Rows{
		Accounts: []store.Account{
			{ID: acctInReports, SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
			{ID: acctNotReports, SourceID: 2, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
		},
		Categories: []store.Category{
			{ID: catExpense, SourceID: 1, Name: "Groceries", FullPath: "Groceries", Kind: "expense"},
			{ID: catIncome, SourceID: 2, Name: "Salary", FullPath: "Salary", Kind: "income"},
			{ID: catSystem, SourceID: 3, Name: "Adjustment", FullPath: "Adjustment", Kind: "system"},
		},
		Transfers: []store.Transfer{{ID: "xfer-orphan", FromSplitID: "orphan-leg"}},
	}
	addSplit(&rows, splitSpec{id: keepSplit, category: new(catExpense), amount: -1000})
	return rows
}

// splitSpec is one split and the transaction carrying it; account defaults to acctInReports.
type splitSpec struct {
	id       string
	account  string
	category *string
	amount   int64
	excluded bool
	payee    *string
}

func addSplit(rows *store.Rows, spec splitSpec) {
	account := spec.account
	if account == "" {
		account = acctInReports
	}
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-" + spec.id, SourceID: int64(len(rows.Transactions) + 1), AccountID: account,
		Date: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), PayeeID: spec.payee,
		Amount: spec.amount, Currency: "CAD", Status: "uncleared", ExcludedFromReports: spec.excluded,
	})
	rows.Splits = append(rows.Splits, store.Split{
		ID: spec.id, SourceID: int64(len(rows.Splits) + 1), TransactionID: "txn-" + spec.id,
		CategoryID: spec.category, Amount: spec.amount,
	})
}

func newStoreWith(t *testing.T, rows store.Rows) *duckstore.Store {
	t.Helper()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), rows)
	require.NoError(t, err)
	return duckstore.New(dir)
}

// queryTexts runs query and returns every cell as DuckDB's text for it.
func queryTexts(t *testing.T, st *duckstore.Store, query string) [][]string {
	t.Helper()
	got, err := st.Query(t.Context(), query, 0)
	require.NoError(t, err)
	texts := make([][]string, len(got.Rows))
	for i, row := range got.Rows {
		texts[i] = make([]string, len(row))
		for j, cell := range row {
			texts[i][j] = cell.Text
		}
	}
	return texts
}

func Test_cash_flow_leaves_out_what_quicken_reports_leave_out(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		add  splitSpec
		xfer *store.Transfer
	}{
		{
			name: "the from leg of a paired transfer",
			add:  splitSpec{id: "left-out", category: new(catExpense), amount: -500},
			xfer: &store.Transfer{ID: "xfer-1", FromSplitID: "left-out", ToSplitID: new("peer-leg")},
		},
		{
			name: "the to leg of a paired transfer",
			add:  splitSpec{id: "left-out", category: new(catExpense), amount: 500},
			xfer: &store.Transfer{ID: "xfer-1", FromSplitID: "peer-leg", ToSplitID: new("left-out")},
		},
		{
			name: "an unmatched transfer leg with no transfer account",
			add:  splitSpec{id: "left-out", category: new(catExpense), amount: -500},
			xfer: &store.Transfer{ID: "xfer-1", FromSplitID: "left-out"},
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
		{name: "a zero-amount uncategorized split", add: splitSpec{id: "left-out", amount: 0}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := reportRows()
			addSplit(&rows, c.add)
			if c.xfer != nil {
				rows.Transfers = append(rows.Transfers, *c.xfer)
			}
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
	addSplit(&rows, splitSpec{id: "expense-under-income", category: new("cat-income-child"), amount: -100})
	addSplit(&rows, splitSpec{id: "income-under-expense", category: new("cat-expense-child"), amount: 200})
	st := newStoreWith(t, rows)

	got := queryTexts(t, st, "SELECT split_id, flow FROM v_cash_flow WHERE split_id <> 'keep' ORDER BY split_id")

	assert.Equal(t, [][]string{{"expense-under-income", "expense"}, {"income-under-expense", "income"}}, got)
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

	got, err := st.Query(t.Context(), "SELECT * FROM v_cash_flow ORDER BY split_id", 0) //nolint:unqueryvet // every column is the point

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
	}, got.Columns)
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

	assert.Equal(t, [][]string{{"excludes accounts where accounts.in_reports is false, as Quicken reports do."}}, got)
}
