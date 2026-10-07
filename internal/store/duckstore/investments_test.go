package duckstore_test

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acctClosedRRSP = "acct-3"
	acctOpenRRSP   = "acct-4"
)

// historyTxn is a buy of shares millionths of security in account on March date, with no amount and every optional column unset.
func historyTxn(source int64, account, security string, date int, shares int64) store.InvestmentTransaction {
	txn := buy(account, security, source, march(date), shares)
	txn.Amount = 0
	return txn
}

// readHistory builds a store of holdingRows(txns...) plus a closed and an open retirement account and two rates, and reads its history.
func readHistory(t *testing.T, txns ...store.InvestmentTransaction) store.InvestmentHistory {
	t.Helper()
	rows := holdingRows(txns...)
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctClosedRRSP, SourceID: 3, Name: "Old RRSP", Type: store.AccountTypeRetirement, Currency: "CAD", Closed: true},
		store.Account{ID: acctOpenRRSP, SourceID: 4, Name: "RRSP", Type: store.AccountTypeRetirement, Currency: "CAD", Active: true})
	st := newStoreWithRates(t, rows, ratesOn(13, 1_250_000, "FXUSDCAD"), ratesOn(16, 1_310_000, "IEXE0101"))
	history, err := st.InvestmentHistory(t.Context())
	require.NoError(t, err)
	return history
}

func Test_investment_history_reads_every_investment_transaction_with_its_cost(t *testing.T) {
	t.Parallel()
	reinvest := historyTxn(9, acctTwo, secUSD, 1, 500_000)
	reinvest.Action, reinvest.CostBasis, reinvest.Currency, reinvest.Memo = "reinvest_dividend", new(int64(1640)), "USD", new("drip")
	closedSell := historyTxn(2, acctClosedRRSP, secAcme, 2, 333_333)
	closedSell.Action, closedSell.Amount = "sell", 800
	openBuy := historyTxn(5, acctOne, secAcme, 2, 2_500_000)
	openBuy.Amount, openBuy.Commission = -250_099, new(int64(99_900))
	split := historyTxn(7, acctOne, secUSD, 3, 0)
	split.Action, split.Shares = "split", nil
	split.SplitNewShares, split.SplitOldShares = new(int64(2_000_000)), new(int64(1_000_000))
	registered := historyTxn(8, acctOpenRRSP, secAcme, 4, oneShare)
	cashOnly := historyTxn(1, acctOne, secAcme, 1, 0)
	cashOnly.Action, cashOnly.Shares, cashOnly.SecurityID, cashOnly.Amount = "dividend", nil, nil, 500

	history := readHistory(t, openBuy, split, cashOnly, registered, closedSell, reinvest)

	assert.Equal(t, []store.InvestmentTransaction{reinvest, closedSell, openBuy, split, registered}, history.Transactions)
}

func Test_investment_history_reads_every_account_security_and_rate(t *testing.T) {
	t.Parallel()

	history := readHistory(t)

	assert.Equal(t, []store.Account{
		{ID: acctOne, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
		{ID: acctTwo, Name: "Brokerage USD", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true},
		{ID: acctClosedRRSP, Name: "Old RRSP", Type: store.AccountTypeRetirement, Currency: "CAD", Closed: true},
		{ID: acctOpenRRSP, Name: "RRSP", Type: store.AccountTypeRetirement, Currency: "CAD", Active: true},
	}, history.Accounts)
	assert.Equal(t, []store.Security{
		{ID: secAcme, SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: secEUR, SourceID: 3, Name: "Euro Fund", Ticker: new("EURF"), Currency: new("EUR")},
		{ID: secNoCurrency, SourceID: 4, Name: "Plain Fund"},
		{ID: secUSD, SourceID: 2, Name: "Globex Inc", Ticker: new("GLBX"), Currency: new("USD")},
	}, history.Securities)
	assert.Equal(t, []store.Rate{ratesOn(13, 1_250_000, "FXUSDCAD"), ratesOn(16, 1_310_000, "IEXE0101")}, history.Rates)
}

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
