package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_recurring_marks_a_series_new_only_when_its_first_charge_is_on_or_after_the_window_start(t *testing.T) {
	cases := []struct {
		name    string
		since   string
		until   string
		wantNew bool
	}{
		{name: "first charge on the window start", since: "2026-07-17", until: "2026-12-31", wantNew: true},
		{name: "first charge the day before the window start", since: "2026-07-18", until: "2026-12-31", wantNew: false},
		{name: "first charge on the window end", since: "2000-01-01", until: "2026-07-17", wantNew: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			series := monthlyEndingOn(t, "2026-09-15", 3)
			window := store.Window{Since: dateOf(t, c.since), Until: dateOf(t, c.until)}

			result := recurringIn(t, window, series)

			require.Len(t, result.Series, 1)
			assert.Equal(t, c.wantNew, result.Series[0].New)
		})
	}
}
