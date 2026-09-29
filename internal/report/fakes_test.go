package report_test

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// fakeStore answers each read with a canned result or fault; Query returns
// at most maxRows of rows, recording the maxRows it was asked for in *gotMaxRows when set.
type fakeStore struct {
	status     store.Status
	accounts   store.AccountList
	rows       [][]store.QueryValue
	gotMaxRows *int
	err        error
}

func (f fakeStore) Status(context.Context) (store.Status, error) { return f.status, f.err }

func (f fakeStore) Accounts(context.Context) (store.AccountList, error) { return f.accounts, f.err }

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
