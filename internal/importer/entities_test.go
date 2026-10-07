package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
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
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})

	fake, _ := importOK(t, b)

	assert.Len(t, fake.Rows.Transactions, 1)
}

func Test_import_refuses_when_one_required_entity_is_missing(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("UserTag")
	fake := &fakeStore{}

	_, err := importBuilt(t, fake, b)

	var unmappable *importer.UnmappableError
	require.ErrorAs(t, err, &unmappable)
	assert.Equal(t, "the snapshot has no UserTag entity, which quarry needs to read Quicken's records", unmappable.Reason)
}

func Test_import_refuses_when_several_required_entities_are_missing(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder().WithoutEntity("CategoryTag").WithoutEntity("UserTag")
	fake := &fakeStore{}

	_, err := importBuilt(t, fake, b)

	var unmappable *importer.UnmappableError
	require.ErrorAs(t, err, &unmappable)
	assert.Equal(t,
		"the snapshot has no CategoryTag or UserTag entity, which quarry needs to read Quicken's records",
		unmappable.Reason)
}
