// White-box: renderSpendingJSON's null key, negative amounts and empty-list
// forms are formatting rules, driven directly over a report.Spending.
package cli

import (
	"testing"
	"time"

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
	found := store.Spending{
		Rows: []store.SpendingRow{
			{Key: nil, Currency: "CAD", Spent: 4208},
			{Key: new("Auto:Fuel"), Currency: "CAD", Spent: -1500},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 2708}},
	}
	s := report.Spending{Spending: found, Window: spendWindow()}

	got, err := renderSpendingJSON(s)

	require.NoError(t, err)
	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "category",
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
	found := store.Spending{
		Rows: []store.SpendingRow{
			{Key: new("Costco"), Currency: "CAD", Spent: 30000},
			{Key: nil, Currency: "CAD", Spent: 4208},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 34208}},
	}
	s := report.Spending{Spending: found, Window: spendWindow(), By: store.SpendByPayee}

	got, err := renderSpendingJSON(s)

	require.NoError(t, err)
	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "payee",
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

func Test_renderSpendingJSON_writes_empty_lists_when_nothing_was_spent(t *testing.T) {
	got, err := renderSpendingJSON(report.Spending{Window: spendWindow()})

	require.NoError(t, err)
	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "category",
  "account_filter": [],
  "rows": [],
  "totals": [],
  "warnings": []
}
`, string(got))
}
