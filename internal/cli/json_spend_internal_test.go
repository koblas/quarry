// White-box: renderSpendingJSON's null key, negative amounts and empty-list
// forms are formatting rules, driven directly over a report.Spending.
package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func spendWindow() store.Window {
	return store.Window{
		Since: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
	}
}

func Test_renderSpendingJSON_writes_a_null_key_for_uncategorized_and_a_signed_amount_for_a_refund(t *testing.T) {
	s := report.Spending{
		Rows: []report.SpendingRow{
			{Key: nil, Currency: "CAD", Spent: 4208},
			{Key: new("Auto:Fuel"), Currency: "CAD", Spent: -1500},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 2708}},
		Window: spendWindow(),
	}

	got, err := renderSpendingJSON(s, []string{})

	require.NoError(t, err)
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
`, string(got))
}

func Test_renderSpendingJSON_names_the_row_key_and_by_for_the_payee_grouping(t *testing.T) {
	s := report.Spending{
		Rows: []report.SpendingRow{
			{Key: new("Costco"), Currency: "CAD", Spent: 30000},
			{Key: nil, Currency: "CAD", Spent: 4208},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 34208}},
		Window: spendWindow(),
		By:     store.SpendByPayee,
	}

	got, err := renderSpendingJSON(s, []string{})

	require.NoError(t, err)
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
`, string(got))
}

func Test_renderSpendingJSON_writes_month_rows_with_their_partial_flag(t *testing.T) {
	s := report.Spending{
		Rows: []report.SpendingRow{
			{Key: new("2026-01"), Currency: "CAD", Spent: 5000, Partial: true},
			{Key: new("2026-02"), Currency: "CAD", Spent: 0},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 5000}},
		Window: spendWindow(),
		By:     store.SpendByMonth,
	}

	got, err := renderSpendingJSON(s, []string{})

	require.NoError(t, err)
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
`, string(got))
}

func Test_renderSpendingJSON_writes_empty_lists_when_nothing_was_spent(t *testing.T) {
	got, err := renderSpendingJSON(report.Spending{Window: spendWindow()}, []string{})

	require.NoError(t, err)
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
`, string(got))
}

// topLevelKeys is the keys of the JSON object in doc, in document order.
func topLevelKeys(t *testing.T, doc []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(doc))
	_, err := dec.Token()
	require.NoError(t, err)
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		require.NoError(t, err)
		name, ok := key.(string)
		require.True(t, ok)
		keys = append(keys, name)
		var skipped json.RawMessage
		require.NoError(t, dec.Decode(&skipped))
	}
	return keys
}

func Test_renderSpendingJSON_puts_currency_right_after_by_in_every_mode(t *testing.T) {
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
			got, err := renderSpendingJSON(report.Spending{Window: spendWindow(), Currency: c.currency}, []string{})

			require.NoError(t, err)
			assert.Equal(t, want, topLevelKeys(t, got))
			assert.Contains(t, string(got), `"currency": "`+c.currency.String()+`",`)
		})
	}
}
