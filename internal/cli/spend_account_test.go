package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	chequingID = "acct-chq"
	visaID     = "acct-visa"
	oldCardID  = "acct-old"
	oldBankID  = "acct-bank"
	linkedID   = "acct-401k"
	bothID     = "acct-both"
)

func accountsStore(accounts ...store.Account) fakeReportStore {
	list := make([]store.AccountBalance, len(accounts))
	for i, a := range accounts {
		list[i] = store.AccountBalance{Account: a}
	}
	return fakeReportStore{accounts: store.AccountList{Accounts: list}}
}

func namedAccounts() fakeReportStore {
	return accountsStore(
		store.Account{ID: chequingID, Name: "Chequing"},
		store.Account{ID: visaID, Name: "Visa Infinite"},
		store.Account{ID: oldCardID, Name: "Old Card", NotInReports: true},
		store.Account{ID: oldBankID, Name: "Old Bank", NotInReports: true},
		store.Account{ID: linkedID, Name: "Netskope 401(k)", LinkedTracking: true},
		store.Account{ID: bothID, Name: "Old 401(k)", NotInReports: true, LinkedTracking: true},
	)
}

// withSpending is fake answering every spend read with one CAD total, so the window is not empty.
func withSpending(fake fakeReportStore) fakeReportStore {
	fake.spending = store.Spending{Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 100}}}
	return fake
}

func leftOutWarning(name string) string {
	return "account \"" + name + "\" is not used in reports in Quicken, so spend leaves it out; " +
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync"
}

func linkedTrackingWarning(name string) string {
	return "account \"" + name + "\" uses linked account tracking in Quicken, so spend leaves it out, as Quicken's reports do"
}

func Test_spend_captions_the_named_accounts(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", "visa infinite", "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in Visa Infinite, Chequing\n\nCategory  Currency  Spent\n", stdout.String())
}

func Test_spend_json_lists_the_named_accounts_in_account_filter(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr,
		"--account", "visa infinite", "--account", chequingID, "--json")

	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"category",
		"account_filter":[{"id":"acct-visa","name":"Visa Infinite"},{"id":"acct-chq","name":"Chequing"}],
		"rows":[],"totals":[{"currency":"CAD","spent":"1.00"}],"warnings":[]}`, stdout.String())
}

func Test_spend_passes_every_account_flag_to_the_report(t *testing.T) {
	var got store.SpendingParams
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.gotSpending = &got

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--account", "Old Card", "--account", visaID)

	require.NoError(t, err)
	assert.Equal(t, []string{oldCardID, visaID}, got.AccountIDs)
}

func Test_spend_json_puts_w2_in_warnings_unprefixed_before_w1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{MultiTagSplits: 2}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--by", "tag", "--account", "Old Card", "--json")

	require.NoError(t, err)
	const w1 = "2 splits carry more than one tag, so the rows add up to more than the total"
	warnings, err := json.Marshal([]string{leftOutWarning("Old Card"), w1})
	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"tag",
		"account_filter":[{"id":"acct-old","name":"Old Card"}],"rows":[],"totals":[],
		"warnings":`+string(warnings)+`}`, stdout.String())
	assert.Equal(t, "quarry: warning: "+leftOutWarning("Old Card")+"\nquarry: warning: "+w1+"\n", stderr.String())
}

func Test_spend_warns_once_per_named_account_left_out_of_reports_in_the_order_given(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr,
		"--account", "Old Card", "--account", chequingID, "--account", oldBankID, "--account", "old card")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("Old Card")+"\nquarry: warning: "+leftOutWarning("Old Bank")+"\n",
		stderr.String())
}

func Test_spend_warns_about_a_linked_tracking_account_in_the_order_given_among_the_left_out_warnings(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr,
		"--account", "Old Card", "--account", linkedID, "--account", oldBankID, "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("Old Card")+"\nquarry: warning: "+linkedTrackingWarning("Netskope 401(k)")+
		"\nquarry: warning: "+leftOutWarning("Old Bank")+"\n", stderr.String())
}

func Test_spend_warns_only_that_linked_tracking_leaves_out_an_account_that_is_also_not_in_reports(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr, "--account", bothID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+linkedTrackingWarning("Old 401(k)")+"\n", stderr.String())
}

func Test_spend_json_lists_a_linked_tracking_warning_before_a_left_out_of_reports_one_unprefixed(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr,
		"--account", linkedID, "--account", "Old Card", "--json")

	require.NoError(t, err)
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{linkedTrackingWarning("Netskope 401(k)"), leftOutWarning("Old Card")}, doc.Warnings)
}

func Test_spend_does_not_warn_about_an_account_in_reports(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, withSpending(namedAccounts()), spendNow, &stdout, &stderr, "--account", chequingID)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}

func Test_spend_refuses_an_unknown_account_without_warning_about_an_excluded_one(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", "Old Card", "--account", "Nowhere")

	require.EqualError(t, err, "no account named \"Nowhere\"; run quarry accounts --all to list them")
	assert.Empty(t, stderr.String())
	assert.Empty(t, stdout.String())
}

func Test_spend_refuses_an_unknown_account_without_warning_about_a_linked_tracking_one(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", linkedID, "--account", "Nowhere")

	require.EqualError(t, err, "no account named \"Nowhere\"; run quarry accounts --all to list them")
	assert.Empty(t, stderr.String())
	assert.Empty(t, stdout.String())
}
