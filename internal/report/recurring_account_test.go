package report_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			run := onAccountOf(chqAccount, monthlyEndingOn(t, "2026-09-12", 5))
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
