package main

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cashFlowLogPrefix     = "quarry: mcp: cash_flow: "
	linkedLineForCashFlow = `account "Linked" uses linked account tracking in Quicken, so cash_flow leaves it out, as Quicken's reports do`
)

func Test_run_mcp_cash_flow_returns_the_cashflow_json_document(t *testing.T) {
	cases := []struct {
		name        string
		cliArgs     []string
		arguments   map[string]any
		wantWarning string
	}{
		{
			name:        "all five given",
			wantWarning: linkedLineForCashFlow,
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
			if c.wantWarning != "" {
				assert.Contains(t, got.toolWarnings, c.wantWarning)
			}
		})
	}
}

func Test_run_mcp_cash_flow_refuses_an_account_without_its_name_on_stderr(t *testing.T) {
	cases := []struct {
		name       string
		arg        string
		want       string
		wantStderr string
		absent     []string
	}{
		{
			name: "no account has the name", arg: "Nope",
			want:       `no account named "Nope"; call describe_schema to list the accounts`,
			wantStderr: cashFlowLogPrefix + unknownAccountLog + "\n",
			absent:     []string{"Nope"},
		},
		{
			name: "the name is empty", arg: "",
			want:       `no account named ""; call describe_schema to list the accounts`,
			wantStderr: cashFlowLogPrefix + unknownAccountLog + "\n",
			absent:     []string{`""`},
		},
		{
			name: "two accounts share the name", arg: "Visa",
			want:       `2 accounts are named "Visa"; pass one of their ids instead: acct-812, acct-977`,
			wantStderr: cashFlowLogPrefix + ambiguousAccountLog + "\n",
			absent:     []string{"Visa", "acct-812", "acct-977"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStore(t, home, spendRows([]store.Account{
				chequingAccount("acct-chq", 1),
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-812", SourceID: 3, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			}))
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
				Name: "cash_flow", Arguments: map[string]any{"accounts": []string{c.arg}},
			})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, c.wantStderr, peer.stderr.String())
			for _, text := range c.absent {
				assert.NotContains(t, peer.stderr.String(), text)
			}
		})
	}
}
