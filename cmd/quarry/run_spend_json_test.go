// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_spend_json_returns_spending_as_a_document(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1),
			usdChequingAccount("acct-usd", 2),
		},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 2), cents: -120450},
		spendSplit{id: "s03", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -4208},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -31210},
	))
	var stdout, stderr bytes.Buffer
	env := spendEnvAt(&stdout, &stderr, time.Date(2026, 9, 29, 22, 0, 0, 0, utcMinus5))

	exitCode := runWith(context.Background(), []string{"spend", "--json"}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "category",
  "currency": "CAD",
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
      "spent": "1204.50"
    },
    {
      "category": "Food:Groceries",
      "currency": "CAD",
      "spent": "123.45"
    },
    {
      "category": "Food:Groceries",
      "currency": "USD",
      "spent": "312.10"
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "spent": "1370.03"
    },
    {
      "currency": "USD",
      "spent": "312.10"
    }
  ],
  "warnings": []
}
`, stdout.String())
}
