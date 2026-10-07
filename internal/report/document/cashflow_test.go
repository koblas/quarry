package document_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
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

func Test_NewCashFlow_writes_a_year_period_a_negative_rate_a_zero_rate_and_partial(t *testing.T) {
	c := report.CashFlow{
		Rows: []report.CashFlowRow{
			cashFlowTestRow("2026", 110000, -10000, new(-10.0), true),
			cashFlowTestRow("2027", 100000, 0, new(0.0), false),
		},
		Totals: []store.CashFlowTotal{{Currency: "USD", Income: 200000, Spent: 210000, Net: -10000, SavingsRatePct: new(-5.0)}},
		By:     store.CashFlowByYear,
		Window: window,
	}

	got := indented(t, document.NewCashFlow(c, []string{}))

	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "year",
  "currency": "native",
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
`, got)
}

func Test_NewCashFlow_writes_empty_lists_not_null_for_an_empty_cash_flow(t *testing.T) {
	c := report.CashFlow{Window: window}

	got := indented(t, document.NewCashFlow(c, []string{}))

	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "month",
  "currency": "native",
  "account_filter": [],
  "periods": [],
  "totals": [],
  "warnings": []
}
`, got)
}

func Test_NewCashFlow_puts_currency_after_by_and_names_the_reporting_currency(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     string
	}{
		{name: "CAD", currency: money.CAD, want: "CAD"},
		{name: "native is the zero value", currency: money.Native, want: "native"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			flow := report.CashFlow{Window: window, Currency: c.currency}

			got := indented(t, document.NewCashFlow(flow, []string{}))

			assert.Equal(t, []string{"since", "until", "by", "currency", "account_filter", "periods", "totals", "warnings"}, topLevelKeys(t, []byte(got)))
			assert.Contains(t, got, `"currency": "`+c.want+`"`)
		})
	}
}

func Test_NewCashFlow_reads_back_with_every_period_total_and_warning_it_was_given(t *testing.T) {
	c := report.CashFlow{
		Rows: []report.CashFlowRow{
			cashFlowTestRow("2025", 80000, 20000, new(20.0), false),
			cashFlowTestRow("2026", 110000, -10000, nil, true),
		},
		Totals:   []store.CashFlowTotal{{Currency: "USD", Income: 200000, Spent: 190000, Net: 10000}},
		Window:   window,
		By:       store.CashFlowByYear,
		Accounts: []store.Account{{ID: "a-1", Name: "Chequing"}},
		Currency: money.USD,
	}
	type pair struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type total struct {
		Currency string `json:"currency"`
		Income   string `json:"income"`
		Spent    string `json:"spent"`
		Net      string `json:"net"`
	}
	var back struct {
		By            string `json:"by"`
		Currency      string `json:"currency"`
		AccountFilter []pair `json:"account_filter"`
		Periods       []struct {
			Period         string   `json:"period"`
			Spent          string   `json:"spent"`
			SavingsRatePct *float64 `json:"savings_rate_pct"`
			Partial        bool     `json:"partial"`
		} `json:"periods"`
		Totals   []total  `json:"totals"`
		Warnings []string `json:"warnings"`
	}

	mustReadBack(t, document.NewCashFlow(c, []string{"first"}), &back)

	assert.Equal(t, "year", back.By)
	assert.Equal(t, "USD", back.Currency)
	assert.Equal(t, []pair{{ID: "a-1", Name: "Chequing"}}, back.AccountFilter)
	require.Len(t, back.Periods, 2)
	assert.Equal(t, "2025", back.Periods[0].Period)
	assert.Equal(t, "800.00", back.Periods[0].Spent)
	assert.InDelta(t, 20.0, *back.Periods[0].SavingsRatePct, 0)
	assert.Nil(t, back.Periods[1].SavingsRatePct)
	assert.True(t, back.Periods[1].Partial)
	assert.Equal(t, []total{{"USD", "2000.00", "1900.00", "100.00"}}, back.Totals)
	assert.Equal(t, []string{"first"}, back.Warnings)
}

func Test_NewCashFlow_copies_the_warnings_it_is_given(t *testing.T) {
	warnings := []string{"first"}

	doc := document.NewCashFlow(report.CashFlow{Window: window}, warnings)
	warnings[0] = "changed"

	assert.Equal(t, []string{"first"}, doc.Warnings)
}
