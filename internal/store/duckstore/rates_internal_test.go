// White-box: needSpan takes the instant as a value, so fixed instants whose
// UTC and local dates differ are the only way to pin which date it reads
// without moving the process zone.
package duckstore

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_needSpan_ends_on_the_local_calendar_date_of_the_instant(t *testing.T) {
	t.Parallel()
	earliest := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	transactions := []store.Transaction{{Date: earliest}}
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "west of UTC, late evening is still the local day when UTC is the next",
			now:  time.Date(2026, 3, 14, 23, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60)),
			want: time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "east of UTC, just after local midnight is the new local day when UTC is the old",
			now:  time.Date(2026, 3, 15, 0, 30, 0, 0, time.FixedZone("UTC+9", 9*60*60)),
			want: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := needSpan(transactions, c.now)

			assert.Equal(t, store.DateSpan{First: earliest, Last: c.want}, got)
		})
	}
}

func Test_needSpan_is_empty_when_there_are_no_transactions(t *testing.T) {
	t.Parallel()

	got := needSpan(nil, time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))

	assert.Equal(t, store.DateSpan{}, got)
}
