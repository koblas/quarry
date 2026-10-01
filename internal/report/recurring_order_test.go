package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

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
