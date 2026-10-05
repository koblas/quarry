package report_test

import (
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_sql_conventions_never_mention_masking(t *testing.T) {
	lower := strings.ToLower(report.SQLConventions)

	assert.NotContains(t, lower, "mask")
	assert.NotContains(t, lower, "redact")
}

func Test_sql_conventions_name_the_views_and_currencies_a_query_writer_needs(t *testing.T) {
	for _, phrase := range []string{"v_spending", "v_cash_flow", "v_account_balances", "v_balances_daily", "v_net_worth", "fx_rates", "transfers.from_split_id"} {
		assert.Contains(t, report.SQLConventions, phrase)
	}
}

func Test_sql_conventions_explain_investment_data(t *testing.T) {
	collapsed := strings.Join(strings.Fields(report.SQLConventions), " ")

	for _, phrase := range []string{
		"investment_transactions", "also has a row in transactions", "split_new_shares",
		"holding_shares", "v_holdings", "Neither includes cash in investment accounts",
	} {
		assert.Contains(t, collapsed, phrase)
	}
	assert.NotContains(t, collapsed, "quarry does not convert prices yet")
}

func Test_sql_conventions_say_what_cost_basis_is_and_leave_acb_to_the_command(t *testing.T) {
	collapsed := strings.Join(strings.Fields(report.SQLConventions), " ")

	assert.Contains(t, collapsed, "a sum of shares is not a holding. "+
		"cost_basis is the cost Quicken records for a buy, reinvested dividend or added shares (NULL when none). prices holds")
	assert.NotContains(t, collapsed, "acb")
}

func Test_sql_conventions_close_the_investment_paragraph_with_the_action_values(t *testing.T) {
	investment := strings.Join(strings.Fields(strings.Split(report.SQLConventions, "\n\n")[1]), " ")

	assert.True(t, strings.HasSuffix(investment, "action is one of "+strings.Join(store.Actions(), ", ")+"."))
}

func Test_sql_conventions_end_with_the_daily_balances_paragraph(t *testing.T) {
	paragraphs := strings.Split(report.SQLConventions, "\n\n")

	assert.True(t, strings.HasPrefix(paragraphs[len(paragraphs)-1], "v_balances_daily has one row per account per day"))
}

func Test_sql_conventions_close_the_balances_paragraph_with_the_net_worth_view_and_where_registered_lives(t *testing.T) {
	paragraphs := strings.Split(report.SQLConventions, "\n\n")
	balances := strings.Join(strings.Fields(paragraphs[len(paragraphs)-1]), " ")

	assert.True(t, strings.HasSuffix(balances, "v_net_worth has one row per day, account type and currency, "+
		"adding up the balances of the accounts Quicken's reports count, as quarry networth does; sum balance_cad or balance_usd "+
		"over one date for the total; a NULL there means no exchange rate for that day. "+
		"Which accounts are registered is not in the store; it is accounts.registered and accounts.non-registered in quarry's config, "+
		"and quarry accounts --json reports it as registered."))
}
