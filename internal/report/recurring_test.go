package report_test

import (
	"slices"
	"testing"
	"time"

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
