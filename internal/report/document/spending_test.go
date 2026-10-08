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

func Test_NewSpending_writes_a_null_key_for_uncategorized_and_a_signed_amount_for_a_refund(t *testing.T) {
	s := report.Spending{
		Rows: []report.SpendingRow{
			{Key: nil, Currency: "CAD", Spent: 4208},
			{Key: new("Auto:Fuel"), Currency: "CAD", Spent: -1500},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 2708}},
		Window: window,
	}

	got := indented(t, document.NewSpending(s, []string{}))

	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "category",
  "currency": "native",
  "account_filter": [],
  "rows": [
    {
      "category": null,
      "currency": "CAD",
      "spent": "42.08"
    },
    {
      "category": "Auto:Fuel",
      "currency": "CAD",
      "spent": "-15.00"
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "spent": "27.08"
    }
  ],
  "warnings": []
}
`, got)
}

func Test_NewSpending_names_the_row_key_and_by_for_the_payee_grouping(t *testing.T) {
	s := report.Spending{
		Rows: []report.SpendingRow{
			{Key: new("Costco"), Currency: "CAD", Spent: 30000},
			{Key: nil, Currency: "CAD", Spent: 4208},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 34208}},
		Window: window,
		By:     store.SpendByPayee,
	}

	got := indented(t, document.NewSpending(s, []string{}))

	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "payee",
  "currency": "native",
  "account_filter": [],
  "rows": [
    {
      "payee": "Costco",
      "currency": "CAD",
      "spent": "300.00"
    },
    {
      "payee": null,
      "currency": "CAD",
      "spent": "42.08"
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "spent": "342.08"
    }
  ],
  "warnings": []
}
`, got)
}

func Test_NewSpending_writes_month_rows_with_their_partial_flag(t *testing.T) {
	s := report.Spending{
		Rows: []report.SpendingRow{
			{Key: new("2026-01"), Currency: "CAD", Spent: 5000, Partial: true},
			{Key: new("2026-02"), Currency: "CAD", Spent: 0},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 5000}},
		Window: window,
		By:     store.SpendByMonth,
	}

	got := indented(t, document.NewSpending(s, []string{}))

	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "month",
  "currency": "native",
  "account_filter": [],
  "rows": [
    {
      "month": "2026-01",
      "currency": "CAD",
      "spent": "50.00",
      "partial": true
    },
    {
      "month": "2026-02",
      "currency": "CAD",
      "spent": "0.00",
      "partial": false
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "spent": "50.00"
    }
  ],
  "warnings": []
}
`, got)
}

func Test_NewSpending_writes_empty_lists_when_nothing_was_spent(t *testing.T) {
	got := indented(t, document.NewSpending(report.Spending{Window: window}, []string{}))

	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "category",
  "currency": "native",
  "account_filter": [],
  "rows": [],
  "totals": [],
  "warnings": []
}
`, got)
}

func Test_NewSpending_puts_currency_right_after_by_in_every_mode(t *testing.T) {
	want := []string{"since", "until", "by", "currency", "account_filter", "rows", "totals", "warnings"}
	cases := []struct {
		name     string
		currency money.Currency
	}{
		{name: "CAD", currency: money.CAD},
		{name: "USD", currency: money.USD},
		{name: "native", currency: money.Native},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := indented(t, document.NewSpending(report.Spending{Window: window, Currency: c.currency}, []string{}))

			assert.Equal(t, want, topLevelKeys(t, []byte(got)))
			assert.Contains(t, got, `"currency": "`+c.currency.String()+`",`)
		})
	}
}

func Test_NewSpending_reads_back_with_every_row_total_and_warning_it_was_given(t *testing.T) {
	s := report.Spending{
		Rows: []report.SpendingRow{
			{Key: nil, Currency: "CAD", Spent: 4208},
			{Key: new("Auto:Fuel"), Currency: "CAD", Spent: -1500},
			{Key: new("Home"), Currency: "USD", Spent: 99},
		},
		Totals:   []store.SpendingTotal{{Currency: "CAD", Spent: 2708}, {Currency: "USD", Spent: 99}},
		Window:   window,
		Accounts: []store.Account{{ID: "a-1", Name: "Chequing"}},
		Currency: money.CAD,
	}
	type pair struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type total struct {
		Currency string `json:"currency"`
		Spent    string `json:"spent"`
	}
	var back struct {
		Since         string `json:"since"`
		Until         string `json:"until"`
		By            string `json:"by"`
		Currency      string `json:"currency"`
		AccountFilter []pair `json:"account_filter"`
		Rows          []struct {
			Category *string `json:"category"`
			Currency string  `json:"currency"`
			Spent    string  `json:"spent"`
		} `json:"rows"`
		Totals   []total  `json:"totals"`
		Warnings []string `json:"warnings"`
	}

	mustReadBack(t, document.NewSpending(s, []string{"first", "second"}), &back)

	assert.Equal(t, "2026-01-01", back.Since)
	assert.Equal(t, "2026-09-29", back.Until)
	assert.Equal(t, "category", back.By)
	assert.Equal(t, "CAD", back.Currency)
	assert.Equal(t, []pair{{ID: "a-1", Name: "Chequing"}}, back.AccountFilter)
	require.Len(t, back.Rows, 3)
	assert.Nil(t, back.Rows[0].Category)
	assert.Equal(t, "Auto:Fuel", *back.Rows[1].Category)
	assert.Equal(t, "-15.00", back.Rows[1].Spent)
	assert.Equal(t, "USD", back.Rows[2].Currency)
	assert.Equal(t, []total{{"CAD", "27.08"}, {"USD", "0.99"}}, back.Totals)
	assert.Equal(t, []string{"first", "second"}, back.Warnings)
}

func Test_NewSpending_copies_the_warnings_it_is_given(t *testing.T) {
	warnings := []string{"first"}

	doc := document.NewSpending(report.Spending{Window: window}, warnings)
	warnings[0] = "changed"

	assert.Equal(t, []string{"first"}, doc.Warnings)
}
