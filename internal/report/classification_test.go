package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	registeredID    = "acct-1"
	nonRegisteredID = "acct-2"
	unlistedID      = "acct-3"
)

func classifiedListing(t *testing.T, account store.Account, includeClosed bool, classification report.Classification) report.AccountListing {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{
		accounts: store.AccountList{Accounts: []store.AccountBalance{{Account: account}}},
	}))

	got, err := srv.Accounts(t.Context(), includeClosed, money.Native, classification)

	require.NoError(t, err)
	require.Len(t, got.Accounts, 1)

	return got
}

func Test_accounts_classify_each_account_by_the_list_that_names_its_id(t *testing.T) {
	classification := report.Classification{Registered: []string{registeredID}, NonRegistered: []string{nonRegisteredID}}
	cases := []struct {
		name    string
		account store.Account
		want    *bool
	}{
		{name: "listed registered", account: store.Account{ID: registeredID, Type: store.AccountTypeBrokerage}, want: new(true)},
		{name: "listed non-registered", account: store.Account{ID: nonRegisteredID, Type: store.AccountTypeBrokerage}, want: new(false)},
		{name: "investment account in neither list", account: store.Account{ID: unlistedID, Type: store.AccountTypeBrokerage}, want: nil},
		{name: "non-investment account in neither list", account: store.Account{ID: unlistedID, Type: "checking"}, want: nil},
		{name: "non-investment account listed registered", account: store.Account{ID: registeredID, Type: "checking"}, want: new(true)},
		{name: "non-investment account listed non-registered", account: store.Account{ID: nonRegisteredID, Type: "checking"}, want: new(false)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifiedListing(t, c.account, false, classification)

			assert.Equal(t, c.want, got.Classification.Of(got.Accounts[0].Account))
		})
	}
}

func Test_accounts_match_a_listed_id_exactly_and_case_sensitively(t *testing.T) {
	account := store.Account{ID: registeredID, Name: "Growth", Type: store.AccountTypeBrokerage}
	cases := []struct {
		name       string
		registered []string
		want       *bool
	}{
		{name: "exact id", registered: []string{registeredID}, want: new(true)},
		{name: "different letter case", registered: []string{"ACCT-1"}, want: nil},
		{name: "leading whitespace", registered: []string{" acct-1"}, want: nil},
		{name: "the account's name instead of its id", registered: []string{"Growth"}, want: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifiedListing(t, account, false, report.Classification{Registered: c.registered})

			assert.Equal(t, c.want, got.Classification.Of(got.Accounts[0].Account))
			assert.Equal(t, c.want == nil, got.Classification.Unclassified(got.Accounts[0].Account))
		})
	}
}

func Test_accounts_leave_an_investment_account_unclassified_only_when_no_list_names_it(t *testing.T) {
	classification := report.Classification{Registered: []string{registeredID}, NonRegistered: []string{nonRegisteredID}}
	cases := []struct {
		name    string
		account store.Account
		want    bool
	}{
		{name: "brokerage in neither list", account: store.Account{ID: unlistedID, Type: store.AccountTypeBrokerage}, want: true},
		{name: "retirement in neither list", account: store.Account{ID: unlistedID, Type: store.AccountTypeRetirement}, want: true},
		{name: "brokerage listed registered", account: store.Account{ID: registeredID, Type: store.AccountTypeBrokerage}, want: false},
		{name: "retirement listed non-registered", account: store.Account{ID: nonRegisteredID, Type: store.AccountTypeRetirement}, want: false},
		{name: "non-investment account in neither list", account: store.Account{ID: unlistedID, Type: "checking"}, want: false},
		{name: "non-investment account listed registered", account: store.Account{ID: registeredID, Type: "checking"}, want: false},
		{name: "closed brokerage in neither list", account: store.Account{ID: unlistedID, Type: store.AccountTypeBrokerage, Closed: true}, want: true},
		{name: "brokerage left out of reports", account: store.Account{ID: unlistedID, Type: store.AccountTypeBrokerage, NotInReports: true}, want: true},
		{name: "brokerage with linked tracking", account: store.Account{ID: unlistedID, Type: store.AccountTypeBrokerage, LinkedTracking: true}, want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifiedListing(t, c.account, true, classification)

			assert.Equal(t, c.want, got.Classification.Unclassified(got.Accounts[0].Account))
		})
	}
}

func Test_accounts_classify_two_accounts_with_the_same_name_each_by_its_own_id(t *testing.T) {
	first := store.AccountBalance{ID: registeredID, Name: "Growth", Type: store.AccountTypeBrokerage}
	second := store.AccountBalance{ID: unlistedID, Name: "Growth", Type: store.AccountTypeBrokerage}
	srv := report.NewServer(report.WithStore(fakeStore{accounts: store.AccountList{Accounts: []store.AccountBalance{first, second}}}))

	got, err := srv.Accounts(t.Context(), false, money.Native, report.Classification{Registered: []string{registeredID}})

	require.NoError(t, err)
	assert.Equal(t, new(true), got.Classification.Of(got.Accounts[0].Account))
	assert.Nil(t, got.Classification.Of(got.Accounts[1].Account))
}

func Test_accounts_carry_the_classification_with_closed_accounts_left_out_or_shown(t *testing.T) {
	open := store.AccountBalance{ID: registeredID, Type: store.AccountTypeBrokerage}
	closed := store.AccountBalance{ID: nonRegisteredID, Type: store.AccountTypeBrokerage, Closed: true}
	classification := report.Classification{Registered: []string{registeredID}, NonRegistered: []string{nonRegisteredID}}
	srv := report.NewServer(report.WithStore(fakeStore{accounts: store.AccountList{Accounts: []store.AccountBalance{open, closed}}}))
	cases := []struct {
		name          string
		includeClosed bool
		wantHidden    int
	}{
		{name: "closed account left out", includeClosed: false, wantHidden: 1},
		{name: "closed account shown", includeClosed: true, wantHidden: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := srv.Accounts(t.Context(), c.includeClosed, money.Native, classification)

			require.NoError(t, err)
			assert.Equal(t, classification, got.Classification)
			assert.Equal(t, c.wantHidden, got.Hidden)
		})
	}
}
