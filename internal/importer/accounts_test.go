package importer_test

import (
	"testing"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func importReason(t *testing.T, err error) string {
	t.Helper()
	var unmappable *importer.UnmappableError
	require.ErrorAs(t, err, &unmappable, "expected *importer.UnmappableError, got %v", err)
	return unmappable.Reason
}

func Test_import_refuses_an_account_with_an_unsupported_currency(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Euro Savings", Type: "CHECKING", Currency: "EUR", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `account "Euro Savings" uses currency EUR; quarry supports CAD and USD accounts`, importReason(t, err))
}

func Test_import_refuses_an_account_with_an_unmapped_type(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "X", Type: "ZZZ", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `account "X" has type ZZZ, which quarry does not map yet`, importReason(t, err))
}

func Test_import_refuses_an_account_with_no_name(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `an account (source id `+itoa(acctPK)+`) has no name`, importReason(t, err))
}

func Test_import_refuses_an_account_with_no_type(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `account "Chequing" has no type`, importReason(t, err))
}

func Test_import_refuses_an_account_with_no_currency(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `account "Chequing" has no currency`, importReason(t, err))
}

// A deleted account with a bad currency and no type must not be reported:
// row filters run before any mapping check, so this row never reaches one.
func Test_import_excludes_a_deleted_account_with_a_bad_currency_and_no_type(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Old", Currency: "EUR", Deleted: true})
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Len(t, fake.Rows.Accounts, 1)
	assert.Equal(t, "Chequing", fake.Rows.Accounts[0].Name)
}

func Test_import_marks_an_account_that_uses_linked_account_tracking(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "A on", Type: "CHECKING", Currency: "CAD", Active: true, SimpleInvesting: new(int64(1))})
	b.Account(v9fixture.AccountRow{Name: "B off", Type: "CHECKING", Currency: "CAD", Active: true, SimpleInvesting: new(int64(0))})
	b.Account(v9fixture.AccountRow{Name: "C unset", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Accounts, 3)
	assert.True(t, fake.Rows.Accounts[0].LinkedTracking, "1 is on")
	assert.False(t, fake.Rows.Accounts[1].LinkedTracking, "0 is off")
	assert.False(t, fake.Rows.Accounts[2].LinkedTracking, "NULL is off")
	assert.False(t, fake.Rows.Accounts[0].NotInReports, "the flags are independent")
}

func Test_import_marks_an_account_quicken_leaves_out_of_reports(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "A off", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: new(int64(0))})
	b.Account(v9fixture.AccountRow{Name: "B on", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: new(int64(1))})
	b.Account(v9fixture.AccountRow{Name: "C unset", Type: "CHECKING", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "D two", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: new(int64(2))})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Accounts, 4)
	assert.True(t, fake.Rows.Accounts[0].NotInReports, "0 is off")
	assert.False(t, fake.Rows.Accounts[1].NotInReports, "1 is on")
	assert.False(t, fake.Rows.Accounts[2].NotInReports, "NULL is on")
	assert.False(t, fake.Rows.Accounts[3].NotInReports, "any non-zero is on")
}
