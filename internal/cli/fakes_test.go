package cli_test

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/require"
)

// spendNow is the clock every report command test runs at: 2026-09-29 noon UTC.
var spendNow = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// fakeReportStore answers each read with a canned result or err, recording its arguments in the
// matching got* field when set; Status panics through the nil embedded interface.
type fakeReportStore struct {
	report.Store

	accounts    store.AccountList
	spending    store.Spending
	cashFlow    store.CashFlow
	result      store.QueryResult
	gotQuery    *string
	gotMaxRows  *int
	gotSpending *store.SpendingParams
	gotCashFlow *store.CashFlowParams
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

func (f fakeReportStore) CashFlow(_ context.Context, params store.CashFlowParams) (store.CashFlow, error) {
	if f.gotCashFlow != nil {
		*f.gotCashFlow = params
	}
	return f.cashFlow, f.err
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

// span is the range of transaction dates from first to last, both YYYY-MM-DD.
func span(t *testing.T, first, last string) store.TransactionRange {
	t.Helper()
	from, err := time.Parse(time.DateOnly, first)
	require.NoError(t, err)
	to, err := time.Parse(time.DateOnly, last)
	require.NoError(t, err)
	return store.TransactionRange{First: from, Last: to}
}
