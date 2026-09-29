package cli_test

import (
	"context"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// fakeReportStore answers Accounts and Query with a canned result or fault, Query recording its query
// and maxRows in *gotQuery and *gotMaxRows when set; Status panics through the nil embedded interface.
type fakeReportStore struct {
	report.Store

	accounts   store.AccountList
	result     store.QueryResult
	gotQuery   *string
	gotMaxRows *int
	err        error
}

func (f fakeReportStore) Accounts(context.Context) (store.AccountList, error) {
	return f.accounts, f.err
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
