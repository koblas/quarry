package report_test

import (
	"testing"

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
