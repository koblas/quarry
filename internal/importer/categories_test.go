package importer_test

import (
	"testing"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_import_refuses_a_category_with_an_unmapped_type(t *testing.T) {
	b := v9fixture.NewBuilder()
	b.Category(v9fixture.TagRow{Name: "Food:Groceries", Type: v9fixture.Int64Ptr(5)})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t, `category "Food:Groceries" has type 5, which quarry does not map yet`, importReason(t, err))
}

func Test_import_refuses_a_category_with_no_name(t *testing.T) {
	b := v9fixture.NewBuilder()
	catPK := b.Category(v9fixture.TagRow{Type: v9fixture.Int64Ptr(1)})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t, `category (source id `+itoa(catPK)+`) has no name`, importReason(t, err))
}

func Test_import_refuses_a_category_with_no_type(t *testing.T) {
	b := v9fixture.NewBuilder()
	b.Category(v9fixture.TagRow{Name: "Groceries"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t, `category "Groceries" has no type`, importReason(t, err))
}

func Test_import_reads_every_payee_and_user_tag(t *testing.T) {
	b := v9fixture.NewBuilder()
	payee1 := b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	payee2 := b.Payee(v9fixture.PayeeRow{Name: "Landlord"})
	tag1 := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	tag2 := b.UserTag(v9fixture.TagRow{Name: "Business"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"payee-" + itoa(payee1), "payee-" + itoa(payee2),
	}, payeeIDs(fake))
	assert.ElementsMatch(t, []string{
		"tag-" + itoa(tag1), "tag-" + itoa(tag2),
	}, tagIDs(fake))
}
