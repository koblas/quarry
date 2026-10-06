package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

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
