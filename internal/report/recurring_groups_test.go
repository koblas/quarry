package report_test

import (
	"context"
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
