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

// anomaliesNamed reads thisYear's anomalies over charges, ordered by date, naming accounts from list.
func anomaliesNamed(t *testing.T, list store.AccountList, charges []store.Charge, names ...string) report.Anomalies {
	t.Helper()
	slices.SortStableFunc(charges, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	srv := report.NewServer(report.WithStore(fakeStore{accounts: list, charges: store.Charges{Rows: charges}}))

	got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow, Accounts: names})

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
