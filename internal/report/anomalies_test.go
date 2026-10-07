package report_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// thisYear is the window the anomalies tests list: January 1 through recurringNow's day.
var thisYear = store.Window{
	Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	Until: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
}

func Test_anomalies_thresholds_hold_at_their_boundaries(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		want   []int64
	}{
		{name: "exactly twice the median is not listed", amount: 12000, want: []int64{}},
		{name: "a cent over twice the median is listed", amount: 12001, want: []int64{12001}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 3, 6000), chargeOn(t, 9, "2026-09-01", ofAmount(c.amount)))

			got := anomaliesIn(t, thisYear, charges)

			assert.Equal(t, c.want, amountsOf(got.Listed))
		})
	}
}

// anomaliesNamed reads thisYear's anomalies over charges, ordered by date, naming accounts from list.
func anomaliesNamed(t *testing.T, list store.AccountList, charges []store.Charge, names ...string) report.Anomalies {
	t.Helper()
	return anomaliesNamedIn(t, thisYear, list, charges, names...)
}

// anomaliesNamedIn is anomaliesNamed over window.
func anomaliesNamedIn(t *testing.T, window store.Window, list store.AccountList, charges []store.Charge, names ...string) report.Anomalies {
	t.Helper()
	slices.SortStableFunc(charges, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	srv := report.NewServer(report.WithStore(fakeStore{accounts: list, charges: store.Charges{Rows: charges}}))

	got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: window, Now: recurringNow, Accounts: names})

	require.NoError(t, err)
	return got
}

var bothAccounts = accountsOf(chqAccount, visaAccount)

// payeeHistoryOn is three earlier 100.00 Gym charges on account.
func payeeHistoryOn(t *testing.T, account store.Account) []store.Charge {
	t.Helper()
	return onAccountOf(account, earlierCharges(t, 3, 10000))
}

// largeGymOn is a 500.00 Gym charge on date numbered sourceID, on account.
func largeGymOn(t *testing.T, account store.Account, sourceID int64, date string) store.Charge {
	t.Helper()
	return onAccountOf(account, []store.Charge{chargeOn(t, sourceID, date, ofAmount(50000))})[0]
}

func Test_anomalies_keep_charges_of_other_accounts_in_the_payees_history(t *testing.T) {
	cases := []struct {
		name  string
		names []string
	}{
		{name: "a named account", names: []string{"Visa"}},
		{name: "no account named", names: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(payeeHistoryOn(t, chqAccount), largeGymOn(t, visaAccount, 9, "2026-09-01"))

			got := anomaliesNamed(t, bothAccounts, charges, c.names...)

			require.Len(t, got.Listed, 1)
			assert.Equal(t, report.BaselinePayee, got.Listed[0].Baseline)
			assert.Equal(t, 3, got.Listed[0].Earlier)
			assert.Equal(t, "acct-visa", got.Listed[0].Account.ID)
			assert.Zero(t, got.NotJudged)
		})
	}
}

func Test_anomalies_keep_charges_of_other_accounts_in_the_categorys_history(t *testing.T) {
	cases := []struct {
		name  string
		names []string
	}{
		{name: "a named account", names: []string{"Visa"}},
		{name: "no account named", names: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(onAccountOf(chqAccount, categoryHistory(t, 10, 20000)), newcomerCharge(t, "2026-09-01", 100001, func(c *store.Charge) { c.Account = visaAccount }))

			got := anomaliesNamed(t, bothAccounts, charges, c.names...)

			require.Len(t, got.Listed, 1)
			assert.Equal(t, report.BaselineCategory, got.Listed[0].Baseline)
			assert.Equal(t, 10, got.Listed[0].Earlier)
		})
	}
}

func Test_anomalies_count_only_the_named_accounts_charges_as_checked_and_not_judged(t *testing.T) {
	cases := []struct {
		name                       string
		names                      []string
		wantListed                 []int64
		wantChecked, wantNotJudged int
	}{
		{name: "a named account counts its own charges", names: []string{"Visa"}, wantListed: []int64{50000}, wantChecked: 2, wantNotJudged: 1},
		{name: "no account named counts every charge", names: nil, wantListed: []int64{50000, 50000}, wantChecked: 4, wantNotJudged: 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := slices.Concat(payeeHistoryOn(t, chqAccount), []store.Charge{
				largeGymOn(t, visaAccount, 9, "2026-09-01"),
				largeGymOn(t, chqAccount, 10, "2026-09-02"),
				onAccountOf(visaAccount, []store.Charge{chargeOn(t, 11, "2026-09-03", paidTo("payee-new", "New"), ofAmount(20000))})[0],
				onAccountOf(chqAccount, []store.Charge{chargeOn(t, 12, "2026-09-04", paidTo("payee-new", "New"), ofAmount(20000))})[0],
			})

			got := anomaliesNamed(t, bothAccounts, charges, c.names...)

			assert.Equal(t, c.wantListed, amountsOf(got.Listed))
			assert.Equal(t, c.wantChecked, got.Checked)
			assert.Equal(t, c.wantNotJudged, got.NotJudged)
		})
	}
}

func Test_anomalies_list_and_count_only_the_named_accounts_payeeless_category_charges(t *testing.T) {
	cases := []struct {
		name         string
		names        []string
		wantAccounts []string
		wantChecked  int
	}{
		{name: "a named account", names: []string{"Visa"}, wantAccounts: []string{"acct-visa"}, wantChecked: 1},
		{name: "no account named", names: nil, wantAccounts: []string{"acct-visa", "acct-chq"}, wantChecked: 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := slices.Concat(onAccountOf(chqAccount, categoryHistory(t, 10, 20000)), []store.Charge{
				onAccountOf(visaAccount, []store.Charge{chargeOn(t, 9, "2026-09-02", unpaid(), inGroceries(), ofAmount(100001))})[0],
				onAccountOf(chqAccount, []store.Charge{chargeOn(t, 10, "2026-09-02", unpaid(), inGroceries(), ofAmount(100001))})[0],
			})

			got := anomaliesNamedIn(t, since(t, "2026-09-01"), bothAccounts, charges, c.names...)

			ids := make([]string, 0, len(got.Listed))
			for _, a := range got.Listed {
				assert.Equal(t, report.BaselineCategory, a.Baseline)
				assert.Equal(t, 10, a.Earlier)
				ids = append(ids, a.Account.ID)
			}
			assert.ElementsMatch(t, c.wantAccounts, ids)
			assert.Equal(t, c.wantChecked, got.Checked)
		})
	}
}

func Test_anomalies_count_only_the_named_accounts_payeeless_charges_without_a_baseline_as_not_judged(t *testing.T) {
	cases := []struct {
		name          string
		names         []string
		wantNotJudged int
	}{
		{name: "a named account", names: []string{"Visa"}, wantNotJudged: 1},
		{name: "no account named", names: nil, wantNotJudged: 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := []store.Charge{
				onAccountOf(visaAccount, []store.Charge{chargeOn(t, 9, "2026-09-02", unpaid(), ofAmount(10000))})[0],
				onAccountOf(chqAccount, []store.Charge{chargeOn(t, 10, "2026-09-03", unpaid(), ofAmount(10000))})[0],
			}

			got := anomaliesNamed(t, bothAccounts, charges, c.names...)

			assert.Equal(t, c.wantNotJudged, got.NotJudged)
			assert.Equal(t, c.wantNotJudged, got.Checked)
			assert.Empty(t, got.Listed)
		})
	}
}

func Test_anomalies_match_an_account_by_id_and_by_name_ignoring_case(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{name: "by id", arg: "acct-visa"},
		{name: "by name ignoring case", arg: "vISA"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(payeeHistoryOn(t, chqAccount), largeGymOn(t, visaAccount, 9, "2026-09-01"), largeGymOn(t, chqAccount, 10, "2026-09-02"))

			got := anomaliesNamed(t, bothAccounts, charges, c.arg)

			require.Len(t, got.Listed, 1)
			assert.Equal(t, "acct-visa", got.Listed[0].Account.ID)
		})
	}
}

func Test_anomalies_list_the_charges_of_every_named_account(t *testing.T) {
	list := accountsOf(chqAccount, visaAccount, savAccount)
	charges := slices.Concat(payeeHistoryOn(t, chqAccount), []store.Charge{
		largeGymOn(t, visaAccount, 9, "2026-09-01"),
		largeGymOn(t, chqAccount, 10, "2026-09-02"),
		largeGymOn(t, savAccount, 11, "2026-09-03"),
	})

	got := anomaliesNamed(t, list, charges, "Chequing", "Visa")

	ids := make([]string, 0, len(got.Listed))
	for _, a := range got.Listed {
		ids = append(ids, a.Account.ID)
	}
	assert.ElementsMatch(t, []string{"acct-chq", "acct-visa"}, ids)
	assert.Equal(t, 2, got.Checked)
}

func Test_anomalies_match_a_closed_named_account(t *testing.T) {
	closed := store.Account{ID: "acct-old", Name: "Old Card", Closed: true}
	charges := append(payeeHistoryOn(t, chqAccount), largeGymOn(t, closed, 9, "2026-09-01"))

	got := anomaliesNamed(t, accountsOf(chqAccount, closed), charges, "Old Card")

	assert.Equal(t, []int64{50000}, amountsOf(got.Listed))
}

func Test_anomalies_echo_the_named_accounts_in_the_order_given_without_repeats(t *testing.T) {
	got := anomaliesNamed(t, bothAccounts, nil, "Visa", "acct-chq", "visa")

	assert.Equal(t, []store.Account{visaAccount, chqAccount}, got.Accounts)
}

func Test_anomalies_echo_no_accounts_when_none_is_named(t *testing.T) {
	got := anomaliesNamed(t, bothAccounts, nil)

	assert.Empty(t, got.Accounts)
}

func Test_anomalies_pass_the_named_account_ids_to_the_one_charges_read(t *testing.T) {
	var got store.ChargeParams
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{accounts: bothAccounts, gotCharges: &got, chargesReads: &reads}))

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow, Accounts: []string{"Visa", "Chequing"}})

	require.NoError(t, err)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29"), AccountIDs: []string{"acct-visa", "acct-chq"}}, got)
	assert.Equal(t, 1, reads)
}

func Test_anomalies_refuse_an_account_they_cannot_pick_without_reading_charges(t *testing.T) {
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

			_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow, Accounts: []string{c.arg}})

			refusal, ok := errors.AsType[report.RefusalError](err)
			require.True(t, ok)
			assert.Equal(t, c.want, refusal.Error())
			assert.Zero(t, reads)
		})
	}
}

func Test_anomalies_refuse_when_the_accounts_read_fails_to_open_the_store(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr, chargesReads: &reads}), report.WithHome(refusalHome))

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow, Accounts: []string{"Visa"}})

	require.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
	assert.Zero(t, reads)
}

func Test_anomalies_report_an_interrupt_during_the_accounts_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead, chargesReads: &reads}))

	_, err := srv.Anomalies(ctx, report.AnomaliesRequest{Window: thisYear, Now: recurringNow, Accounts: []string{"Visa"}})

	require.EqualError(t, err, "anomalies interrupted")
	assert.Zero(t, reads)
}

func Test_anomalies_report_an_interrupt_during_the_charges_read_after_the_accounts_were_named(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(visaAccount), chargesErr: errDiskRead}))

	_, err := srv.Anomalies(ctx, report.AnomaliesRequest{Window: thisYear, Now: recurringNow, Accounts: []string{"Visa"}})

	assert.EqualError(t, err, "anomalies interrupted")
}

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

// anomaliesIn is the Anomalies read over charges, in window, at recurringNow.
func anomaliesIn(t *testing.T, window store.Window, charges []store.Charge) report.Anomalies {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Rows: charges}}))

	got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: window, Now: recurringNow})

	require.NoError(t, err)
	return got
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

func Test_anomalies_read_the_charges_once_through_today_whatever_the_window(t *testing.T) {
	var got store.ChargeParams
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{gotCharges: &got, chargesReads: &reads}))
	window := store.Window{Since: allTime.Since, Until: dateOf(t, "2030-12-31")}

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: window, Now: recurringNow})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29")}, got)
}

func Test_anomalies_read_the_charges_through_the_local_date_when_the_utc_date_is_later(t *testing.T) {
	var got store.ChargeParams
	srv := report.NewServer(report.WithStore(fakeStore{gotCharges: &got}))

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: windowNow})

	require.NoError(t, err)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29")}, got)
}

func Test_anomalies_return_the_window_and_the_transaction_span_of_the_read(t *testing.T) {
	span := store.TransactionRange{First: dateOf(t, "2003-01-04"), Last: dateOf(t, "2026-09-26")}
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Transactions: span}}))

	got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow})

	require.NoError(t, err)
	assert.Equal(t, thisYear, got.Window)
	assert.Equal(t, span, got.Transactions)
}

func Test_anomalies_refuse_a_store_that_cannot_be_opened(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{chargesErr: openErr}), report.WithHome(refusalHome))

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow})

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it", refusal.Error())
}

func Test_anomalies_pass_on_a_charges_read_fault_that_is_not_a_refusal(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{chargesErr: errDiskRead}))

	_, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow})

	require.ErrorIs(t, err, errDiskRead)
	_, refused := errors.AsType[report.RefusalError](err)
	assert.False(t, refused)
}

func Test_anomalies_report_an_interrupt_during_the_charges_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{chargesErr: errDiskRead}))

	_, err := srv.Anomalies(ctx, report.AnomaliesRequest{Window: thisYear, Now: recurringNow})

	assert.EqualError(t, err, "anomalies interrupted")
}

func Test_anomalies_list_charges_dated_on_the_window_bounds_and_not_outside_them(t *testing.T) {
	window := store.Window{Since: dateOf(t, "2026-09-02"), Until: dateOf(t, "2026-09-10")}
	cases := []struct {
		name        string
		date        string
		wantListed  []int64
		wantChecked int
	}{
		{name: "the day before since is history only", date: "2026-09-01", wantListed: []int64{}, wantChecked: 0},
		{name: "a charge on since is listed", date: "2026-09-02", wantListed: []int64{50000}, wantChecked: 1},
		{name: "a charge on until is listed", date: "2026-09-10", wantListed: []int64{50000}, wantChecked: 1},
		{name: "the day after until is not listed", date: "2026-09-11", wantListed: []int64{}, wantChecked: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 3, 10000), chargeOn(t, 9, c.date, ofAmount(50000)))

			got := anomaliesIn(t, window, charges)

			assert.Equal(t, c.wantListed, amountsOf(got.Listed))
			assert.Equal(t, c.wantChecked, got.Checked)
		})
	}
}

func Test_anomalies_count_a_charge_the_day_before_the_window_as_history(t *testing.T) {
	window := store.Window{Since: dateOf(t, "2026-09-02"), Until: dateOf(t, "2026-09-10")}
	charges := append(earlierCharges(t, 2, 10000),
		chargeOn(t, 3, "2026-09-01", ofAmount(10000)),
		chargeOn(t, 9, "2026-09-02", ofAmount(50000)))

	got := anomaliesIn(t, window, charges)

	assert.Equal(t, []int64{50000}, amountsOf(got.Listed))
}

func Test_anomalies_list_newest_first_then_by_descending_source_id(t *testing.T) {
	cases := []struct {
		name             string
		dateA, dateB     string
		sourceA, sourceB int64
		wantSourceIDs    []int64
	}{
		{name: "same day, lower id from the earlier group", dateA: "2026-09-01", dateB: "2026-09-01", sourceA: 20, sourceB: 30, wantSourceIDs: []int64{30, 20}},
		{name: "same day, higher id from the earlier group", dateA: "2026-09-01", dateB: "2026-09-01", sourceA: 30, sourceB: 20, wantSourceIDs: []int64{30, 20}},
		{name: "the later date wins over a higher id, later in the second group", dateA: "2026-09-01", dateB: "2026-09-05", sourceA: 30, sourceB: 20, wantSourceIDs: []int64{20, 30}},
		{name: "the later date wins over a higher id, later in the first group", dateA: "2026-09-05", dateB: "2026-09-01", sourceA: 20, sourceB: 30, wantSourceIDs: []int64{20, 30}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 3, 10000, paidTo("payee-a", "A")), earlierCharges(t, 3, 10000, paidTo("payee-b", "B"))...)
			charges = append(charges,
				chargeOn(t, c.sourceA, c.dateA, paidTo("payee-a", "A"), ofAmount(50000)),
				chargeOn(t, c.sourceB, c.dateB, paidTo("payee-b", "B"), ofAmount(50000)))

			got := anomaliesIn(t, since(t, "2026-09-01"), charges)

			ids := []int64{}
			for _, a := range got.Listed {
				ids = append(ids, a.SourceID)
			}
			assert.Equal(t, c.wantSourceIDs, ids)
		})
	}
}
