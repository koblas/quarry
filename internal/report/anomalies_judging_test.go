package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// since is the window from date through recurringNow's day.
func since(t *testing.T, date string) store.Window {
	t.Helper()
	return store.Window{Since: dateOf(t, date), Until: thisYear.Until}
}

// earlierCharges is count charges of amount cents, 30 days apart from 2025-06-01.
func earlierCharges(t *testing.T, count int, amount int64, opts ...chargeOpt) []store.Charge {
	t.Helper()
	return chargesOn(t, everyDays(t, "2025-06-01", 30, count), append([]chargeOpt{ofAmount(amount)}, opts...)...)
}

// anomaliesIn is the Anomalies read over charges, in window, at recurringNow.
func anomaliesIn(t *testing.T, window store.Window, charges []store.Charge) report.Anomalies {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Rows: charges}}))

	got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: window, Now: recurringNow})

	require.NoError(t, err)
	return got
}

func amountsOf(anomalies []report.Anomaly) []int64 {
	amounts := make([]int64, 0, len(anomalies))
	for _, a := range anomalies {
		amounts = append(amounts, a.Amount)
	}
	return amounts
}

func Test_anomalies_need_three_earlier_charges_for_a_payee_baseline(t *testing.T) {
	cases := []struct {
		name          string
		earlier       int
		wantListed    []int64
		wantNotJudged int
	}{
		{name: "two earlier charges are too few", earlier: 2, wantListed: []int64{}, wantNotJudged: 1},
		{name: "three earlier charges are enough", earlier: 3, wantListed: []int64{50000}, wantNotJudged: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, c.earlier, 10000), chargeOn(t, 9, "2026-09-01", ofAmount(50000)))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Equal(t, c.wantListed, amountsOf(got.Listed))
			assert.Equal(t, c.wantNotJudged, got.NotJudged)
		})
	}
}

func Test_anomalies_do_not_count_same_day_charges_as_history(t *testing.T) {
	cases := []struct {
		name          string
		thirdDate     string
		wantListed    []int64
		wantNotJudged int
	}{
		{name: "a charge the day before is history", thirdDate: "2026-08-31", wantListed: []int64{50000}, wantNotJudged: 0},
		{name: "a charge the same day is not", thirdDate: "2026-09-01", wantListed: []int64{}, wantNotJudged: 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 2, 10000),
				chargeOn(t, 3, c.thirdDate, ofAmount(10000)),
				chargeOn(t, 9, "2026-09-01", ofAmount(50000)))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Equal(t, c.wantListed, amountsOf(got.Listed))
			assert.Equal(t, c.wantNotJudged, got.NotJudged)
		})
	}
}

func Test_anomalies_never_list_a_charge_under_the_minimum(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		want   []int64
	}{
		{name: "a cent under 100.00 is not listed", amount: 9999, want: []int64{}},
		{name: "exactly 100.00 is listed", amount: 10000, want: []int64{10000}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 3, 1000), chargeOn(t, 9, "2026-09-01", ofAmount(c.amount)))

			got := anomaliesIn(t, thisYear, charges)

			assert.Equal(t, c.want, amountsOf(got.Listed))
		})
	}
}

func Test_anomalies_count_only_charges_of_100_or_more_without_a_baseline_as_not_judged(t *testing.T) {
	cases := []struct {
		name    string
		earlier int
		amount  int64
		want    int
	}{
		{name: "100.00 with no baseline is not judged", earlier: 0, amount: 10000, want: 1},
		{name: "99.99 with no baseline is judged by the minimum", earlier: 0, amount: 9999, want: 0},
		{name: "99.99 with a baseline is judged, not unusual", earlier: 3, amount: 9999, want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, c.earlier, 1000), chargeOn(t, 9, "2026-09-01", ofAmount(c.amount)))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Equal(t, 1, got.Checked)
			assert.Equal(t, c.want, got.NotJudged)
		})
	}
}

func Test_anomalies_count_a_charge_with_no_payee_as_checked_but_not_judged(t *testing.T) {
	charges := append(earlierCharges(t, 3, 10000, unpaid()), chargeOn(t, 9, "2026-09-01", unpaid(), ofAmount(50000)))

	got := anomaliesIn(t, since(t, "2026-09-01"), charges)

	assert.Equal(t, 1, got.Checked)
	assert.Equal(t, 1, got.NotJudged)
	assert.Empty(t, got.Listed)
}

func Test_anomalies_take_history_from_the_same_payee_and_currency_only(t *testing.T) {
	cases := []struct {
		name string
		opts []chargeOpt
	}{
		{name: "another payee's charges are not history", opts: []chargeOpt{paidTo("payee-cafe", "Cafe")}},
		{name: "another currency's charges are not history", opts: []chargeOpt{billedIn("USD")}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 3, 10000, c.opts...), chargeOn(t, 9, "2026-09-01", ofAmount(50000)))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Equal(t, 1, got.NotJudged)
			assert.Empty(t, got.Listed)
		})
	}
}

func Test_anomalies_take_history_from_every_account(t *testing.T) {
	charges := append(earlierCharges(t, 3, 10000, onAccount("acct-visa", "Visa")), chargeOn(t, 9, "2026-09-01", ofAmount(50000)))

	got := anomaliesIn(t, since(t, "2026-09-01"), charges)

	assert.Equal(t, []int64{50000}, amountsOf(got.Listed))
}

func Test_anomalies_judge_each_charge_by_the_charges_before_it(t *testing.T) {
	charges := chargesOn(t, []string{"2026-01-01", "2026-02-01", "2026-03-01", "2026-04-01"}, ofAmount(10000))

	got := anomaliesIn(t, thisYear, charges)

	assert.Equal(t, 4, got.Checked)
	assert.Equal(t, 3, got.NotJudged)
}

func Test_anomalies_usual_is_the_median_of_the_earlier_charges(t *testing.T) {
	cases := []struct {
		name    string
		earlier []int64
		want    int64
	}{
		{name: "odd count takes the middle by amount, not by date", earlier: []int64{30000, 10000, 20000}, want: 20000},
		{name: "even count rounds an odd cent total up", earlier: []int64{9000, 12000, 12001, 20000}, want: 12001},
		{name: "even count with an even total is exact", earlier: []int64{9000, 12000, 12002, 20000}, want: 12001},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := []store.Charge{}
			for i, amount := range c.earlier {
				charges = append(charges, chargeOn(t, int64(i+1), time.Date(2025, 6, 1+i, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), ofAmount(amount)))
			}
			charges = append(charges, chargeOn(t, 9, "2026-09-01", ofAmount(100000)))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			require.Len(t, got.Listed, 1)
			assert.Equal(t, c.want, got.Listed[0].Usual)
		})
	}
}

func Test_anomalies_compare_with_every_earlier_charge_not_the_first_three(t *testing.T) {
	charges := append(chargesOn(t, []string{"2025-06-01", "2025-06-02"}, ofAmount(50000)),
		chargeOn(t, 3, "2025-06-03", ofAmount(10000)),
		chargeOn(t, 4, "2025-06-04", ofAmount(10000)),
		chargeOn(t, 5, "2025-06-05", ofAmount(10000)),
		chargeOn(t, 9, "2026-09-01", ofAmount(30000)))

	got := anomaliesIn(t, since(t, "2026-09-01"), charges)

	require.Len(t, got.Listed, 1)
	assert.Equal(t, 5, got.Listed[0].Earlier)
	assert.Equal(t, int64(10000), got.Listed[0].Usual)
}

func Test_anomalies_count_an_earlier_anomaly_as_history(t *testing.T) {
	charges := append(earlierCharges(t, 3, 10000),
		chargeOn(t, 8, "2026-09-01", ofAmount(50000)),
		chargeOn(t, 9, "2026-09-02", ofAmount(60000)))

	got := anomaliesIn(t, since(t, "2026-09-01"), charges)

	require.Len(t, got.Listed, 2)
	assert.Equal(t, []int{4, 3}, []int{got.Listed[0].Earlier, got.Listed[1].Earlier})
}

func Test_anomalies_state_the_multiple_to_a_tenth_rounded_half_away_from_zero(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		want   int64
	}{
		{name: "exactly 4.25 times rounds up to 4.3", amount: 170000, want: 43},
		{name: "just under 4.25 times rounds down to 4.2", amount: 169999, want: 42},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 3, 40000), chargeOn(t, 9, "2026-09-01", ofAmount(c.amount)))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			require.Len(t, got.Listed, 1)
			assert.Equal(t, c.want, got.Listed[0].TimesTenths)
		})
	}
}
