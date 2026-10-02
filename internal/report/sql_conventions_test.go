package report_test

import (
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/report"
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
