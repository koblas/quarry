// White-box: monthEnds is the unexported calendar grid behind a net worth history; its edge combinations
// (month lengths, a bound on a month end, inverted bounds) are cheaper to pin directly than through NetWorth.
package report

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func utcDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func Test_monthEnds_lists_each_month_end_in_the_window_then_the_until_when_it_is_not_one(t *testing.T) {
	cases := []struct {
		name         string
		since, until time.Time
		want         []time.Time
	}{
		{
			name: "a since on a month end is listed", since: utcDay(2026, time.January, 31), until: utcDay(2026, time.March, 31),
			want: []time.Time{utcDay(2026, time.January, 31), utcDay(2026, time.February, 28), utcDay(2026, time.March, 31)},
		},
		{
			name: "a since the day after a month end starts at the next one", since: utcDay(2026, time.February, 1), until: utcDay(2026, time.March, 31),
			want: []time.Time{utcDay(2026, time.February, 28), utcDay(2026, time.March, 31)},
		},
		{
			name: "an until on a month end is not listed twice", since: utcDay(2026, time.January, 1), until: utcDay(2026, time.February, 28),
			want: []time.Time{utcDay(2026, time.January, 31), utcDay(2026, time.February, 28)},
		},
		{
			name: "an until mid-month is appended", since: utcDay(2026, time.January, 1), until: utcDay(2026, time.March, 12),
			want: []time.Time{utcDay(2026, time.January, 31), utcDay(2026, time.February, 28), utcDay(2026, time.March, 12)},
		},
		{
			name: "a one-day window mid-month is that day", since: utcDay(2026, time.March, 12), until: utcDay(2026, time.March, 12),
			want: []time.Time{utcDay(2026, time.March, 12)},
		},
		{
			name: "a one-day window on a month end is that day", since: utcDay(2026, time.January, 31), until: utcDay(2026, time.January, 31),
			want: []time.Time{utcDay(2026, time.January, 31)},
		},
		{
			name: "both bounds in one month list the until", since: utcDay(2026, time.March, 5), until: utcDay(2026, time.March, 12),
			want: []time.Time{utcDay(2026, time.March, 12)},
		},
		{
			name: "January 31 to February in a common year ends on the 28th", since: utcDay(2026, time.January, 31), until: utcDay(2026, time.February, 28),
			want: []time.Time{utcDay(2026, time.January, 31), utcDay(2026, time.February, 28)},
		},
		{
			name: "a leap February ends on the 29th", since: utcDay(2028, time.January, 31), until: utcDay(2028, time.March, 31),
			want: []time.Time{utcDay(2028, time.January, 31), utcDay(2028, time.February, 29), utcDay(2028, time.March, 31)},
		},
		{
			name: "December rolls into January", since: utcDay(2025, time.December, 15), until: utcDay(2026, time.January, 15),
			want: []time.Time{utcDay(2025, time.December, 31), utcDay(2026, time.January, 15)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := monthEnds(store.Window{Since: c.since, Until: c.until})

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_monthEnds_is_empty_when_the_since_is_after_the_until(t *testing.T) {
	got := monthEnds(store.Window{Since: utcDay(2027, time.January, 1), Until: utcDay(2026, time.September, 29)})

	assert.Empty(t, got)
}
