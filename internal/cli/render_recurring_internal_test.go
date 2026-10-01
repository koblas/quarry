// White-box: renderRecurring is an unexported layout rule whose column widths and alignment are best driven directly.
package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
)

func recurringDay(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 0, 0, 0, 0, time.UTC)
}

func Test_renderRecurring_pads_each_column_and_trims_the_trailing_spaces_of_a_row(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(),
		Series: []report.Series{
			{
				Payee: "Rogers", Currency: "CAD", Cadence: report.CadenceMonthly, Amount: 9500, PerYear: new(int64(114000)),
				First: recurringDay(time.May, 3), Last: recurringDay(time.September, 3), State: report.SeriesActive,
			},
			{
				Payee: "Disney Plus", Currency: "CAD", Cadence: report.CadenceMonthly, Amount: 1199,
				First: recurringDay(time.February, 7), Last: recurringDay(time.April, 7), State: report.SeriesEnded,
			},
		},
		Totals: []report.RecurringTotal{{Currency: "CAD", PerYear: 114000}},
	}

	got := renderRecurring(r)

	want := "" +
		"Recurring charges 2026-01-01 to 2026-03-09 in all accounts\n" +
		"\n" +
		"Payee        Currency  Every  Amount  Per year  First       Last        Status  Price changes\n" +
		"Rogers       CAD       month   95.00  1,140.00  2026-05-03  2026-09-03  active\n" +
		"Disney Plus  CAD       month   11.99            2026-02-07  2026-04-07  ended\n" +
		"Total        CAD                      1,140.00\n"
	assert.Equal(t, want, got)
}

func Test_renderRecurring_measures_the_payee_column_after_escaping_the_name(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(),
		Series: []report.Series{{
			Payee: "Foo\nBar", Currency: "CAD", Cadence: report.CadenceWeekly, Amount: 500, PerYear: new(int64(26000)),
			First: recurringDay(time.January, 5), Last: recurringDay(time.February, 5), State: report.SeriesActive,
		}},
	}

	got := renderRecurring(r)

	want := "" +
		"Recurring charges 2026-01-01 to 2026-03-09 in all accounts\n" +
		"\n" +
		"Payee     Currency  Every  Amount  Per year  First       Last        Status  Price changes\n" +
		`Foo\nBar  CAD       week     5.00    260.00  2026-01-05  2026-02-05  active` + "\n"
	assert.Equal(t, want, got)
}

func Test_renderRecurring_names_each_cadence_in_the_Every_cell(t *testing.T) {
	cases := []struct {
		cadence report.Cadence
		want    string
	}{
		{cadence: report.CadenceWeekly, want: "week"},
		{cadence: report.CadenceMonthly, want: "month"},
		{cadence: report.CadenceQuarterly, want: "quarter"},
		{cadence: report.CadenceAnnual, want: "year"},
	}

	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			r := report.Recurring{
				Window: spendingWindow(),
				Series: []report.Series{{
					Payee: "A", Currency: "CAD", Cadence: c.cadence, Amount: 100, PerYear: new(int64(5200)),
					First: recurringDay(time.January, 5), Last: recurringDay(time.February, 5), State: report.SeriesActive,
				}},
			}

			row := strings.Split(renderRecurring(r), "\n")[3]

			assert.Equal(t, []string{"A", "CAD", c.want, "1.00", "52.00", "2026-01-05", "2026-02-05", "active"}, strings.Fields(row))
		})
	}
}

func Test_renderRecurring_adds_new_to_the_status_of_a_new_series(t *testing.T) {
	cases := []struct {
		name  string
		state report.SeriesState
		want  string
	}{
		{name: "active", state: report.SeriesActive, want: "active, new"},
		{name: "ended", state: report.SeriesEnded, want: "ended, new"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := report.Recurring{
				Window: spendingWindow(),
				Series: []report.Series{{
					Payee: "A", Currency: "CAD", Cadence: report.CadenceMonthly, Amount: 100,
					First: recurringDay(time.January, 5), Last: recurringDay(time.February, 5), State: c.state, New: true,
				}},
			}

			row := strings.Split(renderRecurring(r), "\n")[3]

			assert.True(t, strings.HasSuffix(row, "  "+c.want), row)
		})
	}
}

func Test_renderRecurring_keeps_the_currency_totals_in_the_order_given(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(),
		Totals: []report.RecurringTotal{{Currency: "USD", PerYear: 1000}, {Currency: "CAD", PerYear: 2000}},
	}

	lines := strings.Split(renderRecurring(r), "\n")

	assert.Equal(t, []string{"Total", "USD", "10.00"}, strings.Fields(lines[3]))
	assert.Equal(t, []string{"Total", "CAD", "20.00"}, strings.Fields(lines[4]))
}

func Test_renderRecurring_prints_the_caption_and_header_only_for_no_series(t *testing.T) {
	r := report.Recurring{Window: spendingWindow()}

	got := renderRecurring(r)

	want := "" +
		"Recurring charges 2026-01-01 to 2026-03-09 in all accounts\n" +
		"\n" +
		"Payee  Currency  Every  Amount  Per year  First  Last  Status  Price changes\n"
	assert.Equal(t, want, got)
}
