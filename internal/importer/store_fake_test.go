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
	states        []finding.State
	carried       bool
	findingsFault *store.OpenError
	unreadable    bool
	rates         store.RatesSummary
	ratesFault    *store.OpenError
	nextErr       error
	replaceCalls  int
	shareCheck    store.ShareCheck
	shareErr      error
}

func (f *fakeStore) failNext(err error) { f.nextErr = err }

// CheckShares answers the configured shareCheck (default: nothing checked, no mismatch).
func (f *fakeStore) CheckShares(context.Context, store.Rows) (store.ShareCheck, error) {
	return f.shareCheck, f.shareErr
}

func (f *fakeStore) Replace(_ context.Context, rows store.Rows) (store.Replaced, error) {
	f.replaceCalls++
	if f.nextErr != nil {
		return store.Replaced{}, f.nextErr
	}
	f.Rows = rows
	return store.Replaced{
		Path: f.Path, HistoryFault: f.historyFault, Findings: f.findings, FindingStates: f.states, FindingsCarried: f.carried,
		FindingsFault: f.findingsFault, StoreUnreadable: f.unreadable, Rates: f.rates,
		RatesFault: f.ratesFault,
	}, nil
}
