package report_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryNow is the clock the summary tests read: 2026-10-06 noon UTC, after September 2026 ended.
var summaryNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func Test_summary_holds_the_months_anomalies_new_recurring_and_net_worth_change(t *testing.T) {
	hardware := paidTo("payee-hw", "Hardware")
	charges := slices.Concat(
		earlierCharges(t, 3, 6000, hardware),
		[]store.Charge{chargeOn(t, 0, "2026-09-14", hardware, ofAmount(20000))},
		monthlyEndingOn(t, "2026-09-03", 3, paidTo("payee-tv", "Streaming"), ofAmount(2259)),
	)
	slices.SortStableFunc(charges, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	august, september := day(2026, time.August, 31), day(2026, time.September, 30)
	chequing := func(date time.Time, cents int64) store.NetWorthRow {
		row := typedRow("chequing", "CAD", cents, big.NewInt(cents))
		row.Date = date
		return row
	}
	srv := report.NewServer(report.WithStore(fakeStore{
		charges:  store.Charges{Rows: charges},
		netWorth: store.NetWorth{Rows: []store.NetWorthRow{chequing(august, 100000), chequing(september, 150000)}},
	}))
	month, err := report.ParseMonth(new("2026-09"), summaryNow)
	require.NoError(t, err)

	got, err := srv.Summary(t.Context(), report.SummaryRequest{Month: month, Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, []int64{20000}, amountsOf(got.Anomalies.Listed))
	assert.Equal(t, []string{"Streaming"}, payeesOf(got.Recurring))
	require.Len(t, got.NetWorth.Dates, 2)
	assert.Equal(t, []time.Time{august, september}, []time.Time{got.NetWorth.Dates[0].Date, got.NetWorth.Dates[1].Date})
	require.NotNil(t, got.Change)
	assert.Equal(t, []report.NetWorthChangeType{{Type: "chequing", Currency: "CAD", Value: big.NewInt(50000)}}, got.Change.Types)
	assert.Equal(t, []report.NetWorthChangeTotal{{Currency: "CAD", Value: big.NewInt(50000)}}, got.Change.Totals)
}

const noRate = "no rate"

var (
	endOfAugust    = day(2026, time.August, 31)
	endOfSeptember = day(2026, time.September, 30)
)

// heldOn is a net worth row of accountType on date; converted is its balance in CAD, nil when no rate converts it.
func heldOn(date time.Time, accountType, currency string, cents int64, converted *big.Int) store.NetWorthRow {
	row := typedRow(accountType, currency, cents, converted)
	row.Date = date
	return row
}

// cadHeld is a CAD row, which converts to itself.
func cadHeld(date time.Time, accountType string, cents int64) store.NetWorthRow {
	return heldOn(date, accountType, "CAD", cents, big.NewInt(cents))
}

// unrated is a USD row no exchange rate converts.
func unrated(date time.Time, accountType string, cents int64) store.NetWorthRow {
	return heldOn(date, accountType, "USD", cents, nil)
}

func changeOf(t *testing.T, currency money.Currency, rows ...store.NetWorthRow) *report.NetWorthChange {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{netWorth: store.NetWorth{Rows: rows}}))
	request := septemberRequest(t)
	request.Currency = currency

	got, err := srv.Summary(t.Context(), request)

	require.NoError(t, err)
	return got.Change
}

// typeChanges renders each type entry as "<currency> <type> <value>", the value "no rate" when nil.
func typeChanges(change *report.NetWorthChange) []string {
	lines := make([]string, 0, len(change.Types))
	for _, entry := range change.Types {
		lines = append(lines, fmt.Sprintf("%s %s %s", entry.Currency, entry.Type, valueText(entry.Value)))
	}
	return lines
}

// totalChanges renders each total as "<currency> <value>", the value "no rate" when nil.
func totalChanges(change *report.NetWorthChange) []string {
	lines := make([]string, 0, len(change.Totals))
	for _, entry := range change.Totals {
		lines = append(lines, fmt.Sprintf("%s %s", entry.Currency, valueText(entry.Value)))
	}
	return lines
}

func valueText(value *big.Int) string {
	if value == nil {
		return noRate
	}
	return value.String()
}

func Test_change_counts_a_type_missing_on_the_start_day_as_zero(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000),
		cadHeld(endOfSeptember, "chequing", 150000), cadHeld(endOfSeptember, "brokerage", 500000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD brokerage 500000", "CAD chequing 50000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD 550000"}, totalChanges(change))
}

func Test_change_has_one_total_in_the_reporting_currency_when_every_balance_converts(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000), heldOn(endOfAugust, "savings", "USD", 10000, big.NewInt(13000)),
		cadHeld(endOfSeptember, "chequing", 90000), heldOn(endOfSeptember, "savings", "USD", 10000, big.NewInt(13500)))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing -10000", "CAD savings 500"}, typeChanges(change))
	assert.Equal(t, []string{"CAD -9500"}, totalChanges(change))
}

func Test_change_in_USD_uses_the_USD_balances(t *testing.T) {
	row := func(date time.Time, cad, usd int64) store.NetWorthRow {
		held := heldOn(date, "chequing", "CAD", cad, big.NewInt(cad))
		held.BalanceUSD = big.NewInt(usd)
		return held
	}

	change := changeOf(t, money.USD, row(endOfAugust, 100000, 75000), row(endOfSeptember, 100000, 80000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"USD chequing 5000"}, typeChanges(change))
	assert.Equal(t, []string{"USD 5000"}, totalChanges(change))
}

func Test_change_of_a_type_is_no_rate_when_only_the_start_day_needs_a_rate(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000), unrated(endOfAugust, "savings", 10000),
		cadHeld(endOfSeptember, "chequing", 150000), heldOn(endOfSeptember, "savings", "USD", 10000, big.NewInt(13000)))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing 50000", "CAD savings " + noRate}, typeChanges(change))
}

func Test_change_of_a_type_is_no_rate_when_only_the_end_day_needs_a_rate(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000), heldOn(endOfAugust, "savings", "USD", 10000, big.NewInt(13000)),
		cadHeld(endOfSeptember, "chequing", 150000), unrated(endOfSeptember, "savings", 10000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing 50000", "CAD savings " + noRate}, typeChanges(change))
}

func Test_change_of_a_type_is_no_rate_when_one_of_its_rows_converts_and_another_needs_a_rate(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "brokerage", 100000), unrated(endOfAugust, "brokerage", 10000),
		cadHeld(endOfSeptember, "brokerage", 150000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD brokerage " + noRate}, typeChanges(change))
}

func Test_change_total_is_no_rate_when_a_day_holds_a_total_in_another_currency(t *testing.T) {
	change := changeOf(t, money.CAD,
		cadHeld(endOfAugust, "chequing", 100000), unrated(endOfAugust, "savings", 10000),
		cadHeld(endOfSeptember, "chequing", 150000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD " + noRate}, totalChanges(change))
}

func Test_change_total_is_no_rate_when_a_day_has_no_total_in_the_reporting_currency(t *testing.T) {
	change := changeOf(t, money.CAD,
		unrated(endOfAugust, "savings", 10000),
		cadHeld(endOfSeptember, "chequing", 150000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD " + noRate}, totalChanges(change))
}

func Test_change_total_counts_a_start_day_of_unrated_zero_balances_as_zero(t *testing.T) {
	change := changeOf(t, money.CAD,
		unrated(endOfAugust, "savings", 0),
		cadHeld(endOfSeptember, "chequing", 150000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing 150000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD 150000"}, totalChanges(change))
}

func Test_change_counts_an_empty_end_day_as_zero(t *testing.T) {
	change := changeOf(t, money.CAD, cadHeld(endOfAugust, "chequing", 100000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing -100000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD -100000"}, totalChanges(change))
}

func Test_native_change_counts_an_empty_end_day_as_zero(t *testing.T) {
	change := changeOf(t, money.Native, cadHeld(endOfAugust, "chequing", 100000))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing -100000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD -100000"}, totalChanges(change))
}

func Test_change_is_absent_when_the_start_day_has_no_balance(t *testing.T) {
	change := changeOf(t, money.CAD, cadHeld(endOfSeptember, "chequing", 150000))

	assert.Nil(t, change)
}

func Test_native_change_counts_a_currency_on_the_end_day_only_as_zero_on_the_start_day(t *testing.T) {
	change := changeOf(t, money.Native,
		cadHeld(endOfAugust, "chequing", 100000),
		cadHeld(endOfSeptember, "chequing", 100000), heldOn(endOfSeptember, "chequing", "USD", 150000, nil))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD chequing 0", "USD chequing 150000"}, typeChanges(change))
	assert.Equal(t, []string{"CAD 0", "USD 150000"}, totalChanges(change))
}

func Test_native_change_lists_a_type_only_for_the_currencies_that_hold_it(t *testing.T) {
	change := changeOf(t, money.Native,
		cadHeld(endOfAugust, "chequing", 100000), cadHeld(endOfAugust, "brokerage", 200000),
		cadHeld(endOfSeptember, "chequing", 110000), heldOn(endOfSeptember, "chequing", "USD", 5000, nil))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD brokerage -200000", "CAD chequing 10000", "USD chequing 5000"}, typeChanges(change))
}

func Test_native_change_orders_currencies_CAD_then_USD_then_alphabetically(t *testing.T) {
	change := changeOf(t, money.Native,
		heldOn(endOfAugust, "chequing", "GBP", 100, nil), heldOn(endOfAugust, "chequing", "USD", 200, nil),
		heldOn(endOfAugust, "chequing", "CAD", 300, nil), heldOn(endOfAugust, "chequing", "EUR", 400, nil))

	require.NotNil(t, change)
	assert.Equal(t, []string{"CAD -300", "USD -200", "EUR -400", "GBP -100"}, totalChanges(change))
	assert.Equal(t, []string{"CAD chequing -300", "USD chequing -200", "EUR chequing -400", "GBP chequing -100"}, typeChanges(change))
}

func Test_native_change_is_shown_when_only_a_USD_balance_is_on_the_start_day(t *testing.T) {
	change := changeOf(t, money.Native,
		heldOn(endOfAugust, "chequing", "USD", 100000, nil),
		heldOn(endOfSeptember, "chequing", "USD", 120000, nil))

	require.NotNil(t, change)
	assert.Equal(t, []string{"USD chequing 20000"}, typeChanges(change))
	assert.Equal(t, []string{"USD 20000"}, totalChanges(change))
}

func Test_native_change_is_absent_when_the_start_day_has_no_balance(t *testing.T) {
	change := changeOf(t, money.Native, cadHeld(endOfSeptember, "chequing", 150000))

	assert.Nil(t, change)
}

func Test_change_is_absent_for_a_listing_without_dates(t *testing.T) {
	assert.Nil(t, report.NetWorth{}.Change())
}

func coverageOf(t *testing.T, month report.Month, taken time.Time) report.SnapshotCoverage {
	t.Helper()
	status := store.Status{Run: store.ImportRun{Snapshot: store.SnapshotRef{TakenAt: taken}}}
	srv := report.NewServer(report.WithStore(fakeStore{status: status}))

	got, err := srv.Summary(t.Context(), report.SummaryRequest{Month: month, Currency: money.CAD})

	require.NoError(t, err)
	return got.Coverage
}

func Test_a_snapshot_covers_a_month_only_when_taken_at_or_after_local_midnight_ending_it(t *testing.T) {
	toronto, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)
	cases := []struct {
		name  string
		zone  *time.Location
		month string
		taken time.Time
		want  report.SnapshotCoverage
	}{
		{name: "UTC: exactly midnight", zone: time.UTC, month: "2026-09", taken: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "UTC: one nanosecond before midnight", zone: time.UTC, month: "2026-09", taken: time.Date(2026, 9, 30, 23, 59, 59, 999999999, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "fixed EDT: exactly local midnight", zone: edt, month: "2026-09", taken: time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "fixed EDT: one nanosecond before local midnight", zone: edt, month: "2026-09", taken: time.Date(2026, 10, 1, 3, 59, 59, 999999999, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "fixed EDT: past UTC midnight but before local midnight", zone: edt, month: "2026-09", taken: time.Date(2026, 10, 1, 0, 30, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "Toronto: October ends at 04:00Z, still on daylight time", zone: toronto, month: "2026-10", taken: time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "Toronto: one nanosecond before October's end", zone: toronto, month: "2026-10", taken: time.Date(2026, 11, 1, 3, 59, 59, 999999999, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "Toronto: March ends at 04:00Z after the clocks went forward", zone: toronto, month: "2026-03", taken: time.Date(2026, 4, 1, 4, 30, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "Toronto: December ends at 05:00Z on standard time", zone: toronto, month: "2026-12", taken: time.Date(2027, 1, 1, 4, 30, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "Toronto: exactly the year's end", zone: toronto, month: "2026-12", taken: time.Date(2027, 1, 1, 5, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "a leap February ends after the 29th", zone: time.UTC, month: "2024-02", taken: time.Date(2024, 2, 29, 23, 59, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "a leap February's end", zone: time.UTC, month: "2024-02", taken: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "taken long after the month", zone: time.UTC, month: "2026-09", taken: time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "taken before the month began", zone: time.UTC, month: "2026-09", taken: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "no time recorded", zone: time.UTC, month: "2026-09", taken: time.Time{}, want: report.SnapshotTimeUnknown},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			month, err := report.ParseMonth(&c.month, time.Date(2028, 1, 1, 12, 0, 0, 0, c.zone))
			require.NoError(t, err)

			assert.Equal(t, c.want, coverageOf(t, month, c.taken))
		})
	}
}

func Test_a_snapshot_taken_after_midnight_on_the_first_covers_the_default_month(t *testing.T) {
	cases := []struct {
		name  string
		taken time.Time
		want  report.SnapshotCoverage
	}{
		{name: "one minute into the first", taken: time.Date(2026, 10, 1, 0, 1, 0, 0, edt), want: report.SnapshotCovers},
		{name: "the minute before midnight", taken: time.Date(2026, 9, 30, 23, 59, 0, 0, edt), want: report.SnapshotPredatesMonthEnd},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			month, err := report.ParseMonth(nil, time.Date(2026, 10, 1, 0, 5, 0, 0, edt))
			require.NoError(t, err)

			assert.Equal(t, c.want, coverageOf(t, month, c.taken))
		})
	}
}

func Test_a_month_without_a_zone_ends_at_UTC_midnight(t *testing.T) {
	cases := []struct {
		name  string
		taken time.Time
		want  report.SnapshotCoverage
	}{
		{name: "UTC midnight", taken: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "a minute before", taken: time.Date(2026, 9, 30, 23, 59, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			month := report.Month{Start: day(2026, time.September, 1), End: day(2026, time.September, 30)}

			assert.Equal(t, c.want, coverageOf(t, month, c.taken))
		})
	}
}

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
