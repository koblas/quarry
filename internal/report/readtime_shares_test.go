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
