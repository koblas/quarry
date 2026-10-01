package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func Test_recurring_lists_native_series_by_their_own_currency_for_a_payee_charging_in_both(t *testing.T) {
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
