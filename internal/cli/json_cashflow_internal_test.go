// White-box: renderCashFlowJSON's period label, rate and partial fields are formatting
// rules, driven directly over a report.CashFlow.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cashFlowTestRow(period string, spent, net int64, rate *float64, partial bool) report.CashFlowRow {
	var r report.CashFlowRow
	r.Period, r.Currency, r.Income, r.Spent, r.Net, r.SavingsRatePct = period, "USD", 100000, spent, net, rate
	r.Partial = partial
	return r
}

func Test_renderCashFlowJSON_writes_a_year_period_a_negative_rate_a_zero_rate_and_partial(t *testing.T) {
	c := report.CashFlow{
		Rows: []report.CashFlowRow{
			cashFlowTestRow("2026", 110000, -10000, new(-10.0), true),
			cashFlowTestRow("2027", 100000, 0, new(0.0), false),
		},
		Totals: []store.CashFlowTotal{{Currency: "USD", Income: 200000, Spent: 210000, Net: -10000, SavingsRatePct: new(-5.0)}},
		By:     store.CashFlowByYear,
		Window: spendWindow(),
	}

	got, err := renderCashFlowJSON(c, []string{})

	require.NoError(t, err)
	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "year",
  "account_filter": [],
  "periods": [
    {
      "period": "2026",
      "currency": "USD",
      "income": "1000.00",
      "spent": "1100.00",
      "net": "-100.00",
      "savings_rate_pct": -10,
      "partial": true
    },
    {
      "period": "2027",
      "currency": "USD",
      "income": "1000.00",
      "spent": "1000.00",
      "net": "0.00",
      "savings_rate_pct": 0,
      "partial": false
    }
  ],
  "totals": [
    {
      "currency": "USD",
      "income": "2000.00",
      "spent": "2100.00",
      "net": "-100.00",
      "savings_rate_pct": -5
    }
  ],
  "warnings": []
}
`, string(got))
}

func Test_renderCashFlowJSON_writes_empty_lists_not_null_for_an_empty_cash_flow(t *testing.T) {
	c := report.CashFlow{Window: spendWindow()}

	got, err := renderCashFlowJSON(c, []string{})

	require.NoError(t, err)
	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "month",
  "account_filter": [],
  "periods": [],
  "totals": [],
  "warnings": []
}
`, string(got))
}
