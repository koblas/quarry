package report_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// septemberHardwareCharges is three earlier charges and a September one of 200.00 that is unusually large.
func septemberHardwareCharges(t *testing.T) []store.Charge {
	t.Helper()
	payee := paidTo("payee-hw", "Hardware")
	return append(earlierCharges(t, 3, 6000, payee), chargeOn(t, 0, "2026-09-14", payee, ofAmount(20000)))
}

func chequingOn(date time.Time, cents int64) store.NetWorthRow {
	row := typedRow("chequing", "CAD", cents, big.NewInt(cents))
	row.Date = date
	return row
}

func septemberRequest(t *testing.T) report.SummaryRequest {
	t.Helper()
	month, err := report.ParseMonth(new("2026-09"), summaryNow)
	require.NoError(t, err)
	return report.SummaryRequest{Month: month, Currency: money.CAD}
}

func Test_summary_reads_the_store_once(t *testing.T) {
	read := store.Summary{
		Status:   store.Status{QuarryVersion: "from the summary read"},
		Charges:  store.Charges{Rows: septemberHardwareCharges(t)},
		NetWorth: store.NetWorth{Rows: []store.NetWorthRow{chequingOn(day(2026, time.September, 30), 150000)}},
	}
	var summaryReads, chargesReads, netWorthReads int
	srv := report.NewServer(report.WithStore(fakeStore{
		summary:       &read,
		status:        store.Status{QuarryVersion: "from a second read"},
		charges:       store.Charges{},
		netWorth:      store.NetWorth{Rows: []store.NetWorthRow{chequingOn(day(2026, time.September, 30), 1)}},
		summaryReads:  &summaryReads,
		chargesReads:  &chargesReads,
		netWorthReads: &netWorthReads,
	}))

	got, err := srv.Summary(t.Context(), septemberRequest(t))

	require.NoError(t, err)
	assert.Equal(t, "from the summary read", got.Status.QuarryVersion)
	assert.Equal(t, []int64{20000}, amountsOf(got.Anomalies.Listed))
	require.Len(t, got.NetWorth.Dates[1].Rows, 1)
	assert.Equal(t, big.NewInt(150000), got.NetWorth.Dates[1].Rows[0].Balance)
	assert.Equal(t, [3]int{1, 0, 0}, [3]int{summaryReads, chargesReads, netWorthReads})
}

func Test_summary_reads_charges_through_the_month_end_and_net_worth_at_both_month_ends(t *testing.T) {
	var got store.SummaryParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSummary: &got}))

	_, err := srv.Summary(t.Context(), septemberRequest(t))

	require.NoError(t, err)
	assert.Equal(t, store.SummaryParams{
		Through: day(2026, time.September, 30),
		Dates:   []time.Time{day(2026, time.August, 31), day(2026, time.September, 30)},
	}, got)
}

func Test_summary_reads_the_month_before_a_january_across_the_year_end(t *testing.T) {
	cases := []struct {
		name  string
		month string
		dates []time.Time
	}{
		{name: "a january in the current era", month: "2026-01", dates: []time.Time{day(2025, time.December, 31), day(2026, time.January, 31)}},
		{name: "the earliest month", month: "0001-01", dates: []time.Time{day(0, time.December, 31), day(1, time.January, 31)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got store.SummaryParams
			srv := report.NewServer(report.WithStore(fakeStore{gotSummary: &got}))
			month, err := report.ParseMonth(&c.month, summaryNow)
			require.NoError(t, err)

			_, err = srv.Summary(t.Context(), report.SummaryRequest{Month: month, Currency: money.CAD})

			require.NoError(t, err)
			assert.Equal(t, c.dates, got.Dates)
		})
	}
}

func Test_summary_returns_the_month_and_currency_it_was_asked_for(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{}))
	req := septemberRequest(t)
	req.Currency = money.USD

	got, err := srv.Summary(t.Context(), req)

	require.NoError(t, err)
	assert.Equal(t, req.Month, got.Month)
	assert.Equal(t, money.USD, got.Currency)
	assert.Equal(t, money.USD, got.Anomalies.Currency)
	assert.Equal(t, money.USD, got.Recurring.Currency)
	assert.Equal(t, money.USD, got.NetWorth.Currency)
}

func Test_summary_lists_recurring_series_over_the_month_it_summarizes(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{}))
	req := septemberRequest(t)

	got, err := srv.Summary(t.Context(), req)

	require.NoError(t, err)
	assert.Equal(t, req.Month.Window(), got.Recurring.Window)
}

func Test_summary_anomalies_equal_the_anomalies_read_of_the_month(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{
		charges: store.Charges{Rows: append(septemberHardwareCharges(t), chargeOn(t, 1, "2026-10-02", paidTo("payee-hw", "Hardware"), ofAmount(30000))), FirstRate: day(2020, time.January, 2)},
	}))
	req := septemberRequest(t)

	got, err := srv.Summary(t.Context(), req)
	want, wantErr := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: req.Month.Window(), Now: summaryNow, Currency: money.CAD})

	require.NoError(t, err)
	require.NoError(t, wantErr)
	require.Equal(t, []int64{20000}, amountsOf(want.Listed))
	assert.Equal(t, want, got.Anomalies)
}

func Test_summary_net_worth_equals_the_month_end_read(t *testing.T) {
	rows := []store.NetWorthRow{
		chequingOn(day(2026, time.August, 31), 100000),
		chequingOn(day(2026, time.September, 30), 150000),
		typedRow("brokerage", "USD", 9000, nil),
	}
	rows[2].Date = day(2026, time.September, 30)
	srv := report.NewServer(report.WithStore(fakeStore{netWorth: store.NetWorth{Rows: rows, FirstRate: day(2020, time.January, 2)}}))
	req := septemberRequest(t)
	window, err := report.ParseMonthEndWindow(new("2026-08"), new("2026-09"), summaryNow)
	require.NoError(t, err)

	got, err := srv.Summary(t.Context(), req)
	want, wantErr := srv.NetWorth(t.Context(), report.NetWorthRequest{AsOf: req.Month.End, Window: &window, Currency: money.CAD})

	require.NoError(t, err)
	require.NoError(t, wantErr)
	assert.Equal(t, want, got.NetWorth)
}

func Test_summary_refuses_a_store_that_cannot_be_opened(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Summary(t.Context(), septemberRequest(t))

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it", refusal.Error())
}

func Test_summary_passes_on_a_read_fault_that_is_not_a_refusal(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Summary(t.Context(), septemberRequest(t))

	require.ErrorIs(t, err, errDiskRead)
	_, refused := errors.AsType[report.RefusalError](err)
	assert.False(t, refused)
}

func Test_summary_reports_an_interrupt_during_the_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Summary(ctx, septemberRequest(t))

	assert.EqualError(t, err, "summary interrupted")
}
