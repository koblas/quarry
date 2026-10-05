package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// netWorthSeeder is seed as the store hook of a toolDocumentRun; seed sets the HOME both surfaces read.
func netWorthSeeder(seed func(*testing.T)) func(*testing.T, string) {
	return func(t *testing.T, _ string) {
		t.Helper()
		seed(t)
	}
}

func Test_run_mcp_net_worth_returns_the_networth_json_document(t *testing.T) {
	cases := []struct {
		name      string
		seed      func(*testing.T)
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "today by default", seed: seedNetWorthStore, cliArgs: []string{"networth"}, arguments: map[string]any{}},
		{
			name: "one day with a USD balance before the first rate", seed: seedNetWorthStore,
			cliArgs: []string{"networth", "--as-of", "2026-03-05"}, arguments: map[string]any{"as_of": "2026-03-05"},
		},
		{
			name: "month ends from since to until", seed: seedNetWorthHistoryStore,
			cliArgs:   []string{"networth", "--since", "2026-01", "--until", "2026-03"},
			arguments: map[string]any{"since": "2026-01", "until": "2026-03"},
		},
		{
			name: "month ends from since to today", seed: seedNetWorthHistoryStore,
			cliArgs: []string{"networth", "--since", "2026-01"}, arguments: map[string]any{"since": "2026-01"},
		},
		{
			name: "one day in native currencies", seed: seedNetWorthStore,
			cliArgs:   []string{"networth", "--as-of", "2026-03-12", "--currency", "native"},
			arguments: map[string]any{"as_of": "2026-03-12", "currency": "native"},
		},
		{
			name: "month ends in native currencies", seed: seedNetWorthHistoryStore,
			cliArgs:   []string{"networth", "--since", "2026-01", "--until", "2026-03", "--currency", "native"},
			arguments: map[string]any{"since": "2026-01", "until": "2026-03", "currency": "native"},
		},
		{
			name: "month ends before the first balance", seed: seedNetWorthStore,
			cliArgs:   []string{"networth", "--since", "2026-01", "--until", "2026-02"},
			arguments: map[string]any{"since": "2026-01", "until": "2026-02"},
		},
		{
			name: "one day before the first balance", seed: seedNetWorthStore,
			cliArgs: []string{"networth", "--as-of", "2026-03-01"}, arguments: map[string]any{"as_of": "2026-03-01"},
		},
		{
			name: "no account in the reports has a balance", seed: seedUncountedOnlyStore,
			cliArgs: []string{"networth", "--as-of", "2026-03-01"}, arguments: map[string]any{"as_of": "2026-03-01"},
		},
		{
			name: "CAD balances in USD with no exchange rate", seed: seedNetWorthCADOnlyStore,
			cliArgs:   []string{"networth", "--as-of", "2026-01-20", "--currency", "USD"},
			arguments: map[string]any{"as_of": "2026-01-20", "currency": "USD"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: netWorthSeeder(c.seed), cliArgs: c.cliArgs, tool: "net_worth", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, got.cliWarnings, got.toolWarnings)
		})
	}
}
