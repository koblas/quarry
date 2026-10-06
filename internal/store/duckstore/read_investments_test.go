package duckstore_test

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// investmentReads is each read that carries the read-time investment inputs; queriesBefore is how many read
// queries run before its securities read, and the transactions read follows it.
var investmentReads = []struct {
	name          string
	read          func(context.Context, *duckstore.Store) error
	queriesBefore int
}{
	{"findings", func(ctx context.Context, st *duckstore.Store) error { _, err := st.Findings(ctx); return err }, 2},
	{"status", func(ctx context.Context, st *duckstore.Store) error { _, err := st.Status(ctx); return err }, 3},
}

// investmentFaultStages is the two investment reads in order; extraPasses is how many read queries run for real between them.
var investmentFaultStages = []struct {
	name        string
	extraPasses int
}{
	{"securities read", 0},
	{"investment transactions read", 1},
}

// investmentHistoryRows is a store with an add_shares in the first account, a buy in the second, and a dividend naming no security.
func investmentHistoryRows() (store.Rows, store.InvestmentTransaction, store.InvestmentTransaction) {
	add := historyTxn(2, acctOne, secAcme, 2, 2*oneShare)
	add.Action, add.CostBasis = "add_shares", new(int64(500))
	earlier := historyTxn(5, acctTwo, secUSD, 1, oneShare)
	cashOnly := historyTxn(1, acctOne, secAcme, 1, 0)
	cashOnly.Action, cashOnly.Shares, cashOnly.SecurityID, cashOnly.Amount = "dividend", nil, nil, 500
	return holdingRows(add, earlier, cashOnly), add, earlier
}

func Test_findings_reads_every_security_and_every_investment_transaction_with_a_security(t *testing.T) {
	t.Parallel()
	rows, add, earlier := investmentHistoryRows()
	st := newStoreWith(t, rows)

	list, err := st.Findings(t.Context())

	require.NoError(t, err)
	assert.Equal(t, store.Investments{
		Securities: []store.Security{
			{ID: secAcme, SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
			{ID: secEUR, SourceID: 3, Name: "Euro Fund", Ticker: new("EURF"), Currency: new("EUR")},
			{ID: secNoCurrency, SourceID: 4, Name: "Plain Fund"},
			{ID: secUSD, SourceID: 2, Name: "Globex Inc", Ticker: new("GLBX"), Currency: new("USD")},
		},
		Transactions: []store.InvestmentTransaction{earlier, add},
	}, list.Investments)
}

func Test_status_reads_every_security_and_every_investment_transaction_with_a_security(t *testing.T) {
	t.Parallel()
	rows, add, earlier := investmentHistoryRows()
	st := newStoreWith(t, rows)

	got, err := st.Status(t.Context())

	require.NoError(t, err)
	assert.Equal(t, store.Investments{
		Securities: []store.Security{
			{ID: secAcme, SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
			{ID: secEUR, SourceID: 3, Name: "Euro Fund", Ticker: new("EURF"), Currency: new("EUR")},
			{ID: secNoCurrency, SourceID: 4, Name: "Plain Fund"},
			{ID: secUSD, SourceID: 2, Name: "Globex Inc", Ticker: new("GLBX"), Currency: new("USD")},
		},
		Transactions: []store.InvestmentTransaction{earlier, add},
	}, got.Investments)
}

func Test_findings_and_status_return_an_investment_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, read := range investmentReads {
		for _, stage := range investmentFaultStages {
			t.Run(read.name+" "+stage.name, func(t *testing.T) {
				t.Parallel()
				fault := ioFault(`query rows "SELECT id"`)
				st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: read.queriesBefore + stage.extraPasses, queryFault: fault}))

				err := read.read(t.Context(), st)

				assertOtherFault(t, err, "disk read failed")
				assert.ErrorIs(t, err, fault)
			})
		}
	}
}

func Test_findings_and_status_return_an_investment_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, read := range investmentReads {
		for _, stage := range investmentFaultStages {
			t.Run(read.name+" "+stage.name, func(t *testing.T) {
				t.Parallel()
				st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: read.queriesBefore + stage.extraPasses, scanFault: errScanFailed}))

				err := read.read(t.Context(), st)

				assertOtherFault(t, err, errScanFailed.Error())
				assert.ErrorIs(t, err, errScanFailed)
			})
		}
	}
}
