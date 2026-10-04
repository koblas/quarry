package report_test

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// fakeStore answers each read with a canned result or fault; Query returns at most maxRows of rows,
// and the got*/accountsReads/holdingsReads/chargesReads pointers, when set, record what Query, Spending, CashFlow, Charges, Search, Accounts and Holdings were given or how often.
type fakeStore struct {
	status     store.Status
	accounts   store.AccountList
	spending   store.Spending
	cashFlow   store.CashFlow
	charges    store.Charges
	findings   store.FindingList
	holdings   store.Holdings
	search     store.Search
	schema     store.Schema
	rows       [][]store.QueryValue
	gotMaxRows *int

	gotSpending   *store.SpendingParams
	gotCashFlow   *store.CashFlowParams
	gotCharges    *store.ChargeParams
	gotSearch     *store.SearchParams
	gotHoldings   *store.HoldingsParams
	accountsReads *int
	holdingsReads *int
	chargesReads  *int
	schemaReads   *int
	spendingReads *int
	cashFlowReads *int
	err           error
	// chargesErr, when set, is what Charges fails with instead of err.
	chargesErr error
}

func (f fakeStore) Status(context.Context) (store.Status, error) { return f.status, f.err }

func (f fakeStore) Schema(context.Context) (store.Schema, error) {
	if f.schemaReads != nil {
		*f.schemaReads++
	}
	return f.schema, f.err
}

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
	if f.spendingReads != nil {
		*f.spendingReads++
	}
	return f.spending, f.err
}

func (f fakeStore) CashFlow(_ context.Context, params store.CashFlowParams) (store.CashFlow, error) {
	if f.gotCashFlow != nil {
		*f.gotCashFlow = params
	}
	if f.cashFlowReads != nil {
		*f.cashFlowReads++
	}
	return f.cashFlow, f.err
}

func (f fakeStore) Charges(_ context.Context, params store.ChargeParams) (store.Charges, error) {
	if f.gotCharges != nil {
		*f.gotCharges = params
	}
	if f.chargesReads != nil {
		*f.chargesReads++
	}
	if f.chargesErr != nil {
		return store.Charges{}, f.chargesErr
	}
	return f.charges, f.err
}

func (f fakeStore) Search(_ context.Context, params store.SearchParams) (store.Search, error) {
	if f.gotSearch != nil {
		*f.gotSearch = params
	}
	return f.search, f.err
}

func (f fakeStore) Holdings(_ context.Context, params store.HoldingsParams) (store.Holdings, error) {
	if f.gotHoldings != nil {
		*f.gotHoldings = params
	}
	if f.holdingsReads != nil {
		*f.holdingsReads++
	}
	return f.holdings, f.err
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

func (f fakeStore) NetWorth(context.Context, store.NetWorthParams) (store.NetWorth, error) {
	return store.NetWorth{}, f.err
}
