package report_test

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// fakeStore answers each read with a canned result or fault; Query returns at most maxRows of rows,
// and the got*/accountsReads pointers, when set, record what Query, Spending, CashFlow and Accounts were given.
type fakeStore struct {
	status     store.Status
	accounts   store.AccountList
	spending   store.Spending
	cashFlow   store.CashFlow
	findings   store.FindingList
	rows       [][]store.QueryValue
	gotMaxRows *int

	gotSpending   *store.SpendingParams
	gotCashFlow   *store.CashFlowParams
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

func (f fakeStore) CashFlow(_ context.Context, params store.CashFlowParams) (store.CashFlow, error) {
	if f.gotCashFlow != nil {
		*f.gotCashFlow = params
	}
	return f.cashFlow, f.err
}

func (f fakeStore) Findings(context.Context) (store.FindingList, error) { return f.findings, f.err }

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
