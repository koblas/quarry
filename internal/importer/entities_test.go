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

// WithEntity overrides all three away from their default numbers; Import
// must still find each by name, not its usual Z_ENT.
func Test_import_resolves_entities_by_name_from_z_primarykey(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().
		WithEntity("CashFlowTransaction", 9001).
		WithEntity("CategoryTag", 9002).
		WithEntity("UserTag", 9003)
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Len(t, fake.Rows.Transactions, 1)
}

func Test_import_refuses_when_one_required_entity_is_missing(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("UserTag")
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	var unmappable *importer.UnmappableError
	require.ErrorAs(t, err, &unmappable)
	assert.Equal(t, "the snapshot has no UserTag entity, which quarry needs to read Quicken's records", unmappable.Reason)
}

func Test_import_refuses_when_several_required_entities_are_missing(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("CategoryTag").WithoutEntity("UserTag")
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	var unmappable *importer.UnmappableError
	require.ErrorAs(t, err, &unmappable)
	assert.Equal(t,
		"the snapshot has no CategoryTag or UserTag entity, which quarry needs to read Quicken's records",
		unmappable.Reason)
}

func Test_import_fails_when_the_snapshot_path_does_not_exist(t *testing.T) {
	t.Parallel()
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: "/no/such/snapshot.sqlite"})

	require.Error(t, err)
}
