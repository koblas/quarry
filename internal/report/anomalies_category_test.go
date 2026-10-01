package report_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func inGroceries() chargeOpt { return inCategory("cat-groceries", "Food:Groceries") }

// categoryHistory is count groceries charges of amount cents, each from its own payee, 30 days apart from 2025-06-01.
func categoryHistory(t *testing.T, count int, amount int64, opts ...chargeOpt) []store.Charge {
	t.Helper()
	charges := earlierCharges(t, count, amount, append([]chargeOpt{inGroceries()}, opts...)...)
	for i := range charges {
		paidTo(fmt.Sprintf("payee-%d", i), fmt.Sprintf("Vendor %d", i))(&charges[i])
	}
	return charges
}

// newcomerCharge is a groceries charge of amount cents on date (YYYY-MM-DD) by a payee with no earlier charge.
func newcomerCharge(t *testing.T, date string, amount int64, opts ...chargeOpt) store.Charge {
	t.Helper()
	return chargeOn(t, 99, date, append([]chargeOpt{paidTo("payee-new", "Newcomer"), inGroceries(), ofAmount(amount)}, opts...)...)
}

func Test_anomalies_need_ten_earlier_charges_in_the_category_for_a_category_baseline(t *testing.T) {
	cases := []struct {
		name          string
		earlier       int
		wantListed    []int64
		wantNotJudged int
	}{
		{name: "nine earlier charges are too few", earlier: 9, wantListed: []int64{}, wantNotJudged: 1},
		{name: "ten earlier charges are enough", earlier: 10, wantListed: []int64{100001}, wantNotJudged: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(categoryHistory(t, c.earlier, 20000), newcomerCharge(t, "2026-09-01", 100001))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Equal(t, c.wantListed, amountsOf(got.Listed))
			assert.Equal(t, c.wantNotJudged, got.NotJudged)
		})
	}
}

func Test_anomalies_list_a_category_charge_only_over_five_times_the_median(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		want   []int64
	}{
		{name: "exactly five times the median is not listed", amount: 100000, want: []int64{}},
		{name: "a cent over five times the median is listed", amount: 100001, want: []int64{100001}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(categoryHistory(t, 10, 20000), newcomerCharge(t, "2026-09-01", c.amount))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Equal(t, c.want, amountsOf(got.Listed))
		})
	}
}

func Test_anomalies_compare_a_category_charge_with_every_earlier_charge_of_any_account(t *testing.T) {
	history := categoryHistory(t, 11, 20000, onAccount("acct-visa", "Visa"))
	for i := range history {
		history[i].Amount = int64(25000 - 1000*i)
	}
	charges := slices.Concat(history, []store.Charge{newcomerCharge(t, "2026-09-01", 184210)})

	got := anomaliesIn(t, since(t, "2026-09-01"), charges)

	require.Len(t, got.Listed, 1)
	listed := got.Listed[0]
	assert.Equal(t, report.BaselineCategory, listed.Baseline)
	assert.Equal(t, int64(20000), listed.Usual)
	assert.Equal(t, 11, listed.Earlier)
	assert.Equal(t, int64(92), listed.TimesTenths)
}

func Test_anomalies_do_not_count_other_charges_as_category_history(t *testing.T) {
	cases := []struct {
		name string
		opt  chargeOpt
	}{
		{name: "an earlier charge in another currency", opt: billedIn("USD")},
		{name: "an earlier charge in another category", opt: inCategory("cat-fuel", "Auto:Fuel")},
		{name: "an earlier charge in several categories", opt: func(c *store.Charge) { c.Category, c.ExpenseSplits = nil, 2 }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			history := categoryHistory(t, 10, 20000)
			c.opt(&history[9])
			charges := slices.Concat(history, []store.Charge{newcomerCharge(t, "2026-09-01", 100001)})

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Empty(t, got.Listed)
			assert.Equal(t, 1, got.NotJudged)
		})
	}
}

func Test_anomalies_do_not_count_category_charges_that_do_not_precede_the_charge(t *testing.T) {
	cases := []struct {
		name string
		date string
	}{
		{name: "a charge on the same day", date: "2026-09-01"},
		{name: "a charge on a later day", date: "2026-09-02"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(categoryHistory(t, 9, 20000), newcomerCharge(t, "2026-09-01", 100001),
				chargeOn(t, 100, c.date, paidTo("payee-late", "Latecomer"), inGroceries(), ofAmount(5000)))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Empty(t, got.Listed)
			assert.Equal(t, 1, got.NotJudged)
		})
	}
}

func Test_anomalies_give_no_category_baseline_to_a_charge_without_a_single_category(t *testing.T) {
	cases := []struct {
		name string
		opt  chargeOpt
	}{
		{name: "uncategorized", opt: func(c *store.Charge) { c.Category, c.ExpenseSplits = nil, 1 }},
		{name: "split across categories", opt: func(c *store.Charge) { c.Category, c.ExpenseSplits = nil, 2 }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(categoryHistory(t, 10, 20000), newcomerCharge(t, "2026-09-01", 100001, c.opt))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Empty(t, got.Listed)
			assert.Equal(t, 1, got.NotJudged)
		})
	}
}

func Test_anomalies_give_the_category_baseline_to_a_charge_split_within_one_category(t *testing.T) {
	split := func(c *store.Charge) { c.ExpenseSplits = 2 }
	charges := append(categoryHistory(t, 10, 20000), newcomerCharge(t, "2026-09-01", 100001, split))

	got := anomaliesIn(t, since(t, "2026-09-01"), charges)

	require.Len(t, got.Listed, 1)
	assert.Equal(t, report.BaselineCategory, got.Listed[0].Baseline)
}

func Test_anomalies_judge_a_payee_with_three_earlier_charges_on_the_payee_never_the_category(t *testing.T) {
	cases := []struct {
		name          string
		payeeEarlier  int
		wantListed    []int64
		wantNotJudged int
	}{
		{name: "two earlier charges fall back to the category", payeeEarlier: 2, wantListed: []int64{150000}, wantNotJudged: 0},
		{name: "three earlier charges keep the payee baseline", payeeEarlier: 3, wantListed: []int64{}, wantNotJudged: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hardware := chargesOn(t, everyDays(t, "2026-02-01", 30, c.payeeEarlier),
				paidTo("payee-hw", "Hardware"), inGroceries(), ofAmount(100000))
			others := categoryHistory(t, 10-c.payeeEarlier, 10000)
			charges := append(append(others, hardware...), newcomerCharge(t, "2026-09-01", 150000, paidTo("payee-hw", "Hardware")))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			assert.Equal(t, c.wantListed, amountsOf(got.Listed))
			assert.Equal(t, c.wantNotJudged, got.NotJudged)
		})
	}
}

func Test_anomalies_list_a_charge_without_a_payee_against_its_category(t *testing.T) {
	charges := append(categoryHistory(t, 10, 20000), newcomerCharge(t, "2026-09-01", 100001, unpaid()))

	got := anomaliesIn(t, since(t, "2026-09-01"), charges)

	require.Len(t, got.Listed, 1)
	assert.Nil(t, got.Listed[0].Payee)
	assert.Equal(t, report.BaselineCategory, got.Listed[0].Baseline)
}
