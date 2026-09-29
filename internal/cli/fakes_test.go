package cli_test

import (
	"context"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// fakeReportStore answers Accounts, Spending and Query with a canned result or fault, Query recording
// its query and maxRows in *gotQuery and *gotMaxRows, Spending its params in *gotSpending, when set;
// Status panics through the nil embedded interface.
type fakeReportStore struct {
	report.Store

	accounts    store.AccountList
	spending    store.Spending
	result      store.QueryResult
	gotQuery    *string
	gotMaxRows  *int
	gotSpending *store.SpendingParams
	err         error
}

func (f fakeReportStore) Accounts(context.Context) (store.AccountList, error) {
	return f.accounts, f.err
}

func (f fakeReportStore) Spending(_ context.Context, params store.SpendingParams) (store.Spending, error) {
	if f.gotSpending != nil {
		*f.gotSpending = params
	}
	return f.spending, f.err
}

func (f fakeReportStore) CashFlow(context.Context, store.CashFlowParams) (store.CashFlow, error) {
	return store.CashFlow{}, f.err
}

func (f fakeReportStore) Query(_ context.Context, query string, maxRows int) (store.QueryResult, error) {
	if f.gotQuery != nil {
		*f.gotQuery = query
	}
	if f.gotMaxRows != nil {
		*f.gotMaxRows = maxRows
	}
	return f.result, f.err
}

// failingWriter fails every write with err.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
