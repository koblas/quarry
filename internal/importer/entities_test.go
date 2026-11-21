package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
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

func Test_import_refuses_when_required_entities_are_missing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		builder func() *v9fixture.Builder
		want    string
	}{
		{
			name: "one_entity", builder: func() *v9fixture.Builder { return v9fixture.NewBuilder().WithoutEntity("UserTag") },
			want: "the snapshot has no UserTag entity, which quarry needs to read Quicken's records",
		},
		{
			name: "several_entities", builder: func() *v9fixture.Builder {
				return v9fixture.NewBuilder().WithoutEntity("CategoryTag").WithoutEntity("UserTag")
			},
			want: "the snapshot has no CategoryTag or UserTag entity, which quarry needs to read Quicken's records",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := c.builder()

			reason, _ := importRefused(t, b)

			assert.Equal(t, c.want, reason)
		})
	}
}

func Test_import_refuses_when_a_looked_up_entity_is_listed_more_than_once(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		withoutEnt string
		duplicated []string
		want       string
	}{
		{
			name: "one_required_entity", duplicated: []string{"CategoryTag"},
			want: "the snapshot lists the CategoryTag entity more than once, so quarry cannot tell which records are its",
		},
		{
			name: "one_optional_entity", duplicated: []string{"Security"},
			want: "the snapshot lists the Security entity more than once, so quarry cannot tell which records are its",
		},
		{
			name: "two_entities_in_sorted_order", duplicated: []string{"Security", "CategoryTag"},
			want: "the snapshot lists the CategoryTag and Security entities more than once, so quarry cannot tell which records are theirs",
		},
		{
			name: "three_entities", duplicated: []string{"UserTag", "Security", "CategoryTag"},
			want: "the snapshot lists the CategoryTag, Security and UserTag entities more than once, " +
				"so quarry cannot tell which records are theirs",
		},
		{
			name: "a_missing_entity_is_reported_instead", withoutEnt: "UserTag", duplicated: []string{"CategoryTag"},
			want: "the snapshot has no UserTag entity, which quarry needs to read Quicken's records",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			if c.withoutEnt != "" {
				b.WithoutEntity(c.withoutEnt)
			}
			dataPath := snapshotPath(t, b)
			listEntitiesAgain(t, dataPath, c.duplicated...)

			reason, fake := importRefusedFrom(t, dataPath)

			assert.Equal(t, c.want, reason)
			assert.Empty(t, fake.Rows.Transactions)
		})
	}
}
