package importer_test

import (
	"context"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
)

// fakeStore captures the rows the last Replace call received and counts
// every call, standing in for a real Store so importer tests never link
// DuckDB.
type fakeStore struct {
	Rows          store.Rows
	Path          string
	historyFault  *store.OpenError
	findings      finding.Counts
	carried       bool
	findingsFault *store.OpenError
	unreadable    bool
	nextErr       error
	replaceCalls  int
}

func (f *fakeStore) failNext(err error) { f.nextErr = err }

func (f *fakeStore) Replace(_ context.Context, rows store.Rows) (store.Replaced, error) {
	f.replaceCalls++
	if f.nextErr != nil {
		return store.Replaced{}, f.nextErr
	}
	f.Rows = rows
	return store.Replaced{
		Path: f.Path, HistoryFault: f.historyFault, Findings: f.findings, FindingsCarried: f.carried,
		FindingsFault: f.findingsFault, StoreUnreadable: f.unreadable,
	}, nil
}
