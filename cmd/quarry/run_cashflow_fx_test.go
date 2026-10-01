// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cashFlowReport is the part of cashflow's --json document the currency tests read.
type cashFlowReport struct {
	Currency string         `json:"currency"`
	Periods  []cashFlowMark `json:"periods"`
	Totals   []cashFlowMark `json:"totals"`
}

// cashFlowMark is one amount of a cashflow document: a period's row and a total alike.
type cashFlowMark struct {
	Period         string   `json:"period"`
	Currency       string   `json:"currency"`
	Income         string   `json:"income"`
	Spent          string   `json:"spent"`
	Net            string   `json:"net"`
	SavingsRatePct *float64 `json:"savings_rate_pct"`
}

func Test_run_cashflow_converts_each_period_to_cad_by_default(t *testing.T) {
	// Two 0.10 USD splits at 1.25 make 0.26 rounded each, 0.25 summed first.
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 31), cents: 100000},
		spendSplit{id: "s02", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 1, 30), cents: 8000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 10), cents: -20000},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 11), cents: -10},
		spendSplit{id: "s05", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 12), cents: -10},
		spendSplit{id: "s06", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 2, 5), cents: -8000},
		spendSplit{id: "s07", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 2, 6), cents: -1000},
	), rateOnJan2)
	period := []string{"--since", "2026-01", "--until", "2026-02"}

	t.Run("text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), append([]string{"cashflow"}, period...), spendEnv(&stdout, &stderr))

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		const w = 7
		assert.Equal(t, "Cash flow 2026-01-01 to 2026-02-28 in all accounts, amounts in CAD\n\n"+
			cashFlowLine(w, "Month", "Currency", "Income", "Spent", "Net", "Savings rate", "Status")+
			cashFlowLine(w, "2026-01", "CAD", "1,100.00", "200.26", "899.74", "81.8%", "")+
			cashFlowLine(w, "2026-02", "CAD", "0.00", "110.00", "-110.00", "n/a", "")+
			cashFlowLine(w, "Total", "CAD", "1,100.00", "310.26", "789.74", "71.8%", ""),
			stdout.String())
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), append([]string{"cashflow", "--json"}, period...), spendEnv(&stdout, &stderr))

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		var doc cashFlowReport
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
		assert.Equal(t, cashFlowReport{
			Currency: "CAD",
			Periods: []cashFlowMark{
				{Period: "2026-01", Currency: "CAD", Income: "1100.00", Spent: "200.26", Net: "899.74", SavingsRatePct: new(81.8)},
				{Period: "2026-02", Currency: "CAD", Income: "0.00", Spent: "110.00", Net: "-110.00"},
			},
			Totals: []cashFlowMark{{Currency: "CAD", Income: "1100.00", Spent: "310.26", Net: "789.74", SavingsRatePct: new(71.8)}},
		}, doc)
	})
}
