// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_cashflow_json_returns_cash_flow_as_a_document(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 31), cents: 910000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 10), cents: -300000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 1, 20), cents: -320000},
		spendSplit{id: "s04", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 2, 15), cents: -10000},
	))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"cashflow", "--json", "--since", "2026-01", "--until", "2026-02"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-02-28",
  "by": "month",
  "account_filter": [],
  "periods": [
    {
      "period": "2026-01",
      "currency": "CAD",
      "income": "9100.00",
      "spent": "6200.00",
      "net": "2900.00",
      "savings_rate_pct": 31.9,
      "partial": false
    },
    {
      "period": "2026-02",
      "currency": "CAD",
      "income": "0.00",
      "spent": "100.00",
      "net": "-100.00",
      "savings_rate_pct": null,
      "partial": false
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "income": "9100.00",
      "spent": "6300.00",
      "net": "2800.00",
      "savings_rate_pct": 30.8
    }
  ],
  "warnings": []
}
`, stdout.String())
}
