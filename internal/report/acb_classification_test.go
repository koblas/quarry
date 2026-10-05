package report_test

import (
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acbUnclassifiedOne = "acb needs every brokerage and retirement account classified; 1 account is in neither accounts.registered nor accounts.non-registered in " +
		"~/Library/Application Support/quarry/config.toml; quarry findings --type unclassified-account --status all lists it"
	acbUnclassifiedTwo = "acb needs every brokerage and retirement account classified; 2 accounts are in neither accounts.registered nor accounts.non-registered in " +
		"~/Library/Application Support/quarry/config.toml; quarry findings --type unclassified-account --status all lists them"
)

var (
	openBrokerage    = store.Account{ID: "acct-20", Name: "Open", Type: store.AccountTypeBrokerage, Currency: "CAD"}
	closedRetirement = store.Account{ID: "acct-21", Name: "Closed", Type: store.AccountTypeRetirement, Currency: "CAD", Closed: true}
	chequingUnlisted = store.Account{ID: "acct-22", Name: "Chequing", Type: "chequing", Currency: "CAD"}
)

func acbWithAccounts(t *testing.T, extra ...store.Account) (report.ACB, error) {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{history: store.InvestmentHistory{
		Accounts:   append(acbAccounts(), extra...),
		Securities: []store.Security{acbSecurity("sec-1", "XEQT", "CAD")},
	}}))

	return srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday})
}

func Test_acb_refuses_while_an_investment_account_is_unclassified(t *testing.T) {
	cases := []struct {
		name    string
		extra   []store.Account
		message string
	}{
		{name: "one open brokerage account", extra: []store.Account{openBrokerage}, message: acbUnclassifiedOne},
		{name: "one closed retirement account counts", extra: []store.Account{closedRetirement}, message: acbUnclassifiedOne},
		{name: "two accounts take the plural wording", extra: []store.Account{openBrokerage, closedRetirement}, message: acbUnclassifiedTwo},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := acbWithAccounts(t, c.extra...)

			refusal, ok := errors.AsType[report.RefusalError](err)
			require.True(t, ok)
			assert.Equal(t, c.message, refusal.Error())
			assert.Equal(t, report.RefusalGeneric, refusal.Kind)
		})
	}
}

func Test_acb_does_not_count_an_account_that_needs_no_classification(t *testing.T) {
	cases := []struct {
		name  string
		extra []store.Account
	}{
		{name: "every investment account is listed", extra: nil},
		{name: "a chequing account in neither list", extra: []store.Account{chequingUnlisted}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := acbWithAccounts(t, c.extra...)

			assert.NoError(t, err)
		})
	}
}

func Test_acb_refuses_an_unclassified_account_before_an_unknown_security(t *testing.T) {
	history := selectHistory(t)
	history.Accounts = append(history.Accounts, openBrokerage)

	_, err := selectACB(t, history, "sec-9")

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, acbUnclassifiedOne, refusal.Error())
}
