package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var easternDaylight = time.FixedZone("EDT", -4*60*60)

// summaryOf is a summary of the month whose first day is start, read in zone, from a snapshot taken at taken.
func summaryOf(start time.Time, zone *time.Location, taken time.Time, coverage report.SnapshotCoverage) report.Summary {
	return report.Summary{
		Month:    report.Month{Start: start, End: start.AddDate(0, 1, -1), Zone: zone},
		Status:   store.Status{Run: store.ImportRun{Snapshot: store.SnapshotRef{TakenAt: taken}}},
		Coverage: coverage,
	}
}

func Test_SnapshotWarning_says_nothing_unless_the_snapshot_is_known_to_miss_part_of_the_month(t *testing.T) {
	cases := []struct {
		name     string
		coverage report.SnapshotCoverage
	}{
		{name: "the snapshot covers the month", coverage: report.SnapshotCovers},
		{name: "a coverage the report does not define", coverage: report.SnapshotCoverage(99)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := summaryOf(civil(2026, time.September, 1), easternDaylight, civil(2026, time.October, 2), c.coverage)

			assert.Empty(t, document.SnapshotWarning(summary, "run quarry summary again"))
		})
	}
}

func Test_SnapshotWarning_names_when_the_snapshot_was_taken_and_the_month_it_missed(t *testing.T) {
	taken := time.Date(2026, time.September, 28, 18, 2, 0, 0, time.UTC)
	summary := summaryOf(civil(2026, time.September, 1), easternDaylight, taken, report.SnapshotPredatesMonthEnd)

	got := document.SnapshotWarning(summary, "run quarry summary again")

	assert.Equal(t, "the store was built from a snapshot taken 2026-09-28 14:02 EDT, before September 2026 ended, "+
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, "+
		"then run quarry summary again", got)
}

func Test_SnapshotWarning_ends_with_the_phrase_the_surface_gives_for_running_the_report_again(t *testing.T) {
	const body = "the store was built from a snapshot taken 2026-08-30 20:00 UTC, before August 2026 ended, " +
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, then "
	cases := []struct {
		name  string
		again string
	}{
		{name: "the command", again: "run quarry summary again"},
		{name: "the command with a month", again: "run quarry summary --month 2026-08 again"},
		{name: "the tool", again: "call monthly_summary again"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			taken := time.Date(2026, time.August, 30, 20, 0, 0, 0, time.UTC)
			summary := summaryOf(civil(2026, time.August, 1), time.UTC, taken, report.SnapshotPredatesMonthEnd)

			assert.Equal(t, body+c.again, document.SnapshotWarning(summary, c.again))
		})
	}
}

// pinMachineZone sets the zone of the machine to UTC-4 named EDT, so a time read in it differs from UTC.
func pinMachineZone(t *testing.T) {
	t.Helper()
	previous := time.Local                      //nolint:gosmopolitan // saved to restore
	time.Local = easternDaylight                //nolint:gosmopolitan // restored by Cleanup
	t.Cleanup(func() { time.Local = previous }) //nolint:gosmopolitan // restores the zone swapped above
}

func Test_SnapshotWarning_reads_a_month_parsed_from_a_UTC_clock_in_UTC_on_any_machine(t *testing.T) {
	pinMachineZone(t)
	month := "2026-09"
	parsed, err := report.ParseMonth(&month, time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	summary := report.Summary{
		Month:    parsed,
		Status:   store.Status{Run: store.ImportRun{Snapshot: store.SnapshotRef{TakenAt: time.Date(2026, time.September, 28, 18, 2, 0, 0, time.UTC)}}},
		Coverage: report.SnapshotPredatesMonthEnd,
	}

	got := document.SnapshotWarning(summary, "run quarry summary again")

	assert.Contains(t, got, "taken 2026-09-28 18:02 UTC,")
}

func Test_SnapshotWarning_reads_the_snapshot_time_in_the_zone_of_the_month(t *testing.T) {
	pinMachineZone(t)
	taken := time.Date(2026, time.September, 28, 18, 2, 0, 0, time.UTC)
	cases := []struct {
		name string
		zone *time.Location
		want string
	}{
		{name: "eastern daylight time", zone: easternDaylight, want: "taken 2026-09-28 14:02 EDT,"},
		{name: "UTC", zone: time.UTC, want: "taken 2026-09-28 18:02 UTC,"},
		{name: "no zone reads as UTC", zone: nil, want: "taken 2026-09-28 18:02 UTC,"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := summaryOf(civil(2026, time.September, 1), c.zone, taken, report.SnapshotPredatesMonthEnd)

			assert.Contains(t, document.SnapshotWarning(summary, "run quarry summary again"), c.want)
		})
	}
}

func Test_SnapshotWarning_says_it_cannot_tell_when_the_snapshot_records_no_time(t *testing.T) {
	summary := summaryOf(civil(2026, time.September, 1), easternDaylight, time.Time{}, report.SnapshotTimeUnknown)

	got := document.SnapshotWarning(summary, "run quarry summary again")

	assert.Equal(t, "cannot tell whether the store holds all of September 2026: "+
		"its snapshot's manifest does not record when it was taken; "+
		"open your Quicken file and run quarry sync to take a new snapshot", got)
}

func Test_SnapshotWarning_names_any_month_and_year(t *testing.T) {
	cases := []struct {
		name  string
		start time.Time
		want  string
	}{
		{name: "a leap February", start: civil(2024, time.February, 1), want: "all of February 2024:"},
		{name: "the first month the calendar has", start: civil(1, time.January, 1), want: "all of January 0001:"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := summaryOf(c.start, time.UTC, time.Time{}, report.SnapshotTimeUnknown)

			assert.Contains(t, document.SnapshotWarning(summary, "run quarry summary again"), c.want)
		})
	}
}

const (
	unconvertedNoRates = "the store has no exchange rates, so amounts are listed in each account's own currency; " +
		"run quarry sync to fetch them"
	timeUnknownLine = "cannot tell whether the store holds all of September 2026: its snapshot's manifest does not record when it was " +
		"taken; open your Quicken file and run quarry sync to take a new snapshot"
	againPhrase = "run quarry summary again"
)

// calmSummary is a September 2026 summary whose snapshot covers the month and whose three sections each hold
// something and need no exchange rate, so it has no warning of any kind.
func calmSummary() report.Summary {
	s := summaryOf(civil(2026, time.September, 1), time.UTC, time.Time{}, report.SnapshotCovers)
	s.Anomalies = report.Anomalies{Currency: money.CAD, Checked: 3}
	s.Recurring = report.Recurring{Currency: money.CAD, Series: []report.Series{{}}}
	s.NetWorth = rateHistory(money.CAD, rateDay(time.March, 1),
		report.NetWorthDate{Date: rateDay(time.August, 31), Rows: []store.NetWorthRow{convertedRow("chequing", 80_000)}})
	return s
}

func Test_SummaryWarnings_says_the_store_has_no_rates_once_however_many_sections_need_one(t *testing.T) {
	cases := []struct {
		name            string
		charges, series int
	}{
		{name: "charges and series both listed unconverted", charges: 2, series: 3},
		{name: "only charges listed unconverted", charges: 2},
		{name: "only series listed unconverted", series: 3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := calmSummary()
			s.Anomalies.Unconverted = store.Unconverted{Transactions: c.charges}
			s.Recurring.Unconverted = store.Unconverted{Transactions: c.series}

			assert.Equal(t, []string{unconvertedNoRates}, document.SummaryWarnings(s, againPhrase, document.NativeFlag))
		})
	}
}

func Test_SummaryWarnings_counts_the_charges_and_series_dated_before_the_first_rate(t *testing.T) {
	first := rateDay(time.October, 2)
	cases := []struct {
		name            string
		charges, series int
		want            []string
	}{
		{name: "one of each", charges: 1, series: 1, want: []string{
			"1 charge dated before 2026-10-02, the first exchange rate in the store, is listed in USD, not converted to CAD",
			"1 series with a charge dated before 2026-10-02, the first exchange rate in the store, is listed in USD, not converted to CAD",
		}},
		{name: "several of each", charges: 2, series: 3, want: []string{
			"2 charges dated before 2026-10-02, the first exchange rate in the store, are listed in USD, not converted to CAD",
			"3 series with a charge dated before 2026-10-02, the first exchange rate in the store, are listed in USD, not converted to CAD",
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := calmSummary()
			s.Anomalies.Unconverted = store.Unconverted{Transactions: c.charges, FirstRate: first}
			s.Recurring.Unconverted = store.Unconverted{Transactions: c.series, FirstRate: first}

			assert.Equal(t, c.want, document.SummaryWarnings(s, againPhrase, document.NativeFlag))
		})
	}
}

func Test_SummaryWarnings_lists_the_snapshot_then_charge_then_series_then_rate_warning(t *testing.T) {
	first := rateDay(time.October, 2)
	s := summaryOf(civil(2026, time.September, 1), time.UTC, time.Time{}, report.SnapshotTimeUnknown)
	s.Anomalies = report.Anomalies{Currency: money.CAD, Checked: 3, Unconverted: store.Unconverted{Transactions: 2, FirstRate: first}}
	s.Recurring = report.Recurring{Currency: money.CAD, Series: []report.Series{{}}, Unconverted: store.Unconverted{Transactions: 3, FirstRate: first}}
	s.NetWorth = rateHistory(money.CAD, first,
		report.NetWorthDate{Date: rateDay(time.August, 31), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}},
		report.NetWorthDate{Date: rateDay(time.September, 30), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}})

	got := document.SummaryWarnings(s, againPhrase, document.NativeFlag)

	assert.Equal(t, []string{
		timeUnknownLine,
		"2 charges dated before 2026-10-02, the first exchange rate in the store, are listed in USD, not converted to CAD",
		"3 series with a charge dated before 2026-10-02, the first exchange rate in the store, are listed in USD, not converted to CAD",
		"USD balances on 2 month ends before 2026-10-02, " + rateTail,
	}, got)
}

func Test_SummaryWarnings_counts_the_month_ends_before_the_first_rate(t *testing.T) {
	cases := []struct {
		name         string
		first        time.Time
		septemberEnd store.NetWorthRow
		want         string
	}{
		{
			name: "first rate between the two month ends", first: rateDay(time.September, 15), septemberEnd: convertedRow("chequing", 80_000),
			want: "USD balances on 1 month end before 2026-09-15, " + rateTail,
		},
		{
			name: "first rate after both month ends", first: rateDay(time.October, 2), septemberEnd: usdRow("chequing", 80_000),
			want: "USD balances on 2 month ends before 2026-10-02, " + rateTail,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := calmSummary()
			s.NetWorth = rateHistory(money.CAD, c.first,
				report.NetWorthDate{Date: rateDay(time.August, 31), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}},
				report.NetWorthDate{Date: rateDay(time.September, 30), Rows: []store.NetWorthRow{c.septemberEnd}})

			assert.Equal(t, []string{c.want}, document.SummaryWarnings(s, againPhrase, document.NativeFlag))
		})
	}
}

func Test_SummaryWarnings_advises_the_surfaces_own_way_to_list_balances_natively(t *testing.T) {
	cases := []struct {
		name   string
		first  time.Time
		advice document.NativeAdvice
		want   string
	}{
		{name: "flag, before the first rate", first: rateDay(time.October, 2), advice: document.NativeFlag, want: "USD balances on 2 month ends before 2026-10-02, " + rateTail},
		{name: "parameter, before the first rate", first: rateDay(time.October, 2), advice: document.NativeParameter, want: "USD balances on 2 month ends before 2026-10-02, " + rateTailParameter},
		{name: "flag, no rates", advice: document.NativeFlag, want: noRatesLine},
		{name: "parameter, no rates", advice: document.NativeParameter, want: noRatesLineParameter},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := calmSummary()
			s.NetWorth = rateHistory(money.CAD, c.first,
				report.NetWorthDate{Date: rateDay(time.August, 31), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}},
				report.NetWorthDate{Date: rateDay(time.September, 30), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}})

			assert.Equal(t, []string{c.want}, document.SummaryWarnings(s, againPhrase, c.advice))
		})
	}
}

func Test_SummaryWarnings_swaps_the_currencies_in_a_usd_summary(t *testing.T) {
	first := rateDay(time.October, 2)
	s := calmSummary()
	s.Anomalies = report.Anomalies{Currency: money.USD, Checked: 3, Unconverted: store.Unconverted{Transactions: 1, FirstRate: first}}
	s.Recurring = report.Recurring{Currency: money.USD, Series: []report.Series{{}}, Unconverted: store.Unconverted{Transactions: 1, FirstRate: first}}
	s.NetWorth = rateHistory(money.USD, first,
		report.NetWorthDate{Date: rateDay(time.August, 31), Rows: []store.NetWorthRow{cadRow("chequing", 80_000)}})

	got := document.SummaryWarnings(s, againPhrase, document.NativeFlag)

	assert.Equal(t, []string{
		"1 charge dated before 2026-10-02, the first exchange rate in the store, is listed in CAD, not converted to USD",
		"1 series with a charge dated before 2026-10-02, the first exchange rate in the store, is listed in CAD, not converted to USD",
		"CAD balances on 1 month end before 2026-10-02, " + rateTailUSD,
	}, got)
}

func Test_SummaryWarnings_has_no_rate_line_in_a_native_summary(t *testing.T) {
	s := calmSummary()
	s.Anomalies.Currency = money.Native
	s.Recurring.Currency = money.Native
	s.NetWorth = rateHistory(money.Native, time.Time{},
		report.NetWorthDate{Date: rateDay(time.August, 31), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}})

	got := document.SummaryWarnings(s, againPhrase, document.NativeFlag)

	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func Test_SummaryWarnings_says_nothing_about_rates_when_none_is_needed(t *testing.T) {
	cases := []struct {
		name    string
		summary report.Summary
	}{
		{name: "every section converts", summary: calmSummary()},
		{name: "a first rate with no charge or series before it", summary: func() report.Summary {
			s := calmSummary()
			s.Anomalies.Unconverted = store.Unconverted{FirstRate: rateDay(time.October, 2)}
			s.Recurring.Unconverted = store.Unconverted{FirstRate: rateDay(time.October, 2)}
			return s
		}()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := document.SummaryWarnings(c.summary, againPhrase, document.NativeFlag)

			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	}
}

func Test_SummaryWarnings_leaves_out_the_empty_window_and_left_out_lines_of_the_standalone_reports(t *testing.T) {
	leftOut := []store.Account{{ID: "a-1", Name: "Old card", NotInReports: true}}
	cases := []struct {
		name  string
		shape func(*report.Summary)
	}{
		{name: "no charge checked in the month", shape: func(s *report.Summary) { s.Anomalies.Checked = 0 }},
		{name: "no series listed", shape: func(s *report.Summary) { s.Recurring.Series = nil }},
		{name: "no balance on either month end", shape: func(s *report.Summary) {
			s.NetWorth.Dates = []report.NetWorthDate{{Date: rateDay(time.August, 31)}, {Date: rateDay(time.September, 30)}}
		}},
		{name: "an account named and left out of reports", shape: func(s *report.Summary) {
			s.Anomalies.Accounts = leftOut
			s.Recurring.Accounts = leftOut
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := calmSummary()
			c.shape(&s)

			got := document.SummaryWarnings(s, againPhrase, document.NativeFlag)

			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	}
}
