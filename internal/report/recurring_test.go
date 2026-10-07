package report_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recurringNow is the clock every recurring test reads: 2026-09-29 noon UTC.
var recurringNow = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// chargeOpt changes a charge chargeOn built.
type chargeOpt func(*store.Charge)

func paidTo(id, name string) chargeOpt {
	return func(c *store.Charge) { c.PayeeID, c.Payee = &id, &name }
}

func unpaid() chargeOpt {
	return func(c *store.Charge) { c.PayeeID, c.Payee = nil, nil }
}

func billedIn(currency string) chargeOpt { return func(c *store.Charge) { c.Currency = currency } }

func ofAmount(cents int64) chargeOpt { return func(c *store.Charge) { c.Amount = cents } }

func onAccount(id, name string) chargeOpt {
	return func(c *store.Charge) {
		c.Account = store.Account{ID: id, Name: name, Currency: c.Currency, Active: true}
	}
}

func inCategory(id, path string) chargeOpt {
	return func(c *store.Charge) { c.Category = &store.ChargeCategory{ID: id, Path: path} }
}

// chargeOn is a 12.00 CAD charge by Gym on date (YYYY-MM-DD), numbered sourceID, changed by opts.
func chargeOn(t *testing.T, sourceID int64, date string, opts ...chargeOpt) store.Charge {
	t.Helper()
	day, err := time.Parse(time.DateOnly, date)
	require.NoError(t, err)
	c := store.Charge{
		SourceID: sourceID,
		Date:     day,
		Account:  store.Account{ID: "acct-cad", Name: "Chequing", Currency: "CAD", Active: true},
		PayeeID:  new("payee-gym"),
		Payee:    new("Gym"),
		Currency: "CAD",
		Amount:   1200,
	}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// everyDays is count dates gapDays apart from first (YYYY-MM-DD).
func everyDays(t *testing.T, first string, gapDays, count int) []string {
	t.Helper()
	start, err := time.Parse(time.DateOnly, first)
	require.NoError(t, err)
	dates := make([]string, count)
	for i := range dates {
		dates[i] = start.AddDate(0, 0, gapDays*i).Format(time.DateOnly)
	}
	return dates
}

// chargesOn is a charge on each date, built with opts.
func chargesOn(t *testing.T, dates []string, opts ...chargeOpt) []store.Charge {
	t.Helper()
	charges := make([]store.Charge, len(dates))
	for i, date := range dates {
		charges[i] = chargeOn(t, 0, date, opts...)
	}
	return charges
}

// allTime is a window from 2000 through recurringNow's day.
var allTime = store.Window{
	Since: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
	Until: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
}

// recurringOf reads the series of every charge in groups over allTime.
func recurringOf(t *testing.T, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	return recurringIn(t, allTime, groups...)
}

// recurringIn reads the series of every charge in groups, merged oldest first and numbered in that order,
// listing those that ran in window.
func recurringIn(t *testing.T, window store.Window, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	return recurringAt(t, recurringNow, window, groups...)
}

// recurringAt is recurringIn read at now.
func recurringAt(t *testing.T, now time.Time, window store.Window, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	return recurringRead(t, report.RecurringRequest{Window: window, Now: now}, time.Time{}, groups...)
}

// recurringRead answers req from the merged charges of groups (oldest first, numbered in that order)
// on a store whose first exchange rate is dated firstRate.
func recurringRead(t *testing.T, req report.RecurringRequest, firstRate time.Time, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	merged := slices.Concat(groups...)
	slices.SortStableFunc(merged, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	for i := range merged {
		merged[i].SourceID = int64(i + 1)
	}
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Rows: merged, FirstRate: firstRate}}))

	result, err := srv.Recurring(t.Context(), req)

	require.NoError(t, err)
	return result
}

// monthlyEndingOn is count charges 30 days apart, the last on date (YYYY-MM-DD), built with opts.
func monthlyEndingOn(t *testing.T, date string, count int, opts ...chargeOpt) []store.Charge {
	t.Helper()
	return everyDaysEndingOn(t, date, 30, count, opts...)
}

// everyDaysEndingOn is count charges gapDays apart, the last on date (YYYY-MM-DD), built with opts.
func everyDaysEndingOn(t *testing.T, date string, gapDays, count int, opts ...chargeOpt) []store.Charge {
	t.Helper()
	last, err := time.Parse(time.DateOnly, date)
	require.NoError(t, err)
	first := last.AddDate(0, 0, -gapDays*(count-1))
	return chargesOn(t, everyDays(t, first.Format(time.DateOnly), gapDays, count), opts...)
}

// payeesOf lists the payee of each series in result, in order.
func payeesOf(result report.Recurring) []string {
	payees := make([]string, len(result.Series))
	for i, s := range result.Series {
		payees[i] = s.Payee
	}
	return payees
}

func Test_recurring_starts_the_series_again_after_a_charge_off_schedule(t *testing.T) {
	charges := []store.Charge{
		chargeOn(t, 1, "2026-03-05"),
		chargeOn(t, 2, "2026-04-05"),
		chargeOn(t, 3, "2026-05-05"),
		chargeOn(t, 4, "2026-07-14"),
		chargeOn(t, 5, "2026-08-14"),
		chargeOn(t, 6, "2026-09-14"),
	}

	result := recurringOf(t, charges)

	require.Len(t, result.Series, 1)
	assert.Equal(t, time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC), result.Series[0].First)
	assert.Equal(t, 3, result.Series[0].ChargeCount)
}

func dateOf(t *testing.T, date string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, date)
	require.NoError(t, err)
	return parsed
}

func Test_recurring_reads_charges_through_today_even_when_the_window_ends_later(t *testing.T) {
	var got store.ChargeParams
	srv := report.NewServer(report.WithStore(fakeStore{gotCharges: &got}))
	window := store.Window{Since: allTime.Since, Until: dateOf(t, "2030-12-31")}

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: window, Now: recurringNow})

	require.NoError(t, err)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29")}, got)
}

func Test_recurring_reads_charges_through_the_local_date_when_the_utc_date_is_later(t *testing.T) {
	var got store.ChargeParams
	srv := report.NewServer(report.WithStore(fakeStore{gotCharges: &got}))

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: windowNow})

	require.NoError(t, err)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29")}, got)
}

func Test_recurring_reads_the_charges_once(t *testing.T) {
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{chargesReads: &reads}))

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: recurringNow})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
}

func Test_recurring_returns_the_window_it_listed(t *testing.T) {
	window := store.Window{Since: dateOf(t, "2026-03-01"), Until: dateOf(t, "2026-03-31")}

	result := recurringIn(t, window)

	assert.Equal(t, window, result.Window)
}

func Test_recurring_lists_an_ended_series_only_when_the_window_touches_its_first_or_last_charge(t *testing.T) {
	cases := []struct {
		name         string
		since, until string
		want         int
	}{
		{name: "last charge on the first day of the window", since: "2026-01-15", until: "2026-12-31", want: 1},
		{name: "last charge the day before the window", since: "2026-01-16", until: "2026-12-31", want: 0},
		{name: "first charge on the last day of the window", since: "2000-01-01", until: "2025-10-17", want: 1},
		{name: "first charge the day after the window", since: "2000-01-01", until: "2025-10-16", want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ended := monthlyEndingOn(t, "2026-01-15", 4)
			window := store.Window{Since: dateOf(t, c.since), Until: dateOf(t, c.until)}

			result := recurringIn(t, window, ended)

			assert.Len(t, result.Series, c.want)
		})
	}
}

func Test_recurring_lists_an_active_series_only_when_the_window_starts_by_today(t *testing.T) {
	cases := []struct {
		name  string
		since string
		want  int
	}{
		{name: "window starting today", since: "2026-09-29", want: 1},
		{name: "window starting tomorrow", since: "2026-09-30", want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			active := monthlyEndingOn(t, "2026-09-15", 3)
			window := store.Window{Since: dateOf(t, c.since), Until: dateOf(t, "2026-12-31")}

			result := recurringIn(t, window, active)

			assert.Len(t, result.Series, c.want)
		})
	}
}

func Test_recurring_lists_an_active_series_whose_last_charge_is_before_the_window(t *testing.T) {
	monthly := monthlyEndingOn(t, "2026-09-01", 3, paidTo("payee-gym", "Gym"))
	weekly := everyDaysEndingOn(t, "2026-09-01", 7, 4, paidTo("payee-paper", "Paper"))
	window := store.Window{Since: dateOf(t, "2026-09-20"), Until: dateOf(t, "2026-09-29")}

	result := recurringIn(t, window, monthly, weekly)

	assert.Equal(t, []string{"Gym"}, payeesOf(result))
}

// monthlyDates are six monthly charge dates, each 30 days after the last.
func monthlyDates(t *testing.T) []string {
	t.Helper()
	return everyDays(t, "2026-03-01", 30, 6)
}

func currenciesOf(result report.Recurring) []string {
	currencies := make([]string, len(result.Series))
	for i, s := range result.Series {
		currencies[i] = s.Currency
	}
	return currencies
}

func Test_recurring_merges_payees_whose_names_differ_only_in_store_numbers(t *testing.T) {
	dates := monthlyDates(t)
	numbered := chargesOn(t, []string{dates[0], dates[2], dates[4]}, paidTo("payee-12", "NETFLIX.COM 1234"))
	plain := chargesOn(t, []string{dates[1], dates[3], dates[5]}, paidTo("payee-13", "Netflix.com"))

	result := recurringOf(t, numbered, plain)

	require.Len(t, result.Series, 1)
	assert.Equal(t, 6, result.Series[0].ChargeCount)
	assert.Equal(t, "Netflix.com", result.Series[0].Payee)
}

func Test_recurring_keeps_one_payee_in_two_currencies_as_two_series(t *testing.T) {
	cad := chargesOn(t, monthlyDates(t), billedIn("CAD"))
	usd := chargesOn(t, monthlyDates(t), billedIn("USD"))

	result := recurringOf(t, cad, usd)

	assert.ElementsMatch(t, []string{"CAD", "USD"}, currenciesOf(result))
}

func Test_recurring_keeps_a_series_whole_when_it_moves_between_accounts(t *testing.T) {
	dates := monthlyDates(t)
	chequing := chargesOn(t, dates[:3], onAccount("acct-chq", "Chequing"))
	visa := chargesOn(t, dates[3:], onAccount("acct-visa", "Visa"))

	result := recurringOf(t, chequing, visa)

	require.Len(t, result.Series, 1)
	assert.Equal(t, 6, result.Series[0].ChargeCount)
}

func Test_recurring_keeps_a_series_whole_when_its_category_changes(t *testing.T) {
	dates := monthlyDates(t)
	before := chargesOn(t, dates[:3], inCategory("cat-fun", "Fun"))
	after := chargesOn(t, dates[3:], inCategory("cat-health", "Health"))

	result := recurringOf(t, before, after)

	require.Len(t, result.Series, 1)
	assert.Equal(t, 6, result.Series[0].ChargeCount)
}

func Test_recurring_never_makes_a_series_of_charges_with_no_payee(t *testing.T) {
	result := recurringOf(t, chargesOn(t, monthlyDates(t), unpaid()))

	assert.Empty(t, result.Series)
}

func Test_recurring_never_makes_a_series_of_charges_with_a_payee_id_but_no_payee_name(t *testing.T) {
	nameless := func(c *store.Charge) { c.PayeeID, c.Payee = new("payee-gym"), nil }

	result := recurringOf(t, chargesOn(t, monthlyDates(t), nameless))

	assert.Empty(t, result.Series)
}

func Test_recurring_groups_a_payee_with_no_key_by_its_id(t *testing.T) {
	dates := monthlyDates(t)
	first := chargesOn(t, dates[:3], paidTo("payee-77", "#4411"))
	second := chargesOn(t, dates[3:], paidTo("payee-77", "#9902"))

	result := recurringOf(t, first, second)

	require.Len(t, result.Series, 1)
	assert.Equal(t, 6, result.Series[0].ChargeCount)
}

func Test_recurring_keeps_two_payees_with_no_key_apart(t *testing.T) {
	first := chargesOn(t, monthlyDates(t), paidTo("payee-77", "#4411"))
	second := chargesOn(t, monthlyDates(t), paidTo("payee-78", "#9902"))

	result := recurringOf(t, first, second)

	assert.Len(t, result.Series, 2)
}

func Test_recurring_keeps_a_payee_id_fallback_apart_from_an_equal_payee_key(t *testing.T) {
	keyed := chargesOn(t, monthlyDates(t), paidTo("payee-1", "Gym"))
	fallback := chargesOn(t, monthlyDates(t), paidTo("gym", "#4411"))

	result := recurringOf(t, keyed, fallback)

	assert.Len(t, result.Series, 2)
}

func Test_recurring_refuses_when_the_charges_read_fails_to_open_the_store(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Now: recurringNow})

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it", refusal.Error())
}

func Test_recurring_reports_an_interrupt_during_the_charges_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Recurring(ctx, report.RecurringRequest{Now: recurringNow})

	assert.EqualError(t, err, "recurring interrupted")
}

func Test_recurring_lists_both_payees_of_a_renamed_payee_and_names_the_series_by_the_latest(t *testing.T) {
	dates := monthlyDates(t)
	old := chargesOn(t, dates[:3], paidTo("payee-old", "NETFLIX.COM 1234"))
	renamed := chargesOn(t, dates[3:], paidTo("payee-new", "Netflix.com"))

	result := recurringOf(t, old, renamed)

	require.Len(t, result.Series, 1)
	assert.Equal(t, "Netflix.com", result.Series[0].Payee)
	assert.Equal(t, []report.SeriesPayee{
		{ID: "payee-old", Name: "NETFLIX.COM 1234"},
		{ID: "payee-new", Name: "Netflix.com"},
	}, result.Series[0].Payees)
}

func Test_recurring_lists_one_payee_once_however_many_charges_it_has(t *testing.T) {
	result := recurringOf(t, chargesOn(t, monthlyDates(t)))

	require.Len(t, result.Series, 1)
	assert.Equal(t, []report.SeriesPayee{{ID: "payee-gym", Name: "Gym"}}, result.Series[0].Payees)
}

func Test_recurring_lists_both_accounts_when_the_card_changes_mid_run(t *testing.T) {
	dates := monthlyDates(t)
	oldCard := chargesOn(t, dates[:3], onAccount("acct-old", "Old card"))
	newCard := chargesOn(t, dates[3:], onAccount("acct-new", "New card"))

	result := recurringOf(t, oldCard, newCard)

	require.Len(t, result.Series, 1)
	assert.Equal(t, []string{"acct-old", "acct-new"}, accountIDsOf(result.Series[0]))
}

func Test_recurring_leaves_out_an_account_used_only_before_the_run_began(t *testing.T) {
	stray := chargesOn(t, []string{"2025-11-01"}, onAccount("acct-stray", "Stray"))
	run := chargesOn(t, monthlyDates(t), onAccount("acct-run", "Run"))

	result := recurringOf(t, stray, run)

	require.Len(t, result.Series, 1)
	assert.Equal(t, []string{"acct-run"}, accountIDsOf(result.Series[0]))
}

func Test_recurring_sets_the_payee_key_of_a_series_grouped_by_name(t *testing.T) {
	result := recurringOf(t, chargesOn(t, monthlyDates(t), paidTo("payee-1", "Netflix.com")))

	require.Len(t, result.Series, 1)
	assert.Equal(t, new("netflix-com"), result.Series[0].PayeeKey)
}

func Test_recurring_leaves_the_payee_key_nil_for_a_series_grouped_by_payee_id(t *testing.T) {
	result := recurringOf(t, chargesOn(t, monthlyDates(t), paidTo("payee-77", "#4411")))

	require.Len(t, result.Series, 1)
	assert.Nil(t, result.Series[0].PayeeKey)
}

func Test_recurring_gives_each_currency_of_one_name_its_own_payees(t *testing.T) {
	cad := chargesOn(t, monthlyDates(t), paidTo("payee-cad", "Netflix.com"), onAccount("acct-cad", "Chequing"))
	usd := chargesOn(t, monthlyDates(t), paidTo("payee-usd", "NETFLIX.COM 1234"), billedIn("USD"), onAccount("acct-usd", "Dollars"))

	result := recurringOf(t, cad, usd)

	require.Len(t, result.Series, 2)
	assert.Equal(t, []report.SeriesPayee{{ID: "payee-cad", Name: "Netflix.com"}}, result.Series[0].Payees)
	assert.Equal(t, []report.SeriesPayee{{ID: "payee-usd", Name: "NETFLIX.COM 1234"}}, result.Series[1].Payees)
}

func accountIDsOf(s report.Series) []string {
	ids := make([]string, len(s.Accounts))
	for i, a := range s.Accounts {
		ids[i] = a.ID
	}
	return ids
}

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

func Test_recurring_marks_a_series_active_when_a_charge_came_within_its_quiet_period(t *testing.T) {
	result := recurringOf(t, monthlyEndingOn(t, "2026-09-15", 3))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.SeriesActive, result.Series[0].State)
}

func Test_recurring_marks_a_series_ended_and_gives_it_no_yearly_cost_when_no_charge_has_come_lately(t *testing.T) {
	result := recurringOf(t, monthlyEndingOn(t, "2026-01-15", 3))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.SeriesEnded, result.Series[0].State)
	assert.Nil(t, result.Series[0].PerYear)
}

func Test_recurring_counts_days_since_the_last_charge_from_the_local_date(t *testing.T) {
	result := recurringAt(t, windowNow, allTime, monthlyEndingOn(t, "2026-08-15", 3))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.SeriesActive, result.Series[0].State)
}

func Test_recurring_takes_the_amount_and_yearly_cost_from_the_latest_charge(t *testing.T) {
	charges := []store.Charge{
		chargeOn(t, 1, "2026-07-15", ofAmount(1000)),
		chargeOn(t, 2, "2026-08-14", ofAmount(1000)),
		chargeOn(t, 3, "2026-09-13", ofAmount(1040)),
	}

	result := recurringOf(t, charges)

	require.Len(t, result.Series, 1)
	assert.Equal(t, int64(1040), result.Series[0].Amount)
	assert.Equal(t, new(int64(12480)), result.Series[0].PerYear)
}

// activeLast and endedLast are last-charge dates of monthly series: within and past the 45-day quiet period of recurringNow.
const (
	activeLast = "2026-09-15"
	endedLast  = "2026-05-01"
)

// monthlyOf is a monthly series paid to payee, the last charge on last, each of cents.
func monthlyOf(t *testing.T, payee, last string, cents int64, opts ...chargeOpt) []store.Charge {
	t.Helper()
	return monthlyEndingOn(t, last, 3, append([]chargeOpt{paidTo("payee-"+payee, payee), ofAmount(cents)}, opts...)...)
}

func Test_recurring_totals_the_yearly_cost_of_each_currencys_active_series_with_CAD_before_USD(t *testing.T) {
	usd := monthlyOf(t, "Hulu", activeLast, 1000, billedIn("USD"))
	gym := monthlyOf(t, "Gym", activeLast, 2000)
	paper := monthlyOf(t, "Paper", activeLast, 500)

	result := recurringOf(t, usd, gym, paper)

	assert.Equal(t, []report.RecurringTotal{{Currency: "CAD", PerYear: 30000}, {Currency: "USD", PerYear: 12000}}, result.Totals)
}

func Test_recurring_leaves_ended_series_out_of_the_totals(t *testing.T) {
	active := monthlyOf(t, "Gym", activeLast, 1000)
	endedCAD := monthlyOf(t, "Paper", endedLast, 5000)
	endedEUR := monthlyOf(t, "Pasta", endedLast, 700, billedIn("EUR"))

	result := recurringOf(t, active, endedCAD, endedEUR)

	assert.Equal(t, []report.RecurringTotal{{Currency: "CAD", PerYear: 12000}}, result.Totals)
}

func Test_recurring_lists_CAD_series_before_USD_ones_whatever_they_cost(t *testing.T) {
	usd := monthlyOf(t, "Aaa", activeLast, 5000, billedIn("USD"))
	cad := monthlyOf(t, "Bbb", activeLast, 1000)

	result := recurringOf(t, usd, cad)

	assert.Equal(t, []string{"Bbb", "Aaa"}, payeesOf(result))
}

func Test_recurring_lists_an_ended_CAD_series_before_an_active_USD_one(t *testing.T) {
	endedCAD := monthlyOf(t, "Aaa", endedLast, 1000)
	activeUSD := monthlyOf(t, "Bbb", activeLast, 1000, billedIn("USD"))

	result := recurringOf(t, activeUSD, endedCAD)

	assert.Equal(t, []string{"Aaa", "Bbb"}, payeesOf(result))
}

func Test_recurring_lists_active_series_before_ended_ones_even_when_the_ended_one_charged_later(t *testing.T) {
	active := monthlyOf(t, "Zed", "2026-08-20", 1000)
	ended := everyDaysEndingOn(t, "2026-09-10", 7, 4, paidTo("payee-abe", "Abe"))

	result := recurringOf(t, ended, active)

	assert.Equal(t, []string{"Zed", "Abe"}, payeesOf(result))
}

func Test_recurring_lists_the_costliest_active_series_first(t *testing.T) {
	cheap := monthlyOf(t, "Aaa", activeLast, 1000)
	dear := monthlyOf(t, "Bbb", activeLast, 2000)

	result := recurringOf(t, cheap, dear)

	assert.Equal(t, []string{"Bbb", "Aaa"}, payeesOf(result))
}

func Test_recurring_lists_the_most_recently_charged_ended_series_first(t *testing.T) {
	older := monthlyOf(t, "Aaa", "2026-05-01", 1000)
	newer := monthlyOf(t, "Bbb", "2026-06-01", 1000)

	result := recurringOf(t, older, newer)

	assert.Equal(t, []string{"Bbb", "Aaa"}, payeesOf(result))
}

func Test_recurring_orders_series_that_cost_the_same_by_payee_name_ignoring_case(t *testing.T) {
	upper := monthlyOf(t, "Banana", activeLast, 1000)
	lower := monthlyOf(t, "apple", activeLast, 1000)

	result := recurringOf(t, upper, lower)

	assert.Equal(t, []string{"apple", "Banana"}, payeesOf(result))
}

func Test_recurring_orders_series_that_cost_the_same_by_payee_name_before_payee_key(t *testing.T) {
	dashed := monthlyOf(t, "A-b", activeLast, 1000)
	banged := monthlyOf(t, "A!c", activeLast, 1000)

	result := recurringOf(t, dashed, banged)

	assert.Equal(t, []string{"A!c", "A-b"}, payeesOf(result))
}

func Test_recurring_orders_series_with_the_same_payee_name_and_cost_by_payee_id(t *testing.T) {
	later := everyDaysEndingOn(t, activeLast, 30, 4, paidTo("payee-b", "#4411"))
	earlier := everyDaysEndingOn(t, activeLast, 30, 3, paidTo("payee-a", "#4411"))

	result := recurringOf(t, later, earlier)

	assert.Equal(t, []int{3, 4}, chargeCountsOf(result))
}

func chargeCountsOf(result report.Recurring) []int {
	counts := make([]int, len(result.Series))
	for i, s := range result.Series {
		counts[i] = s.ChargeCount
	}
	return counts
}

// monthlyAmounts is one charge a month (30 days apart) ending 2026-09-15, one per amount.
func monthlyAmounts(t *testing.T, amounts ...int64) []store.Charge {
	t.Helper()
	charges := monthlyEndingOn(t, "2026-09-15", len(amounts))
	for i := range charges {
		charges[i].Amount = amounts[i]
	}
	return charges
}

// steppedAmounts is count amounts that start at 1000 and step up 10% of 1000 at each of the first changes steps.
func steppedAmounts(count, changes int) []int64 {
	amounts := make([]int64, count)
	level := int64(1000)
	for i := range amounts {
		if i >= 1 && i <= changes {
			level += 100
		}
		amounts[i] = level
	}
	return amounts
}

// flatThen is eight charges of base followed by last.
func flatThen(base, last int64) []int64 {
	return append(slices.Repeat([]int64{base}, 8), last)
}

func Test_recurring_treats_exactly_5_percent_as_no_price_change(t *testing.T) {
	cases := []struct {
		name string
		last int64
		want int
	}{
		{name: "exactly 5% up is steady", last: 10500, want: 0},
		{name: "exactly 5% down is steady", last: 9500, want: 0},
		{name: "a cent past 5% up is a change", last: 10501, want: 1},
		{name: "a cent past 5% down is a change", last: 9499, want: 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, monthlyAmounts(t, flatThen(10000, c.last)...))

			require.Len(t, result.Series, 1)
			assert.Len(t, result.Series[0].PriceChanges, c.want)
		})
	}
}

func Test_recurring_lists_a_run_with_floor_steps_over_4_changes_and_drops_one_more(t *testing.T) {
	cases := []struct {
		name    string
		amounts []int64
		listed  bool
	}{
		{name: "3 steps allow none: 1 change is dropped", amounts: steppedAmounts(4, 1), listed: false},
		{name: "3 steps allow none: no change is listed", amounts: steppedAmounts(4, 0), listed: true},
		{name: "4 steps allow 1: 1 change is listed", amounts: steppedAmounts(5, 1), listed: true},
		{name: "4 steps allow 1: 2 changes are dropped", amounts: steppedAmounts(5, 2), listed: false},
		{name: "8 steps allow 2: 2 changes are listed", amounts: steppedAmounts(9, 2), listed: true},
		{name: "8 steps allow 2: 3 changes are dropped", amounts: steppedAmounts(9, 3), listed: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, monthlyAmounts(t, c.amounts...))

			assert.Equal(t, c.listed, len(result.Series) == 1)
		})
	}
}

func Test_recurring_lists_an_annual_run_of_two_charges_only_within_5_percent(t *testing.T) {
	cases := []struct {
		name   string
		last   int64
		listed bool
	}{
		{name: "within 5% is listed", last: 10500, listed: true},
		{name: "past 5% is dropped", last: 10501, listed: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := everyDaysEndingOn(t, "2026-09-15", 365, 2)
			charges[0].Amount, charges[1].Amount = 10000, c.last

			result := recurringOf(t, charges)

			assert.Equal(t, c.listed, len(result.Series) == 1)
		})
	}
}

func Test_recurring_lists_a_price_change_whichever_way_the_price_moves_first(t *testing.T) {
	cases := []struct {
		name    string
		amounts []int64
	}{
		{name: "up then down", amounts: []int64{1000, 1000, 1000, 1000, 1000, 1000, 1100, 1000, 1000}},
		{name: "down then up", amounts: []int64{1000, 1000, 1000, 1000, 1000, 1000, 900, 1000, 1000}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, monthlyAmounts(t, c.amounts...))

			require.Len(t, result.Series, 1)
			assert.Len(t, result.Series[0].PriceChanges, 2)
		})
	}
}

func Test_recurring_rounds_change_pct_half_away_from_zero(t *testing.T) {
	cases := []struct {
		name string
		last int64
		want int64
	}{
		{name: "50.5 tenths up rounds to 51", last: 2101, want: 51},
		{name: "50.5 tenths down rounds to -51", last: 1899, want: -51},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, monthlyAmounts(t, flatThen(2000, c.last)...))

			require.Len(t, result.Series, 1)
			assert.Equal(t, c.want, result.Series[0].PriceChanges[0].Tenths)
		})
	}
}

func Test_recurring_measures_the_overall_change_from_the_first_charge_to_the_latest(t *testing.T) {
	amounts := []int64{10000, 10000, 10000, 10000, 10000, 10000, 10000, 11000, 9999}

	result := recurringOf(t, monthlyAmounts(t, amounts...))

	require.Len(t, result.Series, 1)
	assert.Equal(t, int64(10000), result.Series[0].FirstAmount)
	assert.Equal(t, int64(0), result.Series[0].ChangeTenths)
}

func Test_recurring_gives_a_price_change_the_date_of_the_charge_it_lands_on(t *testing.T) {
	charges := monthlyAmounts(t, flatThen(10000, 12000)...)

	result := recurringOf(t, charges)

	require.Len(t, result.Series, 1)
	assert.Equal(t, []report.PriceChange{
		{Date: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), From: 10000, To: 12000, Tenths: 200},
	}, result.Series[0].PriceChanges)
}

func Test_recurring_leaves_a_dropped_series_out_of_the_totals(t *testing.T) {
	steady := monthlyEndingOn(t, "2026-09-15", 9, paidTo("payee-a", "Gym"))
	wobbly := monthlyAmounts(t, steppedAmounts(9, 3)...)
	for i := range wobbly {
		wobbly[i].PayeeID, wobbly[i].Payee = new("payee-b"), new("Rent")
	}

	result := recurringOf(t, steady, wobbly)

	assert.Equal(t, []report.RecurringTotal{{Currency: "CAD", PerYear: 14400}}, result.Totals)
}

// cadenceCase is a Gym charged count times, gapDays apart.
type cadenceCase struct {
	name    string
	gapDays int
	count   int
}

func Test_recurring_recognises_each_cadence_at_the_bounds_of_its_gap_range_and_charge_minimum(t *testing.T) {
	cases := []struct {
		cadenceCase

		want report.Cadence
	}{
		{cadenceCase{"weekly at 6 days", 6, 4}, report.CadenceWeekly},
		{cadenceCase{"weekly at 8 days", 8, 4}, report.CadenceWeekly},
		{cadenceCase{"monthly at 26 days", 26, 3}, report.CadenceMonthly},
		{cadenceCase{"monthly at 35 days", 35, 3}, report.CadenceMonthly},
		{cadenceCase{"quarterly at 84 days", 84, 3}, report.CadenceQuarterly},
		{cadenceCase{"quarterly at 98 days", 98, 3}, report.CadenceQuarterly},
		{cadenceCase{"annual at 350 days", 350, 2}, report.CadenceAnnual},
		{cadenceCase{"annual at 380 days", 380, 2}, report.CadenceAnnual},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, chargesOn(t, everyDays(t, "2024-01-01", c.gapDays, c.count)))

			require.Len(t, result.Series, 1)
			assert.Equal(t, c.want, result.Series[0].Cadence)
			assert.Equal(t, c.count, result.Series[0].ChargeCount)
		})
	}
}

func Test_recurring_lists_no_series_outside_a_cadences_gap_range_or_below_its_charge_minimum(t *testing.T) {
	cases := []cadenceCase{
		{"weekly one day short", 5, 4},
		{"weekly one day past, in no cadence", 9, 4},
		{"weekly with 3 charges", 7, 3},
		{"monthly one day short", 25, 3},
		{"monthly one day past", 36, 3},
		{"monthly with 2 charges", 30, 2},
		{"quarterly one day short", 83, 3},
		{"quarterly one day past", 99, 3},
		{"quarterly with 2 charges", 91, 2},
		{"annual one day short", 349, 2},
		{"annual one day past", 381, 2},
		{"annual with 1 charge", 365, 1},
		{"biweekly", 14, 6},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, chargesOn(t, everyDays(t, "2024-01-01", c.gapDays, c.count)))

			assert.Empty(t, result.Series)
		})
	}
}

func Test_recurring_multiplies_the_latest_amount_by_the_charges_in_a_year(t *testing.T) {
	cases := []struct {
		name    string
		gapDays int
		count   int
		perYear int64
	}{
		{name: "weekly 52 times", gapDays: 7, count: 4, perYear: 52000},
		{name: "monthly 12 times", gapDays: 30, count: 3, perYear: 12000},
		{name: "quarterly 4 times", gapDays: 91, count: 3, perYear: 4000},
		{name: "annual once", gapDays: 365, count: 2, perYear: 1000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			first := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -c.gapDays*(c.count-1))
			result := recurringOf(t, chargesOn(t, everyDays(t, first.Format(time.DateOnly), c.gapDays, c.count), ofAmount(1000)))

			require.Len(t, result.Series, 1)
			assert.Equal(t, new(c.perYear), result.Series[0].PerYear)
		})
	}
}

func Test_recurring_takes_the_cadence_from_the_last_gap_when_weekly_charges_turn_monthly(t *testing.T) {
	weekly := everyDays(t, "2025-10-01", 7, 4)
	monthly := everyDays(t, "2026-03-01", 30, 3)

	result := recurringOf(t, chargesOn(t, append(weekly, monthly...)))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.CadenceMonthly, result.Series[0].Cadence)
	assert.Equal(t, 3, result.Series[0].ChargeCount)
}

func Test_recurring_takes_the_cadence_from_the_last_gap_when_monthly_charges_turn_weekly(t *testing.T) {
	monthly := everyDays(t, "2025-10-01", 30, 3)
	weekly := everyDays(t, "2026-03-01", 7, 4)

	result := recurringOf(t, chargesOn(t, append(monthly, weekly...)))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.CadenceWeekly, result.Series[0].Cadence)
	assert.Equal(t, 4, result.Series[0].ChargeCount)
}

func Test_recurring_starts_the_run_again_after_a_charge_made_a_few_days_after_the_last(t *testing.T) {
	dates := []string{"2026-01-01", "2026-01-31", "2026-03-02", "2026-03-07", "2026-04-06", "2026-05-06", "2026-06-05"}

	result := recurringOf(t, chargesOn(t, dates))

	require.Len(t, result.Series, 1)
	assert.Equal(t, 4, result.Series[0].ChargeCount)
	assert.Equal(t, "2026-03-07", result.Series[0].First.Format(time.DateOnly))
}

var (
	chqAccount   = store.Account{ID: "acct-chq", Name: "Chequing"}
	visaAccount  = store.Account{ID: "acct-visa", Name: "Visa"}
	savAccount   = store.Account{ID: "acct-sav", Name: "Savings"}
	midAccount   = store.Account{ID: "acct-mid", Name: "Mid"}
	namedRefusal = `no account named "Nope"; run quarry accounts --all to list them`
)

// onAccountOf puts every charge in charges on account.
func onAccountOf(account store.Account, charges []store.Charge) []store.Charge {
	for i := range charges {
		charges[i].Account = account
	}
	return charges
}

// netflixOn is a monthly 10.00 Netflix series ending 2026-09-12 on account.
func netflixOn(t *testing.T, account store.Account) []store.Charge {
	t.Helper()
	return onAccountOf(account, monthlyEndingOn(t, "2026-09-12", 4, paidTo("payee-nf", "Netflix"), ofAmount(1000)))
}

// gymOn is a monthly 12.00 Gym series ending 2026-09-12 on account.
func gymOn(t *testing.T, account store.Account) []store.Charge {
	t.Helper()
	return onAccountOf(account, monthlyEndingOn(t, "2026-09-12", 4))
}

// recurringNamed reads allTime's recurring series of charges, merged oldest first and numbered in that order,
// naming accounts from list.
func recurringNamed(t *testing.T, list store.AccountList, charges []store.Charge, names ...string) (report.Recurring, error) {
	t.Helper()
	slices.SortStableFunc(charges, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	for i := range charges {
		charges[i].SourceID = int64(i + 1)
	}
	srv := report.NewServer(report.WithStore(fakeStore{accounts: list, charges: store.Charges{Rows: charges}}))

	return srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: recurringNow, Accounts: names})
}

func Test_recurring_lists_a_series_when_any_charge_of_its_run_is_in_a_named_account(t *testing.T) {
	cases := []struct {
		name  string
		which int
	}{
		{name: "the first charge", which: 0},
		{name: "a middle charge", which: 2},
		{name: "the latest charge", which: 4},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := onAccountOf(chqAccount, monthlyEndingOn(t, "2026-09-12", 5, ofAmount(1200)))
			run[c.which].Account = midAccount

			result, err := recurringNamed(t, accountsOf(chqAccount, midAccount), slices.Clone(run), "Mid")

			require.NoError(t, err)
			require.Len(t, result.Series, 1)
			assert.Equal(t, 5, result.Series[0].ChargeCount)
			assert.Equal(t, run[0].Date, result.Series[0].First)
			assert.Equal(t, run[4].Date, result.Series[0].Last)
			assert.Equal(t, int64(1200), result.Series[0].Amount)
		})
	}
}

func Test_recurring_leaves_out_a_series_no_charge_of_whose_run_is_in_a_named_account(t *testing.T) {
	run := onAccountOf(chqAccount, monthlyEndingOn(t, "2026-09-12", 5))
	run[2].Account = midAccount

	result, err := recurringNamed(t, accountsOf(chqAccount, midAccount, savAccount), run, "Savings")

	require.NoError(t, err)
	assert.Empty(t, result.Series)
}

func Test_recurring_matches_an_account_by_id_and_by_name_ignoring_case(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{name: "by id", arg: "acct-visa"},
		{name: "by name ignoring case", arg: "vISA"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := recurringNamed(t, accountsOf(chqAccount, visaAccount), netflixOn(t, visaAccount), c.arg)

			require.NoError(t, err)
			assert.Equal(t, []string{"Netflix"}, payeesOf(result))
		})
	}
}

func Test_recurring_lists_the_series_of_every_named_account(t *testing.T) {
	charges := slices.Concat(gymOn(t, chqAccount), netflixOn(t, visaAccount),
		onAccountOf(savAccount, monthlyEndingOn(t, "2026-09-12", 4, paidTo("payee-rent", "Rent"))))

	result, err := recurringNamed(t, accountsOf(chqAccount, visaAccount, savAccount), charges, "Chequing", "Visa")

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Gym", "Netflix"}, payeesOf(result))
}

func Test_recurring_totals_only_the_series_it_lists(t *testing.T) {
	charges := slices.Concat(gymOn(t, chqAccount), netflixOn(t, visaAccount))

	result, err := recurringNamed(t, accountsOf(chqAccount, visaAccount), charges, "Visa")

	require.NoError(t, err)
	assert.Equal(t, []report.RecurringTotal{{Currency: "CAD", PerYear: 12000}}, result.Totals)
}

func Test_recurring_leaves_out_a_series_the_steady_gate_drops_even_when_its_account_is_named(t *testing.T) {
	wobbly := onAccountOf(visaAccount, monthlyAmounts(t, steppedAmounts(9, 3)...))

	result, err := recurringNamed(t, accountsOf(visaAccount), wobbly, "Visa")

	require.NoError(t, err)
	assert.Empty(t, result.Series)
	assert.Empty(t, result.Totals)
}

func Test_recurring_echoes_the_named_accounts_in_the_order_given_without_repeats(t *testing.T) {
	result, err := recurringNamed(t, accountsOf(chqAccount, visaAccount), nil, "Visa", "acct-chq", "visa")

	require.NoError(t, err)
	assert.Equal(t, []store.Account{visaAccount, chqAccount}, result.Accounts)
}

func Test_recurring_echoes_no_accounts_when_none_is_named(t *testing.T) {
	result, err := recurringNamed(t, accountsOf(chqAccount), gymOn(t, chqAccount))

	require.NoError(t, err)
	assert.Empty(t, result.Accounts)
	assert.Len(t, result.Series, 1)
}

func Test_recurring_passes_the_named_account_ids_to_the_one_charges_read(t *testing.T) {
	var got store.ChargeParams
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chqAccount, visaAccount), gotCharges: &got, chargesReads: &reads}))

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: recurringNow, Accounts: []string{"Visa", "Chequing"}})

	require.NoError(t, err)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29"), AccountIDs: []string{"acct-visa", "acct-chq"}}, got)
	assert.Equal(t, 1, reads)
}

func Test_recurring_carries_the_span_of_transactions_the_store_gave(t *testing.T) {
	span := store.TransactionRange{First: dateOf(t, "2003-01-04"), Last: dateOf(t, "2025-12-31")}
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Transactions: span}}))

	result, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: recurringNow})

	require.NoError(t, err)
	assert.Equal(t, span, result.Transactions)
}

func Test_recurring_is_empty_only_when_it_lists_no_series(t *testing.T) {
	cases := []struct {
		name    string
		charges []store.Charge
		want    bool
	}{
		{name: "no charges", want: true},
		{name: "one series", charges: gymOn(t, chqAccount), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := recurringNamed(t, accountsOf(chqAccount), c.charges)

			require.NoError(t, err)
			assert.Equal(t, c.want, result.Empty())
		})
	}
}

func Test_recurring_refuses_an_account_it_cannot_pick_without_reading_charges(t *testing.T) {
	list := accountsOf(chqAccount, visaAccount, store.Account{ID: "acct-visa2", Name: "VISA"})
	cases := []struct {
		name string
		arg  string
		want string
	}{
		{name: "an unknown account", arg: "Nope", want: namedRefusal},
		{name: "an ambiguous name", arg: "Visa", want: `2 accounts are named "Visa"; pass one of their ids instead: acct-visa, acct-visa2`},
		{name: "an empty argument", arg: "", want: `no account named ""; run quarry accounts --all to list them`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reads := 0
			srv := report.NewServer(report.WithStore(fakeStore{accounts: list, chargesReads: &reads}))

			_, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: recurringNow, Accounts: []string{c.arg}})

			refusal, ok := errors.AsType[report.RefusalError](err)
			require.True(t, ok)
			assert.Equal(t, c.want, refusal.Error())
			assert.Zero(t, reads)
		})
	}
}

func Test_recurring_refuses_when_the_accounts_read_fails_to_open_the_store(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr, chargesReads: &reads}), report.WithHome(refusalHome))

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Now: recurringNow, Accounts: []string{"Visa"}})

	require.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
	assert.Zero(t, reads)
}

func Test_recurring_refuses_when_the_charges_read_fails_after_the_accounts_were_named(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(visaAccount), chargesErr: openErr}), report.WithHome(refusalHome))

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Now: recurringNow, Accounts: []string{"Visa"}})

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it", refusal.Error())
}

func Test_recurring_reports_an_interrupt_during_the_accounts_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead, chargesReads: &reads}))

	_, err := srv.Recurring(ctx, report.RecurringRequest{Now: recurringNow, Accounts: []string{"Visa"}})

	require.EqualError(t, err, "recurring interrupted")
	assert.Zero(t, reads)
}

func Test_recurring_reports_an_interrupt_during_the_charges_read_after_the_accounts_were_named(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(visaAccount), chargesErr: errDiskRead}))

	_, err := srv.Recurring(ctx, report.RecurringRequest{Now: recurringNow, Accounts: []string{"Visa"}})

	assert.EqualError(t, err, "recurring interrupted")
}

// firstRateDay is the date of the first exchange rate on the store behind recurringListedIn.
var firstRateDay = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

func inCAD(cents int64) chargeOpt { return func(c *store.Charge) { c.AmountCAD = &cents } }

func inUSD(cents int64) chargeOpt { return func(c *store.Charge) { c.AmountUSD = &cents } }

// recurringListedIn reads allTime's series of groups, listed in currency.
func recurringListedIn(t *testing.T, currency money.Currency, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	return recurringRead(t, report.RecurringRequest{Window: allTime, Now: recurringNow, Currency: currency}, firstRateDay, groups...)
}

// usdRun is a monthly Hulu series in USD ending activeLast, five charges of 10.00 USD whose CAD cell
// is 12.00 on the first and 13.00 on the latest.
func usdRun(t *testing.T) []store.Charge {
	t.Helper()
	run := monthlyEndingOn(t, activeLast, 5, paidTo("payee-hulu", "Hulu"), billedIn("USD"), ofAmount(1000), inCAD(1300))
	run[0].AmountCAD = new(int64(1200))
	return run
}

func Test_recurring_lists_a_series_in_the_reporting_currency_at_the_rate_of_its_first_and_latest_charge(t *testing.T) {
	result := recurringListedIn(t, money.CAD, usdRun(t))

	require.Len(t, result.Series, 1)
	got := result.Series[0]
	assert.Equal(t, "CAD", got.Currency)
	assert.Equal(t, int64(1300), got.Amount)
	assert.Equal(t, int64(1200), got.FirstAmount)
	assert.Equal(t, int64(15600), *got.PerYear)
	assert.Equal(t, "USD", got.NativeCurrency)
	assert.Equal(t, int64(1000), got.NativeAmount)
	assert.Equal(t, int64(1000), got.NativeFirstAmount)
	assert.Equal(t, store.Unconverted{FirstRate: firstRateDay}, result.Unconverted)
	assert.Equal(t, money.CAD, result.Currency)
}

func Test_recurring_lists_a_series_in_usd_with_the_usd_cell_of_a_cad_charge(t *testing.T) {
	cad := monthlyEndingOn(t, activeLast, 5, paidTo("payee-gym", "Gym"), ofAmount(1300), inUSD(1000))

	result := recurringListedIn(t, money.USD, cad)

	require.Len(t, result.Series, 1)
	assert.Equal(t, "USD", result.Series[0].Currency)
	assert.Equal(t, int64(1000), result.Series[0].Amount)
	assert.Equal(t, "CAD", result.Series[0].NativeCurrency)
	assert.Equal(t, int64(1300), result.Series[0].NativeAmount)
}

func Test_recurring_lists_a_series_in_its_own_currency_when_the_target_is_native(t *testing.T) {
	result := recurringListedIn(t, money.Native, usdRun(t))

	require.Len(t, result.Series, 1)
	assert.Equal(t, "USD", result.Series[0].Currency)
	assert.Equal(t, int64(1000), result.Series[0].Amount)
	assert.Equal(t, int64(12000), *result.Series[0].PerYear)
	assert.Equal(t, store.Unconverted{}, result.Unconverted)
}

func Test_recurring_keeps_a_series_whose_first_charge_has_no_rate_entirely_native(t *testing.T) {
	run := usdRun(t)
	run[0].AmountCAD = nil

	result := recurringListedIn(t, money.CAD, run)

	require.Len(t, result.Series, 1)
	got := result.Series[0]
	assert.Equal(t, "USD", got.Currency)
	assert.Equal(t, int64(1000), got.Amount)
	assert.Equal(t, int64(1000), got.FirstAmount)
	assert.Equal(t, int64(12000), *got.PerYear)
	assert.Equal(t, store.Unconverted{Transactions: 1, FirstRate: firstRateDay}, result.Unconverted)
}

func Test_recurring_keeps_a_series_whose_latest_charge_has_no_rate_entirely_native(t *testing.T) {
	run := usdRun(t)
	run[len(run)-1].AmountCAD = nil

	result := recurringListedIn(t, money.CAD, run)

	require.Len(t, result.Series, 1)
	assert.Equal(t, "USD", result.Series[0].Currency)
	assert.Equal(t, int64(1000), result.Series[0].Amount)
	assert.Equal(t, 1, result.Unconverted.Transactions)
}

func Test_recurring_converts_a_series_already_in_the_reporting_currency_without_counting_it(t *testing.T) {
	cad := monthlyEndingOn(t, activeLast, 3, paidTo("payee-gym", "Gym"), ofAmount(2000))

	result := recurringListedIn(t, money.CAD, cad)

	require.Len(t, result.Series, 1)
	assert.Equal(t, "CAD", result.Series[0].Currency)
	assert.Equal(t, int64(2000), result.Series[0].Amount)
	assert.Equal(t, store.Unconverted{FirstRate: firstRateDay}, result.Unconverted)
}

func Test_recurring_lists_a_third_currency_series_in_its_own_currency_without_counting_it(t *testing.T) {
	eur := monthlyEndingOn(t, activeLast, 3, paidTo("payee-pasta", "Pasta"), billedIn("EUR"), ofAmount(700))

	result := recurringListedIn(t, money.CAD, eur)

	require.Len(t, result.Series, 1)
	assert.Equal(t, "EUR", result.Series[0].Currency)
	assert.Equal(t, int64(700), result.Series[0].Amount)
	assert.Zero(t, result.Unconverted.Transactions)
}

func Test_recurring_finds_no_price_change_when_only_the_rate_moves(t *testing.T) {
	run := monthlyEndingOn(t, activeLast, 5, paidTo("payee-hulu", "Hulu"), billedIn("USD"), ofAmount(1000))
	for i, cents := range []int64{1200, 1300, 1400, 1500, 1600} {
		run[i].AmountCAD = &cents
	}

	result := recurringListedIn(t, money.CAD, run)

	require.Len(t, result.Series, 1)
	assert.Empty(t, result.Series[0].PriceChanges)
	assert.Zero(t, result.Series[0].ChangeTenths)
}

func Test_recurring_finds_a_price_change_when_the_native_price_moves_and_the_converted_one_does_not(t *testing.T) {
	run := monthlyEndingOn(t, activeLast, 5, paidTo("payee-hulu", "Hulu"), billedIn("USD"), ofAmount(1000), inCAD(1300))
	run[4].Amount = 1200

	result := recurringListedIn(t, money.CAD, run)

	require.Len(t, result.Series, 1)
	got := result.Series[0]
	require.Len(t, got.PriceChanges, 1)
	assert.Equal(t, report.PriceChange{Date: run[4].Date, From: 1000, To: 1200, Tenths: 200}, got.PriceChanges[0])
	assert.Equal(t, int64(200), got.ChangeTenths)
	assert.Equal(t, int64(1300), got.Amount)
}

func Test_recurring_counts_an_unconverted_series_that_has_ended(t *testing.T) {
	run := monthlyEndingOn(t, endedLast, 3, paidTo("payee-hulu", "Hulu"), billedIn("USD"), ofAmount(1000))

	result := recurringListedIn(t, money.CAD, run)

	require.Len(t, result.Series, 1)
	assert.Equal(t, 1, result.Unconverted.Transactions)
}

func Test_recurring_does_not_count_an_unconverted_series_the_window_leaves_out(t *testing.T) {
	run := monthlyEndingOn(t, endedLast, 3, paidTo("payee-hulu", "Hulu"), billedIn("USD"), ofAmount(1000))
	window := store.Window{Since: dateOf(t, "2026-09-20"), Until: dateOf(t, "2026-09-29")}

	result := recurringRead(t, report.RecurringRequest{Window: window, Now: recurringNow, Currency: money.CAD}, firstRateDay, run)

	assert.Empty(t, result.Series)
	assert.Zero(t, result.Unconverted.Transactions)
}

func Test_recurring_does_not_count_an_unconverted_series_no_named_account_charged(t *testing.T) {
	hulu := onAccountOf(visaAccount, monthlyEndingOn(t, activeLast, 3, paidTo("payee-hulu", "Hulu"), billedIn("USD"), ofAmount(1000)))
	gym := onAccountOf(chqAccount, monthlyEndingOn(t, activeLast, 3, paidTo("payee-gym", "Gym"), ofAmount(2000)))
	srv := report.NewServer(report.WithStore(fakeStore{
		accounts: accountsOf(chqAccount, visaAccount),
		charges:  store.Charges{Rows: append(hulu, gym...), FirstRate: firstRateDay},
	}))

	result, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: recurringNow, Currency: money.CAD, Accounts: []string{"Chequing"}})

	require.NoError(t, err)
	assert.Equal(t, []string{"Gym"}, payeesOf(result))
	assert.Zero(t, result.Unconverted.Transactions)
}

func Test_recurring_totals_converted_series_in_the_reporting_currency_and_unconverted_ones_in_their_own(t *testing.T) {
	converted := usdRun(t)
	unconverted := monthlyEndingOn(t, activeLast, 3, paidTo("payee-zoo", "Zoo"), billedIn("USD"), ofAmount(500))
	cad := monthlyEndingOn(t, activeLast, 3, paidTo("payee-gym", "Gym"), ofAmount(2000))

	result := recurringListedIn(t, money.CAD, unconverted, converted, cad)

	assert.Equal(t, []report.RecurringTotal{{Currency: "CAD", PerYear: 39600}, {Currency: "USD", PerYear: 6000}}, result.Totals)
}

func Test_recurring_totals_an_unconverted_cad_series_in_cad_before_the_usd_total(t *testing.T) {
	unconverted := monthlyEndingOn(t, activeLast, 3, paidTo("payee-gym", "Gym"), ofAmount(2000))
	converted := monthlyEndingOn(t, activeLast, 3, paidTo("payee-zoo", "Zoo"), ofAmount(500), inUSD(400))

	result := recurringListedIn(t, money.USD, converted, unconverted)

	assert.Equal(t, []report.RecurringTotal{{Currency: "CAD", PerYear: 24000}, {Currency: "USD", PerYear: 4800}}, result.Totals)
}

func Test_recurring_interleaves_converted_and_own_currency_series_by_converted_yearly_cost(t *testing.T) {
	small := monthlyEndingOn(t, activeLast, 3, paidTo("payee-aaa", "Aaa"), ofAmount(1000))
	middle := monthlyEndingOn(t, activeLast, 3, paidTo("payee-bbb", "Bbb"), billedIn("USD"), ofAmount(500), inCAD(1500))
	large := monthlyEndingOn(t, activeLast, 3, paidTo("payee-ccc", "Ccc"), ofAmount(2000))

	result := recurringListedIn(t, money.CAD, small, middle, large)

	assert.Equal(t, []string{"Ccc", "Bbb", "Aaa"}, payeesOf(result))
}

func Test_recurring_lists_a_cad_series_before_a_converted_one_of_the_same_payee_and_cost_whichever_is_detected_first(t *testing.T) {
	cases := []struct {
		name               string
		cadCount, usdCount int
	}{
		{name: "the cad series is detected first", cadCount: 4, usdCount: 3},
		{name: "the converted series is detected first", cadCount: 3, usdCount: 4},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cad := monthlyEndingOn(t, activeLast, c.cadCount, paidTo("payee-gym", "Gym"), ofAmount(1300))
			usd := monthlyEndingOn(t, activeLast, c.usdCount, paidTo("payee-gym", "Gym"), billedIn("USD"), ofAmount(1000), inCAD(1300))

			result := recurringListedIn(t, money.CAD, usd, cad)

			require.Len(t, result.Series, 2)
			assert.Equal(t, []string{"CAD", "USD"}, nativeCurrenciesOf(result))
		})
	}
}

func Test_recurring_lists_a_usd_series_before_a_converted_one_of_the_same_payee_and_cost_whichever_is_detected_first(t *testing.T) {
	cases := []struct {
		name               string
		usdCount, cadCount int
	}{
		{name: "the usd series is detected first", usdCount: 4, cadCount: 3},
		{name: "the converted series is detected first", usdCount: 3, cadCount: 4},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			usd := monthlyEndingOn(t, activeLast, c.usdCount, paidTo("payee-gym", "Gym"), billedIn("USD"), ofAmount(1000))
			cad := monthlyEndingOn(t, activeLast, c.cadCount, paidTo("payee-gym", "Gym"), ofAmount(1300), inUSD(1000))

			result := recurringListedIn(t, money.USD, cad, usd)

			require.Len(t, result.Series, 2)
			assert.Equal(t, []string{"USD", "CAD"}, nativeCurrenciesOf(result))
		})
	}
}

func Test_recurring_in_native_orders_a_payees_series_by_currency(t *testing.T) {
	usd := monthlyEndingOn(t, activeLast, 4, paidTo("payee-gym", "Gym"), billedIn("USD"), ofAmount(1000))
	cad := monthlyEndingOn(t, activeLast, 3, paidTo("payee-gym", "Gym"), ofAmount(1000))

	result := recurringListedIn(t, money.Native, usd, cad)

	assert.Equal(t, []string{"CAD", "USD"}, nativeCurrenciesOf(result))
}

func Test_recurring_reads_the_charges_once_when_it_converts(t *testing.T) {
	reads := 0
	run := usdRun(t)
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Rows: run, FirstRate: firstRateDay}, chargesReads: &reads}))

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: recurringNow, Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
}

// nativeCurrenciesOf lists the native currency of each series in result, in order.
func nativeCurrenciesOf(result report.Recurring) []string {
	currencies := make([]string, len(result.Series))
	for i, s := range result.Series {
		currencies[i] = s.NativeCurrency
	}
	return currencies
}
