package importer_test

import (
	"errors"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errShareCheckFault = errors.New("scratch database unavailable")

func oneShareMismatch() store.ShareCheck {
	return store.ShareCheck{
		Checked:    2,
		Mismatched: []store.ShareMismatch{{AccountID: "acct-1", SecurityID: "sec-1", Quarry: 1000000, Quicken: 2000000}},
	}
}

func Test_import_does_not_replace_the_store_when_share_counts_differ(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.NewBuilder().WriteBundle(t, t.TempDir())
	fake := &fakeStore{shareCheck: oneShareMismatch()}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, store.ErrValidationFailed)
	assert.Zero(t, fake.replaceCalls)
	assert.False(t, result.Built)
	assert.Equal(t, 2, result.Validation.Shares.Checked)
	require.Len(t, result.Validation.Shares.Mismatched, 1)
	assert.Equal(t, "acct-1", result.Validation.Shares.Mismatched[0].AccountID)
}

func Test_import_replaces_the_store_and_records_the_holdings_checked_when_share_counts_match(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.NewBuilder().WriteBundle(t, t.TempDir())
	fake := &fakeStore{shareCheck: store.ShareCheck{Checked: 3}}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, 1, fake.replaceCalls)
	assert.Equal(t, 3, result.Validation.Shares.Checked)
	require.Len(t, fake.Rows.ImportRuns, 1)
	assert.Equal(t, 3, fake.Rows.ImportRuns[0].SharesChecked)
}

func Test_import_reports_share_counts_alongside_a_balance_failure(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.01"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{shareCheck: store.ShareCheck{Checked: 4}}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, store.ErrValidationFailed)
	assert.Len(t, result.Validation.Balances.Mismatched, 1)
	assert.Equal(t, 4, result.Validation.Shares.Checked)
}

func Test_import_returns_the_share_check_error_without_replacing_the_store(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.NewBuilder().WriteBundle(t, t.TempDir())
	fake := &fakeStore{shareErr: errShareCheckFault}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, errShareCheckFault)
	require.NotErrorIs(t, err, store.ErrValidationFailed)
	require.EqualError(t, err, "check share counts: scratch database unavailable")
	assert.Zero(t, fake.replaceCalls)
}
