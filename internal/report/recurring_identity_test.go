package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_recurring_lists_both_payees_of_a_renamed_payee_and_names_the_series_by_the_latest(t *testing.T) {
	dates := monthlyDates(t)
	old := chargesOn(t, dates[:3], paidTo("payee-old", "NETFLIX.COM 1234"))
	renamed := chargesOn(t, dates[3:], paidTo("payee-new", "Netflix.com"))

	result := recurringOf(t, old, renamed)

	require.Len(t, result.Series, 1)
	assert.Equal(t, "Netflix.com", result.Series[0].Payee)
	assert.Equal(t, []report.SeriesPayee{
		{ID: "payee-old", Name: "NETFLIX.COM 1234"},
		{ID: "payee-new", Name: "Netflix.com"},
	}, result.Series[0].Payees)
}

func Test_recurring_lists_one_payee_once_however_many_charges_it_has(t *testing.T) {
	result := recurringOf(t, chargesOn(t, monthlyDates(t)))

	require.Len(t, result.Series, 1)
	assert.Equal(t, []report.SeriesPayee{{ID: "payee-gym", Name: "Gym"}}, result.Series[0].Payees)
}

func Test_recurring_lists_both_accounts_when_the_card_changes_mid_run(t *testing.T) {
	dates := monthlyDates(t)
	oldCard := chargesOn(t, dates[:3], onAccount("acct-old", "Old card"))
	newCard := chargesOn(t, dates[3:], onAccount("acct-new", "New card"))

	result := recurringOf(t, oldCard, newCard)

	require.Len(t, result.Series, 1)
	assert.Equal(t, []string{"acct-old", "acct-new"}, accountIDsOf(result.Series[0]))
}

func Test_recurring_leaves_out_an_account_used_only_before_the_run_began(t *testing.T) {
	stray := chargesOn(t, []string{"2025-11-01"}, onAccount("acct-stray", "Stray"))
	run := chargesOn(t, monthlyDates(t), onAccount("acct-run", "Run"))

	result := recurringOf(t, stray, run)

	require.Len(t, result.Series, 1)
	assert.Equal(t, []string{"acct-run"}, accountIDsOf(result.Series[0]))
}

func Test_recurring_sets_the_payee_key_of_a_series_grouped_by_name(t *testing.T) {
	result := recurringOf(t, chargesOn(t, monthlyDates(t), paidTo("payee-1", "Netflix.com")))

	require.Len(t, result.Series, 1)
	assert.Equal(t, new("netflix-com"), result.Series[0].PayeeKey)
}

func Test_recurring_leaves_the_payee_key_nil_for_a_series_grouped_by_payee_id(t *testing.T) {
	result := recurringOf(t, chargesOn(t, monthlyDates(t), paidTo("payee-77", "#4411")))

	require.Len(t, result.Series, 1)
	assert.Nil(t, result.Series[0].PayeeKey)
}

func Test_recurring_gives_each_currency_of_one_name_its_own_payees(t *testing.T) {
	cad := chargesOn(t, monthlyDates(t), paidTo("payee-cad", "Netflix.com"), onAccount("acct-cad", "Chequing"))
	usd := chargesOn(t, monthlyDates(t), paidTo("payee-usd", "NETFLIX.COM 1234"), billedIn("USD"), onAccount("acct-usd", "Dollars"))

	result := recurringOf(t, cad, usd)

	require.Len(t, result.Series, 2)
	assert.Equal(t, []report.SeriesPayee{{ID: "payee-cad", Name: "Netflix.com"}}, result.Series[0].Payees)
	assert.Equal(t, []report.SeriesPayee{{ID: "payee-usd", Name: "NETFLIX.COM 1234"}}, result.Series[1].Payees)
}

func accountIDsOf(s report.Series) []string {
	ids := make([]string, len(s.Accounts))
	for i, a := range s.Accounts {
		ids[i] = a.ID
	}
	return ids
}
