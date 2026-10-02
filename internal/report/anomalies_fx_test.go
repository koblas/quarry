package report_test

import (
	"slices"
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rates the anomalies fixtures convert at, in millionths of a CAD per USD.
const (
	rate120 money.Rate = 1_200_000
	rate140 money.Rate = 1_400_000
)

func atRate(rate money.Rate) chargeOpt { return func(c *store.Charge) { c.USDCAD = rate } }

// anomaliesListedIn reads anomalies from September 2026 over charges, listed in currency, on a store whose first rate is firstRateDay.
func anomaliesListedIn(t *testing.T, currency money.Currency, charges []store.Charge) report.Anomalies {
	t.Helper()
	slices.SortStableFunc(charges, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Rows: charges, FirstRate: firstRateDay}}))

	got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: since(t, "2026-09-01"), Now: recurringNow, Currency: currency})

	require.NoError(t, err)
	return got
}

// usdHistory is three earlier Gym charges of 100.00 USD, 120.00 CAD at 1.20.
func usdHistory(t *testing.T) []store.Charge {
	t.Helper()
	return earlierCharges(t, 3, 10000, billedIn("USD"), atRate(rate120), inCAD(12000))
}

// bigUSDCharge is a 500.00 USD Gym charge on 2026-09-01 that is 700.00 CAD at 1.40.
func bigUSDCharge(t *testing.T, opts ...chargeOpt) store.Charge {
	t.Helper()
	return chargeOn(t, 9, "2026-09-01", append([]chargeOpt{billedIn("USD"), ofAmount(50000), atRate(rate140), inCAD(70000)}, opts...)...)
}

func Test_anomalies_list_a_usd_charge_and_its_usual_in_cad_at_the_charges_own_rate(t *testing.T) {
	got := anomaliesListedIn(t, money.CAD, append(usdHistory(t), bigUSDCharge(t)))

	require.Len(t, got.Listed, 1)
	an := got.Listed[0]
	assert.Equal(t, "CAD", an.ListedCurrency)
	assert.Equal(t, int64(70000), an.ListedAmount)
	assert.Equal(t, int64(14000), an.ListedUsual)
	assert.Equal(t, "USD", an.Currency)
	assert.Equal(t, int64(50000), an.Amount)
	assert.Equal(t, int64(10000), an.Usual)
	assert.Equal(t, int64(50), an.TimesTenths)
	assert.Equal(t, money.CAD, got.Currency)
	assert.Equal(t, store.Unconverted{FirstRate: firstRateDay}, got.Unconverted)
}

func Test_anomalies_convert_usual_at_the_charges_rate_not_the_baselines(t *testing.T) {
	history := earlierCharges(t, 3, 10000, billedIn("USD"), atRate(rate120), inCAD(12000))
	big := bigUSDCharge(t, atRate(1_000_000))

	got := anomaliesListedIn(t, money.CAD, append(history, big))

	require.Len(t, got.Listed, 1)
	assert.Equal(t, int64(10000), got.Listed[0].ListedUsual)
}

func Test_anomalies_list_a_cad_charge_and_its_usual_in_usd(t *testing.T) {
	history := earlierCharges(t, 3, 10000, atRate(1_250_000), inUSD(8000))
	big := chargeOn(t, 9, "2026-09-01", ofAmount(50000), atRate(1_250_000), inUSD(40000))

	got := anomaliesListedIn(t, money.USD, append(history, big))

	require.Len(t, got.Listed, 1)
	an := got.Listed[0]
	assert.Equal(t, "USD", an.ListedCurrency)
	assert.Equal(t, int64(40000), an.ListedAmount)
	assert.Equal(t, int64(8000), an.ListedUsual)
	assert.Equal(t, "CAD", an.Currency)
	assert.Equal(t, store.Unconverted{FirstRate: firstRateDay}, got.Unconverted)
}

func Test_anomalies_list_every_charge_in_its_own_currency_when_the_target_is_native(t *testing.T) {
	got := anomaliesListedIn(t, money.Native, append(usdHistory(t), bigUSDCharge(t)))

	require.Len(t, got.Listed, 1)
	an := got.Listed[0]
	assert.Empty(t, an.ListedCurrency)
	assert.Zero(t, an.ListedAmount)
	assert.Equal(t, money.Native, got.Currency)
	assert.Equal(t, store.Unconverted{}, got.Unconverted)
}

func Test_anomalies_list_a_charge_already_in_the_reporting_currency_without_a_rate_or_counting_it(t *testing.T) {
	got := anomaliesListedIn(t, money.CAD, append(earlierCharges(t, 3, 10000), chargeOn(t, 9, "2026-09-01", ofAmount(50000))))

	require.Len(t, got.Listed, 1)
	an := got.Listed[0]
	assert.Equal(t, "CAD", an.ListedCurrency)
	assert.Equal(t, int64(50000), an.ListedAmount)
	assert.Equal(t, int64(10000), an.ListedUsual)
	assert.Equal(t, store.Unconverted{FirstRate: firstRateDay}, got.Unconverted)
}

func Test_anomalies_keep_a_charge_with_no_rate_in_its_own_currency_and_count_it(t *testing.T) {
	history := earlierCharges(t, 3, 10000, billedIn("USD"))
	big := chargeOn(t, 9, "2026-09-01", billedIn("USD"), ofAmount(50000))

	got := anomaliesListedIn(t, money.CAD, append(history, big))

	require.Len(t, got.Listed, 1)
	an := got.Listed[0]
	assert.Empty(t, an.ListedCurrency)
	assert.Equal(t, int64(50000), an.Amount)
	assert.Equal(t, store.Unconverted{Transactions: 1, FirstRate: firstRateDay}, got.Unconverted)
}

func Test_anomalies_keep_both_amounts_native_when_only_one_of_them_converts(t *testing.T) {
	cases := []struct {
		name string
		opts []chargeOpt
	}{
		{name: "a converted cell without a rate to convert the usual", opts: []chargeOpt{inCAD(70000)}},
		{name: "a rate without a converted cell", opts: []chargeOpt{atRate(rate140)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			big := chargeOn(t, 9, "2026-09-01", append([]chargeOpt{billedIn("USD"), ofAmount(50000)}, c.opts...)...)

			got := anomaliesListedIn(t, money.CAD, append(earlierCharges(t, 3, 10000, billedIn("USD")), big))

			require.Len(t, got.Listed, 1)
			assert.Empty(t, got.Listed[0].ListedCurrency)
			assert.Equal(t, 1, got.Unconverted.Transactions)
		})
	}
}

func Test_anomalies_list_a_third_currency_charge_in_its_own_currency_without_counting_it(t *testing.T) {
	history := earlierCharges(t, 3, 10000, billedIn("EUR"))
	big := chargeOn(t, 9, "2026-09-01", billedIn("EUR"), ofAmount(50000))

	got := anomaliesListedIn(t, money.CAD, append(history, big))

	require.Len(t, got.Listed, 1)
	assert.Empty(t, got.Listed[0].ListedCurrency)
	assert.Equal(t, store.Unconverted{FirstRate: firstRateDay}, got.Unconverted)
}

func Test_anomalies_convert_a_category_baseline_the_same_way(t *testing.T) {
	history := earlierCharges(t, 10, 10000, unpaid(), inCategory("cat-hw", "Hardware"), billedIn("USD"), atRate(rate120), inCAD(12000))
	big := chargeOn(t, 9, "2026-09-01", unpaid(), inCategory("cat-hw", "Hardware"), billedIn("USD"), ofAmount(60000), atRate(rate140), inCAD(84000))

	got := anomaliesListedIn(t, money.CAD, append(history, big))

	require.Len(t, got.Listed, 1)
	an := got.Listed[0]
	assert.Equal(t, report.BaselineCategory, an.Baseline)
	assert.Equal(t, "CAD", an.ListedCurrency)
	assert.Equal(t, int64(84000), an.ListedAmount)
	assert.Equal(t, int64(14000), an.ListedUsual)
}

func Test_anomalies_finds_none_when_only_the_rate_moves(t *testing.T) {
	big := bigUSDCharge(t, ofAmount(20000), inCAD(28000))

	got := anomaliesListedIn(t, money.CAD, append(usdHistory(t), big))

	assert.Empty(t, got.Listed)
	assert.Equal(t, 1, got.Checked)
	assert.Equal(t, store.Unconverted{FirstRate: firstRateDay}, got.Unconverted)
}

func Test_anomalies_list_a_charge_whose_native_price_jumped_while_the_converted_one_stayed_flat(t *testing.T) {
	big := bigUSDCharge(t, ofAmount(20001), inCAD(12500))

	got := anomaliesListedIn(t, money.CAD, append(usdHistory(t), big))

	require.Len(t, got.Listed, 1)
	assert.Equal(t, int64(12500), got.Listed[0].ListedAmount)
}

func Test_anomalies_never_list_a_usd_charge_under_100_in_its_own_currency(t *testing.T) {
	cases := []struct {
		name       string
		amount     int64
		wantListed int
	}{
		{name: "99.99 USD is under the floor though it is over 100.00 CAD", amount: 9999, wantListed: 0},
		{name: "100.00 USD is at the floor", amount: 10000, wantListed: 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			history := earlierCharges(t, 3, 1000, billedIn("USD"), atRate(rate140), inCAD(1400))
			big := chargeOn(t, 9, "2026-09-01", billedIn("USD"), ofAmount(c.amount), atRate(rate140), inCAD(c.amount*14/10))

			got := anomaliesListedIn(t, money.CAD, append(history, big))

			assert.Len(t, got.Listed, c.wantListed)
			assert.Equal(t, 1, got.Checked)
		})
	}
}

func Test_anomalies_judge_alike_in_every_reporting_currency(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
	}{
		{name: "CAD", currency: money.CAD},
		{name: "USD", currency: money.USD},
		{name: "native", currency: money.Native},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			history := usdHistory(t)
			usual := chargeOn(t, 8, "2026-09-01", billedIn("USD"), ofAmount(15000), atRate(rate140), inCAD(21000), inUSD(15000))
			lone := chargeOn(t, 9, "2026-09-02", paidTo("payee-new", "New"), billedIn("USD"), ofAmount(20000), atRate(rate140), inCAD(28000))
			big := bigUSDCharge(t, inUSD(50000), func(c *store.Charge) { c.SourceID = 10 })

			got := anomaliesListedIn(t, c.currency, slices.Concat(history, []store.Charge{usual, lone, big}))

			assert.Equal(t, []int64{50000}, amountsOf(got.Listed))
			assert.Equal(t, 3, got.Checked)
			assert.Equal(t, 1, got.NotJudged)
		})
	}
}

func Test_anomalies_count_only_listed_unconverted_charges(t *testing.T) {
	window := since(t, "2026-09-01")
	history := func(id string) []store.Charge {
		return earlierCharges(t, 3, 10000, billedIn("USD"), paidTo(id, id))
	}
	charges := slices.Concat(history("A"), history("B"), history("C"),
		[]store.Charge{
			chargeOn(t, 20, "2026-09-01", paidTo("A", "A"), billedIn("USD"), ofAmount(50000)),
			chargeOn(t, 21, "2026-09-02", paidTo("B", "B"), billedIn("USD"), ofAmount(15000)),
			chargeOn(t, 22, "2026-08-31", paidTo("C", "C"), billedIn("USD"), ofAmount(50000)),
		})
	slices.SortStableFunc(charges, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Rows: charges, FirstRate: firstRateDay}}))

	got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: window, Now: recurringNow, Currency: money.CAD})

	require.NoError(t, err)
	assert.Equal(t, []int64{50000}, amountsOf(got.Listed))
	assert.Equal(t, 1, got.Unconverted.Transactions)
}

func Test_anomalies_count_nothing_for_an_unconverted_charge_in_an_account_that_was_not_named(t *testing.T) {
	history := onAccountOf(chqAccount, earlierCharges(t, 3, 10000, billedIn("USD")))
	large := onAccountOf(visaAccount, []store.Charge{chargeOn(t, 9, "2026-09-01", billedIn("USD"), ofAmount(50000))})
	charges := slices.Concat(history, large)
	slices.SortStableFunc(charges, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	srv := report.NewServer(report.WithStore(fakeStore{accounts: bothAccounts, charges: store.Charges{Rows: charges, FirstRate: firstRateDay}}))

	got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow, Accounts: []string{"Chequing"}, Currency: money.CAD})

	require.NoError(t, err)
	assert.Empty(t, got.Listed)
	assert.Zero(t, got.Unconverted.Transactions)
}

func Test_anomalies_read_the_charges_once_whatever_the_reporting_currency(t *testing.T) {
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{chargesReads: &reads}))

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow, Currency: money.USD})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
}
