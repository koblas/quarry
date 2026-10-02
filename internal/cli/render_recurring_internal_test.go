// White-box: renderRecurring is an unexported layout rule whose column widths and alignment are best driven directly.
package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
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
				Payee: "Rogers", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceMonthly, Amount: 9500, PerYear: new(int64(114000)),
				First: recurringDay(time.May, 3), Last: recurringDay(time.September, 3), State: report.SeriesActive,
			},
			{
				Payee: "Disney Plus", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceMonthly, Amount: 1199,
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
			Payee: "Foo\nBar", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceWeekly, Amount: 500, PerYear: new(int64(26000)),
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

func Test_renderRecurring_measures_the_payee_column_in_characters_not_bytes(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(),
		Series: []report.Series{
			{
				Payee: "Société", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceWeekly, Amount: 500, PerYear: new(int64(26000)),
				First: recurringDay(time.January, 5), Last: recurringDay(time.February, 5), State: report.SeriesActive,
			},
			{
				Payee: "Rogers", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceWeekly, Amount: 500, PerYear: new(int64(26000)),
				First: recurringDay(time.January, 5), Last: recurringDay(time.February, 5), State: report.SeriesActive,
			},
		},
	}

	got := renderRecurring(r)

	want := "" +
		"Recurring charges 2026-01-01 to 2026-03-09 in all accounts\n" +
		"\n" +
		"Payee    Currency  Every  Amount  Per year  First       Last        Status  Price changes\n" +
		"Société  CAD       week     5.00    260.00  2026-01-05  2026-02-05  active\n" +
		"Rogers   CAD       week     5.00    260.00  2026-01-05  2026-02-05  active\n"
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
					Payee: "A", Currency: "CAD", NativeCurrency: "CAD", Cadence: c.cadence, Amount: 100, PerYear: new(int64(5200)),
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
					Payee: "A", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceMonthly, Amount: 100,
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

func Test_renderRecurring_prints_the_ruled_sample_row_with_its_price_change(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(),
		Series: []report.Series{{
			Payee: "Rogers", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceMonthly, Amount: 9500, FirstAmount: 8500, NativeAmount: 9500, NativeFirstAmount: 8500, PerYear: new(int64(114000)),
			First: recurringDay(time.May, 3), Last: recurringDay(time.September, 3), State: report.SeriesActive,
			PriceChanges: []report.PriceChange{{Date: recurringDay(time.July, 3), From: 8500, To: 9500, Tenths: 118}},
			ChangeTenths: 118,
		}},
	}

	row := strings.Split(renderRecurring(r), "\n")[3]

	assert.Equal(t, "Rogers  CAD       month   95.00  1,140.00  2026-05-03  2026-09-03  active  1: 85.00 -> 95.00 (+11.8%)", row)
}

func Test_renderRecurring_writes_the_first_to_latest_change_in_the_price_changes_cell(t *testing.T) {
	cases := []struct {
		name         string
		changes      int
		first, last  int64
		changeTenths int64
		want         string
	}{
		{name: "a rise is signed plus", changes: 1, first: 8500, last: 9500, changeTenths: 118, want: "1: 85.00 -> 95.00 (+11.8%)"},
		{name: "a fall is signed minus", changes: 2, first: 1199, last: 1099, changeTenths: -83, want: "2: 11.99 -> 10.99 (-8.3%)"},
		{name: "no net change is unsigned", changes: 2, first: 10000, last: 10000, changeTenths: 0, want: "2: 100.00 -> 100.00 (0.0%)"},
		{name: "a sub-tenth fall that rounds to zero is unsigned", changes: 3, first: 10000, last: 9999, changeTenths: 0, want: "3: 100.00 -> 99.99 (0.0%)"},
		{name: "a fraction under one percent keeps its leading zero", changes: 1, first: 10000, last: 10050, changeTenths: 5, want: "1: 100.00 -> 100.50 (+0.5%)"},
		{name: "amounts of a thousand or more are grouped", changes: 1, first: 99900, last: 123456, changeTenths: 236, want: "1: 999.00 -> 1,234.56 (+23.6%)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := report.Recurring{
				Window: spendingWindow(),
				Series: []report.Series{{
					Payee: "A", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceMonthly, Amount: c.last, FirstAmount: c.first, NativeAmount: c.last, NativeFirstAmount: c.first,
					First: recurringDay(time.January, 5), Last: recurringDay(time.February, 5), State: report.SeriesEnded,
					PriceChanges: make([]report.PriceChange, c.changes), ChangeTenths: c.changeTenths,
				}},
			}

			row := strings.Split(renderRecurring(r), "\n")[3]

			assert.True(t, strings.HasSuffix(row, "  "+c.want), row)
		})
	}
}

func Test_renderRecurring_leaves_the_price_changes_cell_empty_without_a_change(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(),
		Series: []report.Series{{
			Payee: "A", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceMonthly, Amount: 1000, FirstAmount: 1040,
			First: recurringDay(time.January, 5), Last: recurringDay(time.February, 5), State: report.SeriesEnded,
			ChangeTenths: -38,
		}},
	}

	row := strings.Split(renderRecurring(r), "\n")[3]

	assert.True(t, strings.HasSuffix(row, "ended"), row)
}

func Test_renderRecurring_captions_the_named_accounts_escaped_and_joined(t *testing.T) {
	r := report.Recurring{
		Window:   spendingWindow(),
		Accounts: []store.Account{{ID: "acct-1", Name: "Foo\nBar"}, {ID: "acct-2", Name: "Chequing"}},
	}

	got := renderRecurring(r)

	assert.Contains(t, got, "Recurring charges 2026-01-01 to 2026-03-09 in Foo\\nBar, Chequing\n\n")
}

func Test_renderRecurring_shows_the_native_currency_after_a_converted_rows_currency(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(), Currency: money.CAD,
		Series: []report.Series{
			{
				Payee: "Gym", Currency: "CAD", NativeCurrency: "USD", Cadence: report.CadenceMonthly,
				Amount: 2100, FirstAmount: 1560, NativeAmount: 1500, NativeFirstAmount: 1200, PerYear: new(int64(25200)),
				First: recurringDay(time.February, 12), Last: recurringDay(time.September, 12), State: report.SeriesActive, New: true,
				PriceChanges: []report.PriceChange{{Date: recurringDay(time.June, 12), From: 1200, To: 1500, Tenths: 250}}, ChangeTenths: 250,
			},
			{
				Payee: "Rent", Currency: "CAD", NativeCurrency: "CAD", Cadence: report.CadenceMonthly,
				Amount: 5000, FirstAmount: 4000, NativeAmount: 5000, NativeFirstAmount: 4000, PerYear: new(int64(60000)),
				First: recurringDay(time.February, 12), Last: recurringDay(time.September, 12), State: report.SeriesActive,
				PriceChanges: []report.PriceChange{{Date: recurringDay(time.June, 12), From: 4000, To: 5000, Tenths: 250}}, ChangeTenths: 250,
			},
		},
		Totals: []report.RecurringTotal{{Currency: "CAD", PerYear: 85200}},
	}

	got := renderRecurring(r)

	want := "" +
		"Recurring charges 2026-01-01 to 2026-03-09 in all accounts, amounts in CAD\n" +
		"\n" +
		"Payee  Currency   Every  Amount  Per year  First       Last        Status       Price changes\n" +
		"Gym    CAD (USD)  month   21.00    252.00  2026-02-12  2026-09-12  active, new  1: USD 12.00 -> USD 15.00 (+25.0%)\n" +
		"Rent   CAD        month   50.00    600.00  2026-02-12  2026-09-12  active       1: 40.00 -> 50.00 (+25.0%)\n" +
		"Total  CAD                         852.00\n"
	assert.Equal(t, want, got)
}

func Test_renderRecurring_reads_the_price_change_in_the_native_amounts_not_the_converted_ones(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(), Currency: money.USD,
		Series: []report.Series{{
			Payee: "Hydro", Currency: "USD", NativeCurrency: "CAD", Cadence: report.CadenceMonthly,
			Amount: 900, FirstAmount: 900, NativeAmount: 1299, NativeFirstAmount: 999, PerYear: new(int64(10800)),
			First: recurringDay(time.February, 12), Last: recurringDay(time.September, 12), State: report.SeriesActive,
			PriceChanges: []report.PriceChange{{Date: recurringDay(time.June, 12), From: 999, To: 1299, Tenths: 300}}, ChangeTenths: 300,
		}},
	}

	row := strings.Split(renderRecurring(r), "\n")[3]

	assert.Equal(t, "Hydro  USD (CAD)  month    9.00    108.00  2026-02-12  2026-09-12  active  1: CAD 9.99 -> CAD 12.99 (+30.0%)", row)
	assert.Contains(t, renderRecurring(r), "in all accounts, amounts in USD\n")
}

func Test_renderRecurring_lists_an_ended_converted_series_without_per_year_or_total(t *testing.T) {
	r := report.Recurring{
		Window: spendingWindow(), Currency: money.CAD,
		Series: []report.Series{{
			Payee: "Old", Currency: "CAD", NativeCurrency: "USD", Cadence: report.CadenceMonthly, Amount: 1400, FirstAmount: 1400,
			NativeAmount: 1000, NativeFirstAmount: 1000,
			First: recurringDay(time.January, 5), Last: recurringDay(time.February, 5), State: report.SeriesEnded, New: true,
		}},
	}

	got := renderRecurring(r)

	want := "" +
		"Recurring charges 2026-01-01 to 2026-03-09 in all accounts, amounts in CAD\n" +
		"\n" +
		"Payee  Currency   Every  Amount  Per year  First       Last        Status      Price changes\n" +
		"Old    CAD (USD)  month   14.00            2026-01-05  2026-02-05  ended, new\n"
	assert.Equal(t, want, got)
}

func Test_renderRecurring_captions_native_mode_without_an_amounts_clause(t *testing.T) {
	got := renderRecurring(report.Recurring{Window: spendingWindow(), Currency: money.Native})

	assert.Contains(t, got, "in all accounts\n\n")
	assert.NotContains(t, got, "amounts in")
}
