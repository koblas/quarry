package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// The alphabetically-first offender is reported; "(and 2 more)" counts
// only the other two of its own class.
func Test_import_reports_the_first_offender_and_how_many_more(t *testing.T) {
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Bravo", Type: "ZZZ", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Alpha", Type: "ZZZ", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Charlie", Type: "ZZZ", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`account "Alpha" has type ZZZ, which quarry does not map yet (and 2 more)`,
		importReason(t, err))
}

// A missing entity outranks an account currency fault
// even though the account offender was added to the accumulator first.
func Test_import_reports_the_earliest_class_when_several_fail(t *testing.T) {
	b := v9fixture.NewBuilder().WithoutEntity("UserTag")
	b.Account(v9fixture.AccountRow{Name: "Euro Savings", Type: "CHECKING", Currency: "EUR", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		"the snapshot has no UserTag entity, which quarry needs to read Quicken's records",
		importReason(t, err))
}

// Within one class, an undated offender (account) must sort before a
// dated one (transaction).
func Test_import_reports_an_undated_offender_before_a_dated_one_in_the_same_class(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	goodAcct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	b.Transaction(v9fixture.TransactionRow{Account: goodAcct, PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `an account (source id `+itoa(acctPK)+`) has no name (and 1 more)`, importReason(t, err))
}
