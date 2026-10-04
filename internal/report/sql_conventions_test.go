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
	for _, phrase := range []string{"v_spending", "v_cash_flow", "v_account_balances", "fx_rates", "transfers.from_split_id"} {
		assert.Contains(t, report.SQLConventions, phrase)
	}
}

func Test_sql_conventions_explain_investment_data(t *testing.T) {
	collapsed := strings.Join(strings.Fields(report.SQLConventions), " ")

	for _, phrase := range []string{
		"investment_transactions", "not in transactions", "split_new_shares",
		"holding_shares", "v_holdings", "Neither includes cash in investment accounts",
	} {
		assert.Contains(t, collapsed, phrase)
	}
	assert.NotContains(t, collapsed, "quarry does not convert prices yet")
}

func Test_sql_conventions_list_the_action_values(t *testing.T) {
	collapsed := strings.Join(strings.Fields(report.SQLConventions), " ")

	assert.True(t, strings.HasSuffix(collapsed, "action is one of "+strings.Join(store.Actions(), ", ")+"."))
}
