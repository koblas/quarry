package importer_test

import (
	"testing"
	"time"

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
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	b.Transaction(v9fixture.TransactionRow{Entity: v9fixture.EntInvestmentTransaction, Account: brokeragePK, Amount: "-40.00", PostedDate: &day})
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
		ID: 1, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Snapshot: snap,
		Counts:          store.Counts{Accounts: 2, Transactions: 2, Splits: 2, Transfers: 1},
		BalancesChecked: 1, TransfersOneSided: 1, InvestmentTransactionsNotImported: 1,
	}, run)
	assert.Equal(t, time.UTC, run.StartedAt.Location())
	assert.Equal(t, time.UTC, run.FinishedAt.Location())
	assert.False(t, run.StartedAt.Before(before))
	assert.False(t, run.FinishedAt.Before(run.StartedAt))
	assert.False(t, after.Before(run.FinishedAt))
}
