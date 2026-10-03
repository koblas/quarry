package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_run_mcp_cash_flow_returns_the_cashflow_json_document(t *testing.T) {
	cases := []struct {
		name      string
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "all five given",
			cliArgs: []string{
				"cashflow", "--since", "2026-01", "--until", "2026-08", "--by", "month", "--currency", "CAD",
				"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card",
			},
			arguments: map[string]any{
				"since": "2026-01", "until": "2026-08", "by": "month", "currency": "CAD",
				"accounts": []string{"Chequing", "US Chequing", "Linked", "Old Card"},
			},
		},
		{name: "none given", cliArgs: []string{"cashflow"}, arguments: map[string]any{}},
		{
			name:    "native currency",
			cliArgs: []string{"cashflow", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{
			name:    "grouped by year",
			cliArgs: []string{"cashflow", "--by", "year"}, arguments: map[string]any{"by": "year"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: populatedAnalysisStore, cliArgs: c.cliArgs, tool: "cash_flow", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "cashflow", "cash_flow"), got.toolWarnings)
		})
	}
}
