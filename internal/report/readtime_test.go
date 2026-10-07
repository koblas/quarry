package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const unclassifiedOne = "unclassified-account:acct-1"

// accountsFindings lists the findings of req over accounts and stored findings.
func accountsFindings(t *testing.T, req report.FindingsRequest, accounts []store.Account, stored ...store.Finding) report.FindingsListing {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{findings: store.FindingList{Findings: stored, Accounts: accounts}}))
	got, err := srv.Findings(t.Context(), req)
	require.NoError(t, err)
	return got
}

func brokerage(id, name string) store.Account {
	return store.Account{ID: id, Name: name, Type: store.AccountTypeBrokerage, Currency: "CAD"}
}

func Test_findings_lists_an_investment_account_that_neither_list_names(t *testing.T) {
	cases := []struct {
		name    string
		account store.Account
	}{
		{name: "a brokerage account", account: store.Account{ID: "acct-1", Type: store.AccountTypeBrokerage}},
		{name: "a retirement account", account: store.Account{ID: "acct-1", Type: store.AccountTypeRetirement}},
		{name: "a closed account", account: store.Account{ID: "acct-1", Type: store.AccountTypeBrokerage, Closed: true}},
		{name: "an account left out of reports", account: store.Account{ID: "acct-1", Type: store.AccountTypeBrokerage, NotInReports: true}},
		{name: "an account using linked tracking", account: store.Account{ID: "acct-1", Type: store.AccountTypeRetirement, LinkedTracking: true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := accountsFindings(t, report.FindingsRequest{}, []store.Account{c.account})

			assert.Equal(t, map[finding.Type][]string{finding.UnclassifiedAccount: {unclassifiedOne}}, listIDs(got))
		})
	}
}

func Test_findings_leaves_out_an_account_the_config_or_its_type_classifies(t *testing.T) {
	cases := []struct {
		name    string
		account store.Account
		class   report.Classification
	}{
		{name: "listed registered", account: brokerage("acct-1", "TFSA"), class: report.Classification{Registered: []string{"acct-1"}}},
		{name: "listed non-registered", account: brokerage("acct-1", "Margin"), class: report.Classification{NonRegistered: []string{"acct-1"}}},
		{name: "not an investment account", account: store.Account{ID: "acct-1", Type: "chequing"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := accountsFindings(t, report.FindingsRequest{Classification: c.class}, []store.Account{c.account})

			assert.Empty(t, got.Groups)
		})
	}
}

func Test_findings_describes_an_unclassified_account_by_one_item_with_no_first_found_time(t *testing.T) {
	account := store.Account{ID: "acct-1", Name: "Old RRSP", Type: store.AccountTypeRetirement, Currency: "USD", Closed: true}

	got := accountsFindings(t, report.FindingsRequest{}, []store.Account{account})

	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Findings, 1)
	listed := got.Groups[0].Findings[0]
	assert.Equal(t, []store.FindingItem{{
		AccountID: "acct-1", Account: "Old RRSP", AccountType: store.AccountTypeRetirement, Currency: "USD", Closed: true,
	}}, listed.Items)
	assert.Equal(t, []any{finding.StatusOpen, true, false, false}, []any{listed.Status, listed.FirstFoundAt.IsZero(), listed.New, listed.NewlyFixed})
	assert.Nil(t, listed.FixedAt)
}

func Test_findings_sorts_unclassified_accounts_by_name_ignoring_case_then_id(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		want     []string
	}{
		{
			name:     "a name before another stays first",
			accounts: []store.Account{brokerage("acct-9", "Alpha"), brokerage("acct-2", "Bravo")},
			want:     []string{"unclassified-account:acct-9", "unclassified-account:acct-2"},
		},
		{
			name:     "a name after another moves behind it",
			accounts: []store.Account{brokerage("acct-2", "Bravo"), brokerage("acct-9", "Alpha")},
			want:     []string{"unclassified-account:acct-9", "unclassified-account:acct-2"},
		},
		{
			name:     "case does not decide the order",
			accounts: []store.Account{brokerage("acct-1", "Beta"), brokerage("acct-2", "alpha")},
			want:     []string{"unclassified-account:acct-2", "unclassified-account:acct-1"},
		},
		{
			name:     "the id breaks a tie in the name",
			accounts: []store.Account{brokerage("acct-2", "Same"), brokerage("acct-1", "Same")},
			want:     []string{"unclassified-account:acct-1", "unclassified-account:acct-2"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := accountsFindings(t, report.FindingsRequest{}, c.accounts)

			assert.Equal(t, c.want, idsOf(got.Groups[0]))
		})
	}
}

func Test_findings_lists_an_unclassified_account_after_the_group_of_every_stored_type(t *testing.T) {
	got := accountsFindings(t, report.FindingsRequest{}, []store.Account{brokerage("acct-1", "TFSA")},
		dated("unused-category:cat-1", finding.UnusedCategory, march1))

	assert.Equal(t, []string{"unused-category:cat-1"}, idsOf(got.Groups[0]))
	assert.Equal(t, []string{unclassifiedOne}, idsOf(got.Groups[1]))
}

func Test_findings_ignores_an_unclassified_account_whose_id_is_in_the_ignore_list(t *testing.T) {
	req := report.FindingsRequest{Ignore: []string{unclassifiedOne}, Status: report.FindingsAll}

	got := accountsFindings(t, req, []store.Account{brokerage("acct-1", "TFSA")})

	assert.Equal(t, []finding.Status{finding.StatusIgnored}, []finding.Status{got.Groups[0].Findings[0].Status})
	assert.Equal(t, finding.Counts{Ignored: 1}, got.Counts)
	assert.Empty(t, got.Unmatched)
}

func Test_findings_reports_the_ignore_id_of_a_classified_account_as_unmatched(t *testing.T) {
	req := report.FindingsRequest{Ignore: []string{unclassifiedOne}, Classification: report.Classification{Registered: []string{"acct-1"}}}

	got := accountsFindings(t, req, []store.Account{brokerage("acct-1", "TFSA")})

	assert.Equal(t, []string{unclassifiedOne}, got.Unmatched)
}

func Test_findings_never_lists_an_unclassified_account_as_fixed(t *testing.T) {
	got := accountsFindings(t, report.FindingsRequest{Status: finding.StatusFixed}, []store.Account{brokerage("acct-1", "TFSA")})

	assert.Empty(t, got.Groups)
	assert.Equal(t, finding.Counts{Open: 1}, got.Counts)
}

func Test_findings_does_not_count_an_unclassified_account_as_new(t *testing.T) {
	newDuplicate := dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1)
	newDuplicate.New = true

	got := accountsFindings(t, report.FindingsRequest{}, []store.Account{brokerage("acct-1", "TFSA")}, newDuplicate)

	assert.Equal(t, finding.Counts{Open: 2, New: 1}, got.Counts)
}

func Test_findings_counts_only_unclassified_accounts_under_their_type_filter(t *testing.T) {
	req := report.FindingsRequest{Type: finding.UnclassifiedAccount}

	got := accountsFindings(t, req, []store.Account{brokerage("acct-1", "TFSA"), brokerage("acct-2", "Margin")},
		dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1))

	assert.Equal(t, finding.Counts{Open: 2}, got.Counts)
	assert.Equal(t, map[finding.Type][]string{finding.UnclassifiedAccount: {"unclassified-account:acct-2", unclassifiedOne}}, listIDs(got))
}

func Test_read_time_states_holds_only_the_open_unclassified_accounts_and_never_marks_one_new(t *testing.T) {
	stored := dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1)
	stored.New = true
	list := store.FindingList{
		Findings: []store.Finding{stored},
		Accounts: []store.Account{
			brokerage("acct-1", "TFSA"),
			brokerage("acct-2", "Margin"),
			{ID: "acct-3", Name: "Chequing", Type: "chequing"},
		},
	}

	got := report.ReadTimeStates(list, report.Classification{Registered: []string{"acct-2"}})

	assert.Equal(t, []finding.State{{ID: unclassifiedOne}}, got)
}

const (
	xeqtID     = "sec-1"
	oneAndHalf = int64(1_500_000)
)

// nonRegistered is a config listing acct-1 as non-registered.
func nonRegistered() report.Classification {
	return report.Classification{NonRegistered: []string{"acct-1"}}
}

// noCostAdd is an add_shares of oneAndHalf shares of sec-1 into account on 2026-03-day, with no cost basis.
func noCostAdd(id, account string, day int) store.InvestmentTransaction {
	return store.InvestmentTransaction{
		ID: id, AccountID: account, SecurityID: new(xeqtID), Date: time.Date(2026, time.March, day, 0, 0, 0, 0, time.UTC),
		Action: store.ActionAddShares, Shares: new(oneAndHalf), Currency: "USD",
	}
}

func sharesList(accounts []store.Account, txns ...store.InvestmentTransaction) store.FindingList {
	return store.FindingList{
		Accounts: accounts,
		Investments: store.Investments{
			Securities:   []store.Security{{ID: xeqtID, Name: "XEQT"}},
			Transactions: txns,
		},
	}
}

func sharesFindings(t *testing.T, req report.FindingsRequest, list store.FindingList) report.FindingsListing {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{findings: list}))
	got, err := srv.Findings(t.Context(), req)
	require.NoError(t, err)
	return got
}

func sharesIDs(got report.FindingsListing) []string { return listIDs(got)[finding.SharesWithoutCost] }

func Test_findings_lists_shares_without_cost_only_in_a_non_registered_account(t *testing.T) {
	accounts := []store.Account{
		brokerage("acct-1", "TFSA"),
		brokerage("acct-2", "Unlisted"),
		brokerage("acct-3", "Margin"),
		{ID: "acct-4", Name: "Old margin", Type: store.AccountTypeBrokerage, Closed: true},
	}
	class := report.Classification{Registered: []string{"acct-1"}, NonRegistered: []string{"acct-3", "acct-4"}}
	list := sharesList(accounts,
		noCostAdd("itxn-1", "acct-1", 1), noCostAdd("itxn-2", "acct-2", 2), noCostAdd("itxn-3", "acct-3", 3), noCostAdd("itxn-4", "acct-4", 4))

	got := sharesFindings(t, report.FindingsRequest{Classification: class}, list)

	assert.Equal(t, []string{"shares-without-cost:itxn-4", "shares-without-cost:itxn-3"}, sharesIDs(got))
}

func Test_findings_does_not_list_an_add_of_zero_or_negative_units(t *testing.T) {
	zero, negative := noCostAdd("itxn-1", "acct-1", 1), noCostAdd("itxn-2", "acct-1", 2)
	zero.Shares, negative.Shares = new(int64(0)), new(-oneAndHalf)
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, zero, negative, noCostAdd("itxn-3", "acct-1", 3))

	got := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered()}, list)

	assert.Equal(t, []string{"shares-without-cost:itxn-3"}, sharesIDs(got))
}

func Test_findings_does_not_list_an_add_with_no_unit_count(t *testing.T) {
	missing := noCostAdd("itxn-1", "acct-1", 1)
	missing.Shares = nil
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, missing, noCostAdd("itxn-2", "acct-1", 2))

	got := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered()}, list)

	assert.Equal(t, []string{"shares-without-cost:itxn-2"}, sharesIDs(got))
}

func Test_findings_does_not_list_a_reinvest_with_no_cost(t *testing.T) {
	reinvest := noCostAdd("itxn-1", "acct-1", 1)
	reinvest.Action = store.ActionReinvestDividend
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, reinvest, noCostAdd("itxn-2", "acct-1", 2))

	got := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered()}, list)

	assert.Equal(t, []string{"shares-without-cost:itxn-2"}, sharesIDs(got))
}

func Test_findings_does_not_list_an_add_that_has_a_cost(t *testing.T) {
	costed := noCostAdd("itxn-1", "acct-1", 1)
	costed.CostBasis = new(int64(30_000))
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, costed, noCostAdd("itxn-2", "acct-1", 2))

	got := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered()}, list)

	assert.Equal(t, []string{"shares-without-cost:itxn-2"}, sharesIDs(got))
}

func Test_findings_does_not_list_an_add_that_names_no_security(t *testing.T) {
	bare := noCostAdd("itxn-1", "acct-1", 1)
	bare.SecurityID = nil
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, bare, noCostAdd("itxn-2", "acct-1", 2))

	got := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered()}, list)

	assert.Equal(t, []string{"shares-without-cost:itxn-2"}, sharesIDs(got))
}

func Test_findings_lists_a_future_dated_add_with_no_cost(t *testing.T) {
	future := noCostAdd("itxn-1", "acct-1", 1)
	future.Date = time.Date(2999, time.January, 1, 0, 0, 0, 0, time.UTC)
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, future)

	got := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered()}, list)

	assert.Equal(t, []string{"shares-without-cost:itxn-1"}, sharesIDs(got))
}

func Test_findings_lists_no_shares_without_cost_when_the_classification_is_empty(t *testing.T) {
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, noCostAdd("itxn-1", "acct-1", 1))

	listed := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered()}, list)
	unreadable := sharesFindings(t, report.FindingsRequest{}, list)

	assert.Equal(t, []string{"shares-without-cost:itxn-1"}, sharesIDs(listed))
	assert.Empty(t, sharesIDs(unreadable))
}

func Test_findings_describes_shares_without_cost_by_one_item_with_the_accounts_currency_and_no_first_found_time(t *testing.T) {
	list := sharesList([]store.Account{{ID: "acct-1", Name: "Margin", Type: store.AccountTypeBrokerage, Currency: "CAD"}}, noCostAdd("itxn-7", "acct-1", 2))

	got := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered()}, list)

	listed := got.Groups[0].Findings[0]
	assert.Equal(t, []store.FindingItem{{
		Date: time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC), AccountID: "acct-1", Account: "Margin", Currency: "CAD",
		InvestmentTransactionID: new("itxn-7"), SecurityID: new(xeqtID), Security: "XEQT", Shares: oneAndHalf,
	}}, listed.Items)
	assert.Equal(t, finding.StatusOpen, listed.Status, "status")
	assert.True(t, listed.FirstFoundAt.IsZero(), "first found at is unset")
	assert.False(t, listed.New, "new")
	assert.False(t, listed.NewlyFixed, "newly fixed")
}

func Test_findings_sorts_shares_without_cost_newest_date_first_then_id(t *testing.T) {
	cases := []struct {
		name string
		txns []store.InvestmentTransaction
		want []string
	}{
		{
			name: "a newer date before an older stays first",
			txns: []store.InvestmentTransaction{noCostAdd("itxn-1", "acct-1", 2), noCostAdd("itxn-2", "acct-1", 1)},
			want: []string{"shares-without-cost:itxn-1", "shares-without-cost:itxn-2"},
		},
		{
			name: "a newer date after an older moves ahead of it",
			txns: []store.InvestmentTransaction{noCostAdd("itxn-1", "acct-1", 1), noCostAdd("itxn-2", "acct-1", 2)},
			want: []string{"shares-without-cost:itxn-2", "shares-without-cost:itxn-1"},
		},
		{
			name: "the id breaks a tie in the date",
			txns: []store.InvestmentTransaction{noCostAdd("itxn-2", "acct-1", 1), noCostAdd("itxn-1", "acct-1", 1)},
			want: []string{"shares-without-cost:itxn-1", "shares-without-cost:itxn-2"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered()}, sharesList([]store.Account{brokerage("acct-1", "Margin")}, c.txns...))

			assert.Equal(t, c.want, sharesIDs(got))
		})
	}
}

func Test_findings_ignores_shares_without_cost_whose_id_is_in_the_ignore_list(t *testing.T) {
	req := report.FindingsRequest{Classification: nonRegistered(), Ignore: []string{"shares-without-cost:itxn-1"}, Status: report.FindingsAll}
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, noCostAdd("itxn-1", "acct-1", 1), noCostAdd("itxn-2", "acct-1", 2))

	got := sharesFindings(t, req, list)

	assert.Equal(t, finding.Counts{Open: 1, Ignored: 1}, got.Counts)
	assert.Empty(t, got.Unmatched)
}

func Test_findings_never_lists_shares_without_cost_as_fixed(t *testing.T) {
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, noCostAdd("itxn-1", "acct-1", 1))

	fixed := sharesFindings(t, report.FindingsRequest{Classification: nonRegistered(), Status: finding.StatusFixed}, list)

	assert.Empty(t, fixed.Groups)
	assert.Equal(t, finding.Counts{Open: 1}, fixed.Counts)
}

func Test_findings_counts_only_shares_without_cost_under_their_type_filter(t *testing.T) {
	req := report.FindingsRequest{Classification: nonRegistered(), Type: finding.SharesWithoutCost}
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, noCostAdd("itxn-1", "acct-1", 1), noCostAdd("itxn-2", "acct-1", 2))
	list.Findings = []store.Finding{dated("duplicate:txn-1+txn-2", finding.Duplicate, march1, march1)}

	got := sharesFindings(t, req, list)

	assert.Equal(t, finding.Counts{Open: 2}, got.Counts)
	assert.Equal(t, map[finding.Type][]string{finding.SharesWithoutCost: {"shares-without-cost:itxn-2", "shares-without-cost:itxn-1"}}, listIDs(got))
}

func Test_read_time_states_includes_each_shares_without_cost_and_never_marks_one_new(t *testing.T) {
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, noCostAdd("itxn-1", "acct-1", 1))

	got := report.ReadTimeStates(list, nonRegistered())

	assert.Equal(t, []finding.State{{ID: "shares-without-cost:itxn-1"}}, got)
}

func Test_count_findings_counts_the_shares_without_cost_in_the_status_investments(t *testing.T) {
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, noCostAdd("itxn-1", "acct-1", 1), noCostAdd("itxn-2", "acct-1", 2))
	st := store.Status{Accounts: list.Accounts, Investments: list.Investments}

	got := report.CountFindings(st, []string{"shares-without-cost:itxn-2"}, nonRegistered())

	assert.Equal(t, finding.Counts{Open: 1, Ignored: 1}, got)
}

func Test_count_findings_counts_no_shares_without_cost_when_the_classification_is_empty(t *testing.T) {
	list := sharesList([]store.Account{brokerage("acct-1", "Margin")}, noCostAdd("itxn-1", "acct-1", 1))
	st := store.Status{Accounts: list.Accounts, Investments: list.Investments}

	got := report.CountFindings(st, []string{"unclassified-account:acct-1"}, report.Classification{})

	assert.Equal(t, finding.Counts{Ignored: 1}, got)
}
