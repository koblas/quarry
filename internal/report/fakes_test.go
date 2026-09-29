package report_test

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// fakeStore answers each read with a canned result or fault; Query returns at most maxRows of rows,
// and the got*/accountsReads pointers, when set, record what Query, Spending and Accounts were given.
type fakeStore struct {
	status     store.Status
	accounts   store.AccountList
	spending   store.Spending
	rows       [][]store.QueryValue
	gotMaxRows *int

	gotSpending   *store.SpendingParams
	accountsReads *int
	err           error
}

func (f fakeStore) Status(context.Context) (store.Status, error) { return f.status, f.err }

func (f fakeStore) Accounts(context.Context) (store.AccountList, error) {
	if f.accountsReads != nil {
		*f.accountsReads++
	}
	return f.accounts, f.err
}

func (f fakeStore) Spending(_ context.Context, params store.SpendingParams) (store.Spending, error) {
	if f.gotSpending != nil {
		*f.gotSpending = params
	}
	return f.spending, f.err
}

func (f fakeStore) CashFlow(context.Context, store.CashFlowParams) (store.CashFlow, error) {
	return store.CashFlow{}, f.err
}

func (f fakeStore) Query(_ context.Context, _ string, maxRows int) (store.QueryResult, error) {
	if f.gotMaxRows != nil {
		*f.gotMaxRows = maxRows
	}
	rows := f.rows
	if maxRows > 0 {
		rows = rows[:min(maxRows, len(rows))]
	}
	return store.QueryResult{Rows: rows}, f.err
}
