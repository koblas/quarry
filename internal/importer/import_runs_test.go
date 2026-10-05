package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_import_hands_the_store_one_import_run_describing_the_build(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := chequingWithOneReconciledTxn(b, "100.00")
	day := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.00"})
	transferLeg(b, chequingPK, "-5.00", 101, "Old Visa")
	savingsPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	transferLeg(b, chequingPK, "-20.00", 201, "202")
	transferLeg(b, savingsPK, "15.00", 202, "201")
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	investmentWithEntry(b, v9fixture.TransactionRow{Type: new(int64(3)), Account: brokeragePK, Amount: "-40.00", PostedDate: &day})
	bundle := b.WriteBundle(t, t.TempDir())
	snap := store.SnapshotRef{Path: bundle.DataPath, SHA256: "9f86d081", SchemaFingerprint: "sha256:abc"}
	fake := &fakeStore{}
	before := time.Now().UTC()

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), snap)

	after := time.Now().UTC()
	require.NoError(t, err)
	require.Len(t, fake.Rows.ImportRuns, 1)
	run := fake.Rows.ImportRuns[0]
	assert.Equal(t, store.ImportRun{
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Snapshot: snap,
		Counts:          store.Counts{Accounts: 3, Transactions: 5, Splits: 5, Transfers: 2, InvestmentTransactions: 1},
		BalancesChecked: 1, TransfersOneSided: 1,
		BalancesNeverReconciled: 1, InvestmentAccounts: 1, TransfersPaired: 1, TransfersCrossCurrency: 1,
	}, run)
	assert.Equal(t, time.UTC, run.StartedAt.Location())
	assert.Equal(t, time.UTC, run.FinishedAt.Location())
	assert.False(t, run.StartedAt.Before(before))
	assert.False(t, run.FinishedAt.Before(run.StartedAt))
	assert.False(t, after.Before(run.FinishedAt))
}

func Test_import_counts_securities_and_prices_in_the_import_run(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acmePK := newAcme(b)
	barePK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund", Currency: "USD"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay1, ClosingPrice: "12.5"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: acmePK, QuoteDate: &priceDay2, ClosingPrice: "13"})
	b.SecurityQuote(v9fixture.SecurityQuoteRow{Security: barePK, QuoteDate: &priceDay1, ClosingPrice: "4"})

	fake, _ := importSecurities(t, b)

	require.Len(t, fake.Rows.ImportRuns, 1)
	assert.Equal(t, store.Counts{Accounts: 1, Securities: 2, Prices: 3}, fake.Rows.ImportRuns[0].Counts)
}

func Test_import_reports_the_history_fault_the_store_returns(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fault := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: "/store/quarry.duckdb"}
	fake := &fakeStore{historyFault: fault}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.True(t, result.Built)
	assert.Same(t, fault, result.HistoryFault)
}

func Test_import_reports_no_history_fault_when_the_store_returns_none(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.True(t, result.Built)
	assert.Nil(t, result.HistoryFault)
}

func Test_import_passes_the_carry_faults_through_to_the_result(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: "/store/quarry.duckdb", Reason: "its findings table repeats an id"}
	fake := &fakeStore{findingsFault: fault, unreadable: true}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Same(t, fault, result.FindingsFault)
	assert.True(t, result.StoreUnreadable)
}

func Test_import_passes_the_rates_fault_through_to_the_result(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: "/store/quarry.duckdb", Reason: "its fx_rates table repeats a date"}
	fake := &fakeStore{ratesFault: fault}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Same(t, fault, result.RatesFault)
}

func Test_import_returns_the_stores_findings_counts(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	counts := finding.Counts{Open: 4, New: 3}

	result, err := importer.NewServer(importer.WithStore(&fakeStore{findings: counts})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, counts, result.Findings)
}

func Test_import_passes_the_finding_states_through_to_the_result(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	states := []finding.State{{ID: "uncategorized:payee-1", New: true}, {ID: "duplicate:txn-1:txn-2", Fixed: true, NewlyFixed: true}}

	result, err := importer.NewServer(importer.WithStore(&fakeStore{states: states})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, states, result.FindingStates)
}

func Test_import_returns_whether_the_stores_findings_were_carried(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{carried: true})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.True(t, result.FindingsCarried)
}

func Test_import_passes_the_rates_summary_through_to_the_result(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	rates := store.RatesSummary{
		First: time.Date(2005, 3, 1, 0, 0, 0, 0, time.UTC), Last: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Added: 7, FetchError: "unreachable",
	}

	result, err := importer.NewServer(importer.WithStore(&fakeStore{rates: rates})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, rates, result.Rates)
}
